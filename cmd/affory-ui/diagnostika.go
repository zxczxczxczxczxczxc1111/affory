package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/puti"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sobytiya"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/zhurnaly"
)

// Выгрузка диагностики (О6 аудита 1.6.1). Журналы собирает служба и отдаёт
// порциями, а файл пишет окно от имени человека по пути из диалога.
// Содержимое через страницу не проходит: она узнаёт только итог.
//
// С 1.10.0 выгрузка не зависит от службы. Кнопка «Сохранить журнал» стоит в
// окне любого отказа, а самый нужный отказ это как раз служба, которая не
// отвечает: тогда окно читает её журналы само (папка журналов открыта
// пользователям на чтение) и дописывает свой раздел: журнал окна, его
// падения и события Windows про службу и наши процессы.

// Шов для тестов: диалог сохранения.
var vybratKudaDiagnostiku = (*most).dialogDiagnostiki

// Швы для тестов: каталог журналов службы и чтение журнала событий. Тест не
// читает ни журналы живой службы, ни журнал событий машины.
var (
	katalogZhurnalovSluzhby = filepath.Join(puti.KatalogDannyh, puti.PodkatalogZhurnalov)
	prochitatSobytiya       = sobytiya.Prochitat
)

// Предел числа порций: выгрузка шести журналов по 1 МиБ это полтора десятка
// порций, и служба, которая отдаёт их без конца, это дефект, а не журнал.
const maksPorciy = 64

const (
	// Хвост журнала службы, тот же, что берёт сама служба.
	hvostZhurnala = 1 << 20
	hvostOkna     = 512 << 10
	hvostPadeniya = 256 << 10
	// Событий одного журнала Windows: служба, падающая по кругу, дала бы
	// тысячи одинаковых строк.
	predelSobytiy = 50
	// Две недели в миллисекундах, как их ждёт timediff в XPath журнала.
	dveNedeliMs = 14 * 24 * 60 * 60 * 1000
)

func (m *most) dialogDiagnostiki(imya string) (string, error) {
	d := m.app.Dialog.SaveFile().
		SetMessage("Куда сохранить журнал Affory").
		SetFilename(imya).
		AddFilter("Текст (*.txt)", "*.txt")
	if kat := katalogRabochegoStola(); kat != "" {
		d = d.SetDirectory(kat)
	}
	return otvetDialoga(d.PromptForSingleSelection())
}

// katalogRabochegoStola: журнал сохраняют, чтобы сразу переслать, и рабочий
// стол это место, где файл находят без поиска (просьба владельца 21.10.2026).
func katalogRabochegoStola() string {
	put, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil {
		log.Printf("рабочий стол не найден, журнал предложу сохранить в «Документы»: %v", err)
		return katalogDokumentov()
	}
	return put
}

// SohranitDiagnostiku спрашивает, куда сохранить, собирает выгрузку и пишет
// файл. Пустая строка без ошибки значит, что человек передумал.
func (m *most) SohranitDiagnostiku() (string, error) {
	imya := "affory-zhurnal-" + time.Now().Format("2006-01-02-150405") + ".txt"
	put, err := vybratKudaDiagnostiku(m, imya)
	if err != nil || put == "" {
		return "", err
	}
	dannye, bezSluzhby := m.sobratVygruzku()
	if err := zapisatTselikom(put, dannye); err != nil {
		log.Printf("выгрузка: файл %s не записан: %v", put, err)
		return "", err
	}
	log.Printf("выгрузка: сохранена в %s, %d байт", put, len(dannye))
	itog := fmt.Sprintf("сохранено: %s, %s", filepath.Base(put), razmerFayla(len(dannye)))
	if bezSluzhby {
		itog += ". Служба не ответила, её журналы прочитаны напрямую"
	}
	return itog, nil
}

// sobratVygruzku: часть службы (или её журналы, прочитанные окном) плюс
// раздел окна. Второе значение говорит, что служба не ответила.
func (m *most) sobratVygruzku() ([]byte, bool) {
	var b bytes.Buffer
	sluzhby, err := m.zabratDiagnostiku()
	if err == nil {
		b.Write(sluzhby)
	} else {
		log.Printf("выгрузка: служба не отдала диагностику, читаю журналы сам: %v", err)
		b.WriteString(ssylki.Vychistit(vygruzkaBezSluzhby(err), nil, nil))
	}
	b.WriteString(ssylki.Vychistit(razdelOkna(), nil, nil))
	return b.Bytes(), err != nil
}

// vygruzkaBezSluzhby читает журналы службы с диска. Ключей из набора окно не
// знает, поэтому вычистка идёт только по виду ссылок, и шапка это говорит.
func vygruzkaBezSluzhby(prichina error) string {
	var b bytes.Buffer
	b.WriteString("Affory, выгрузка диагностики\n")
	fmt.Fprintf(&b, "служба не ответила: %v\n", prichina)
	b.WriteString("журналы службы окно прочитало само: ключи вычищены только по виду ссылок\n")
	for _, imya := range puti.ZhurnalySluzhby {
		zhurnaly.DopisatRazdel(&b, imya, filepath.Join(katalogZhurnalovSluzhby, imya), hvostZhurnala)
	}
	return b.String()
}

