package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/katalog"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Старые правила приложений, ставшие карточками сервисов (D2, 22.09.2026).
//
// Списка было два - «Приложения» и «Сервисы», - и один сервис жил в обоих
// сразу: «Discord» в приложениях это exe, «Discord» в сервисах это домены.
// Человеку это ровно один Discord. Теперь карточка сервиса несёт и домены, и
// программу, и правило приложения на тот же exe стало вторым местом, где
// живёт одно и то же.
//
// Старое правило СНИМАЕТСЯ, а не переезжает в карточку. Переезд пришлось бы
// достраивать догадками: охват запускаемых программ у карточки включён всегда,
// а у старого правила мог быть выключен, и молчаливое расширение маршрута на
// игры это ровно тот сорт правки, за которым потом не видно причины. Снятие
// ничего не выдумывает, а что включить обратно, человек решает сам - об этом
// сказано в описании выпуска.

// snyatPravilaStavshieServisami убирает правила приложений на программы, за
// которые теперь отвечают карточки сервисов.
//
// Правила на СВОИ программы - игру, браузер, рабочий клиент - не трогаются:
// карточки сервиса для них нет, и снимать их было бы нечем заменить.
//
// Идёт ОДИН раз за жизнь набора, под флагом. На каждом чтении она снимала бы и
// то правило, которое человек завёл руками уже после обновления, - а он мог
// завести его именно потому, что хочет маршрут для программы и не хочет для
// доменов сервиса.
//
// Возвращает отчёт, по строке на снятое правило. Молчаливая правка набора
// стоила разбору жалоб про подписки трёх заходов вслепую.
func (n *Nabor) snyatPravilaStavshieServisami() []string {
	if n.Pravila.StaryeProgrammySnyaty {
		return nil
	}
	n.Pravila.StaryeProgrammySnyaty = true
	if n.Pravila.Trafik == nil || len(n.Pravila.Trafik.Prilozheniya) == 0 {
		return nil
	}
	poImeni, err := programmyKataloga()
	if err != nil {
		// Вшитый каталог не читается - это отдельная беда, и сверять не с чем.
		// Набор остаётся как есть: правила приложений работают и без чистки.
		return []string{"чистка правил приложений пропущена: " + err.Error()}
	}

	ostalis := make([]protokol.PraviloPrilozheniya, 0, len(n.Pravila.Trafik.Prilozheniya))
	var otchyot []string
	for _, a := range n.Pravila.Trafik.Prilozheniya {
		id, ok := poImeni[strings.ToLower(filepath.Base(a.Put))]
		if !ok {
			ostalis = append(ostalis, a)
			continue
		}
		otchyot = append(otchyot, fmt.Sprintf(
			"правило приложения %s снято: этой программой теперь управляет карточка сервиса %q", filepath.Base(a.Put), id))
	}
	if len(otchyot) == 0 {
		return nil
	}
	n.Pravila.Trafik.Prilozheniya = ostalis
	return otchyot
}

// programmyKataloga отдаёт имя файла → id сервиса. Одно имя принадлежит одному
// сервису: `steam.exe` есть только у Steam.
func programmyKataloga() (map[string]string, error) {
	k, err := katalog.Chitat()
	if err != nil {
		return nil, err
	}
	itog := map[string]string{}
	for _, s := range k.Servisy {
		for _, imya := range s.Programmy {
			itog[strings.ToLower(imya)] = s.Id
		}
	}
	return itog, nil
}
