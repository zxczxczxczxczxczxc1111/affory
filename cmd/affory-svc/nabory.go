package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Наборы правил (rule_set), которые служба даёт ядру.
//
// Один набор, категория российских доменов из sing-geosite: это то самое
// правило «category-ru → direct» из спеки (§«Домены в режиме TUN»), в новом
// формате. Список пополняется здесь, а не в генераторе: генератор адресов не
// выдумывает.
//
// Why the service downloads the file itself instead of leaving it to the core:
// a remote set with no cache and no initial_path is fetched at core start, and
// a failed fetch refuses the WHOLE config. The spec's rule for every download
// is the opposite: a failure never touches the tunnel. So the file is put on
// disk before the core starts, and a set that could not be fetched is simply
// left out of this start.
var naboryPoUmolchaniyu = []genkonfig.NaborPravil{{
	Teg: tegNaboraRu,
	URL: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ru.srs",
}}

// tegNaboraRu это тот набор, которым распоряжается выключатель в правилах.
const tegNaboraRu = "ru"

// Потолок на файл набора. category-ru весит сотни килобайт; шестнадцать
// мегабайт это защита от чужой страницы-заглушки, а не от набора.
const potolokNabora = 16 << 20

// Сколько ждать загрузку набора при подъёме. Дольше держать подъём ради
// доменного списка нельзя: туннель важнее исключений.
const zhdatNabor = 20 * time.Second

func katalogNaborov() string { return filepath.Join(sostoyanie.KatalogDannyh(), "nabory") }
func putKesha() string       { return filepath.Join(sostoyanie.KatalogDannyh(), "kesh.db") }

// naboryIzUmolchaniy кладёт каждому набору его initial_path в каталоге данных.
func naboryIzUmolchaniy() []genkonfig.NaborPravil {
	itog := make([]genkonfig.NaborPravil, 0, len(naboryPoUmolchaniyu))
	for _, n := range naboryPoUmolchaniyu {
		n.Fayl = filepath.Join(katalogNaborov(), n.Teg+".srs")
		itog = append(itog, n)
	}
	return itog
}

// adresaNaborov отдаёт адреса загрузки: их хосты обязаны стоять в правиле
// петли и в разрешающих правилах, иначе загрузка уедет в туннель.
func (s *Sluzhba) adresaNaborov() []string {
	var a []string
	for _, n := range s.naboryPoNastroyke() {
		a = append(a, n.URL)
	}
	return a
}

// naboryPoNastroyke это желаемые наборы за вычетом тех, от которых человек
// отказался. Одно место на оба списка: адреса для брандмауэра и файлы для
// ядра расходиться не должны, а расходились бы тихо.
func (s *Sluzhba) naboryPoNastroyke() []genkonfig.NaborPravil {
	zhelaemye := s.naboryZhelaemye()
	if !s.ruSpisokVyklyuchen() {
		return zhelaemye
	}
	// Выключатель снимает ИМЕННО российский набор, а не наборы вообще:
	// появится второй, и «выключить ru» не должно унести заодно и его.
	itog := make([]genkonfig.NaborPravil, 0, len(zhelaemye))
	for _, n := range zhelaemye {
		if n.Teg != tegNaboraRu {
			itog = append(itog, n)
		}
	}
	return itog
}

// ruSpisokVyklyuchen читает решение человека из набора.
//
// Отказ чтения это НЕ повод считать список выключенным: неизвестность здесь
// трактуется как умолчание, иначе запертое хранилище само меняло бы маршрут
// российских сайтов.
func (s *Sluzhba) ruSpisokVyklyuchen() bool {
	n, err := s.nabor()
	if err != nil {
		log.Printf("набор не прочитан, российский список остаётся включённым: %v", err)
		return false
	}
	return n.Pravila.BezRuSpiska
}

// podgotovitNabory возвращает наборы, чей файл лежит на диске: те, что уже
// были, и те, что удалось скачать прямо сейчас. Неудача загрузки это строка в
// журнале и набор, пропущенный на этот подъём, а не отказ подъёма.
func (s *Sluzhba) podgotovitNabory() []genkonfig.NaborPravil {
	var gotovy []genkonfig.NaborPravil
	for _, n := range s.naboryPoNastroyke() {
		if _, err := os.Stat(n.Fayl); err == nil {
			gotovy = append(gotovy, n)
			continue
		}
		if err := s.skachatNaborVFayl(n); err != nil {
			log.Printf("набор %s не загружен, подъём без него: %v", n.Teg, err)
			continue
		}
		gotovy = append(gotovy, n)
	}
	return gotovy
}

func (s *Sluzhba) skachatNaborVFayl(n genkonfig.NaborPravil) error {
	ctx, otm := context.WithTimeout(s.fonCtx, zhdatNabor)
	defer otm()
	telo, err := s.skachatNabor(ctx, n.URL)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(n.Fayl), 0o755); err != nil {
		return fmt.Errorf("каталог наборов не создан: %w", err)
	}
	// Через временный файл и переименование: ядро читает initial_path сам, и
	// половина набора на диске это половина правил без единой ошибки.
	vremennyy := n.Fayl + ".chast"
	if err := os.WriteFile(vremennyy, telo, 0o644); err != nil {
		return fmt.Errorf("набор не записан: %w", err)
	}
	if err := os.Rename(vremennyy, n.Fayl); err != nil {
		_ = os.Remove(vremennyy)
		return fmt.Errorf("набор не переименован: %w", err)
	}
	return nil
}

var errNaborVelik = errors.New("набор больше потолка")

// skachatNaborPoSeti это настоящая загрузка. Идёт обычным клиентом: до подъёма
// туннеля нет, а под поднятым хост набора стоит в правиле петли, то есть
// уходит мимо.
func skachatNaborPoSeti(ctx context.Context, adres string) ([]byte, error) {
	zapros, err := http.NewRequestWithContext(ctx, http.MethodGet, adres, nil)
	if err != nil {
		return nil, fmt.Errorf("адрес набора не разобран: %w", err)
	}
	klient := &http.Client{Timeout: zhdatNabor}
	otvet, err := klient.Do(zapros)
	if err != nil {
		return nil, err
	}
	defer otvet.Body.Close()
	if otvet.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("код ответа %d", otvet.StatusCode)
	}
	// Потолок+1 и сравнение: LimitReader на потолок усёк бы набор молча, а
	// усечённый бинарный набор ядро отвергло бы уже на старте.
	telo, err := io.ReadAll(io.LimitReader(otvet.Body, potolokNabora+1))
	if err != nil {
		return nil, err
	}
	if len(telo) > potolokNabora {
		return nil, errNaborVelik
	}
	return telo, nil
}