// razdelOkna: описание окна, язык человека, журнал окна, падения окна и
// события Windows про службу и наши процессы.
func razdelOkna() string {
	var b bytes.Buffer
	b.WriteString("\n===== окно\n")
	fmt.Fprintln(&b, opisanieOkna())
	if yazyki, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME); err == nil {
		fmt.Fprintf(&b, "язык человека: %s\n", strings.Join(yazyki, ", "))
	} else {
		fmt.Fprintf(&b, "язык человека не прочитан: %v\n", err)
	}
	kat := katalogZhurnalaOkna()
	zhurnaly.DopisatRazdel(&b, puti.ZhurnalOkna, filepath.Join(kat, puti.ZhurnalOkna), hvostOkna)
	zhurnaly.DopisatRazdel(&b, puti.PadenieOkna, filepath.Join(kat, puti.PadenieOkna), hvostPadeniya)

	b.WriteString("\n===== события Windows за 14 дней\n")
	for _, z := range []struct {
		zhurnal, xpath string
		imena          []string
	}{
		// Диспетчер служб: не запустилась (7000, 7009, 7011), упала (7022,
		// 7023, 7024, 7031, 7034). В данных его отображаемое имя Affory.
		{"System", fmt.Sprintf("*[System[Provider[@Name='Service Control Manager'] and "+
			"(EventID=7000 or EventID=7009 or EventID=7011 or EventID=7022 or EventID=7023 or "+
			"EventID=7024 or EventID=7031 or EventID=7034) and TimeCreated[timediff(@SystemTime) <= %d]]]", dveNedeliMs),
			[]string{"affory"}},
		// Сбой процесса (1000), отчёт об ошибке (1001), зависание (1002).
		{"Application", fmt.Sprintf("*[System[Provider[@Name='Application Error' or @Name='Windows Error Reporting' or "+
			"@Name='Application Hang'] and (EventID=1000 or EventID=1001 or EventID=1002) and "+
			"TimeCreated[timediff(@SystemTime) <= %d]]]", dveNedeliMs),
			[]string{"affory-", "sing-box"}},
	} {
		fmt.Fprintf(&b, "-- %s\n", z.zhurnal)
		zapisi, err := prochitatSobytiya(z.zhurnal, z.xpath, predelSobytiy, func(s sobytiya.Zapis) bool {
			return s.Upominaet(z.imena...)
		})
		if err != nil {
			fmt.Fprintf(&b, "(не прочитан: %v)\n", err)
		}
		if len(zapisi) == 0 && err == nil {
			b.WriteString("(нет)\n")
		}
		for _, s := range zapisi {
			fmt.Fprintln(&b, s.Stroka())
		}
	}
	return b.String()
}

func (m *most) zabratDiagnostiku() ([]byte, error) {
	var id string
	var dannye []byte
	for range maksPorciy {
		telo, err := json.Marshal(map[string]any{"id": id, "smeshchenie": len(dannye)})
		if err != nil {
			return nil, err
		}
		k, err := m.komandaSluzhbe("exportDiagnostics", telo)
		if err != nil {
			return nil, err
		}
		var p struct {
			Id          string `json:"id"`
			Vsego       int    `json:"vsego"`
			Smeshchenie int    `json:"smeshchenie"`
			Kusok       string `json:"kusok"`
		}
		if err := json.Unmarshal(k.Telo, &p); err != nil {
			return nil, fmt.Errorf("порция диагностики не разобрана: %w", err)
		}
		if p.Smeshchenie != len(dannye) {
			return nil, fmt.Errorf("служба отдала порцию со смещения %d, а ждали %d", p.Smeshchenie, len(dannye))
		}
		kusok, err := base64.StdEncoding.DecodeString(p.Kusok)
		if err != nil {
			return nil, fmt.Errorf("порция диагностики не декодируется: %w", err)
		}
		id = p.Id
		dannye = append(dannye, kusok...)
		if len(dannye) >= p.Vsego {
			return dannye, nil
		}
		if len(kusok) == 0 {
			return nil, errors.New("служба отдала пустую порцию посреди выгрузки")
		}
	}
	return nil, fmt.Errorf("выгрузка не закончилась за %d порций", maksPorciy)
}

// zapisatTselikom пишет во временный файл рядом и подменяет им целевой: при
// отказе на полпути прежний файл остаётся как был, а полфайла не остаётся.
func zapisatTselikom(put string, dannye []byte) error {
	vremennyy, err := os.CreateTemp(filepath.Dir(put), ".affory-diagnostika-*")
	if err != nil {
		return fmt.Errorf("файл не создан: %w", err)
	}
	imya := vremennyy.Name()
	_, err = vremennyy.Write(dannye)
	if errZakr := vremennyy.Close(); err == nil {
		err = errZakr
	}
	if err == nil {
		err = os.Rename(imya, put)
	}
	if err != nil {
		if errUd := os.Remove(imya); errUd != nil && !errors.Is(errUd, os.ErrNotExist) {
			return fmt.Errorf("файл не записан: %w (временный %s остался: %v)", err, imya, errUd)
		}
		return fmt.Errorf("файл не записан: %w", err)
	}
	return nil
}

func razmerFayla(n int) string {
	switch {
	case n >= 1<<20:
		return strings.Replace(fmt.Sprintf("%.1f МБ", float64(n)/(1<<20)), ".", ",", 1)
	case n >= 1<<10:
		return fmt.Sprintf("%d КБ", n>>10)
	default:
		return fmt.Sprintf("%d Б", n)
	}
}
