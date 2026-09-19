package main

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"golang.org/x/sys/windows/registry"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Всплывающее уведомление ровно одно: вышла новая версия. Остальное окно
// говорит само.
//
// Поводов было пять: обновление, смена несущего, срыв, отказ подъёма,
// восстановление. Четыре сняты 19.09.2026 по решению владельца, и дело не
// только во внешнем виде всплывашек Windows. Каждый из четырёх повторял то,
// что УЖЕ нарисовано: состояние туннеля и имя несущего стоят на главном
// экране постоянно, а отказ приходит в статусе кодом и рисуется баннером с
// действием («Повторить», «Открыть серверы»). Всплывашка не добавляла к
// этому ничего, кроме себя самой, и появлялась тем чаще, чем хуже канал:
// на дрожащем соединении срыв и восстановление идут парами.
//
// Что осталось человеку с ЗАКРЫТЫМ окном: подсказка трея, где состояние
// написано словами и обновляется каждые пять секунд (trey.go), и цвет значка.
// Отказ, случившийся при закрытом окне, живёт в статусе службы и рисуется
// сразу, как только окно открыли, а не теряется вместе с всплывашкой.

type Uvedomlenie struct {
	Zagolovok, Tekst string
}

// chtoSoobshchit решает по паре состояний. Чистая функция, чтобы правила
// проверялись таблицей, а не запуском Windows.
//
// uzheSkazali это версия, о которой человеку уже говорили в прошлые запуски.
// Без неё «один раз на версию» держалось только в памяти процесса, а окно
// поднимается при каждом входе в Windows: в госте 13.09.2026 накопился 21
// всплывашка про одно и то же обновление.
func chtoSoobshchit(pred, nov protokol.StatusOtvet, uzheSkazali string) (Uvedomlenie, bool) {
	// Состояние туннеля не уведомляется вовсе: ни срыв, ни восстановление, ни
	// смена несущего. Это состояние, а не новость, и оно нарисовано в окне и
	// написано в подсказке трея.
	//
	// Обновление показывается один раз на версию, в любом состоянии туннеля.
	if nov.Obnovlenie != nil && nov.Obnovlenie.Versiya != uzheSkazali &&
		(pred.Obnovlenie == nil || pred.Obnovlenie.Versiya != nov.Obnovlenie.Versiya) {
		return Uvedomlenie{"есть обновление " + nov.Obnovlenie.Versiya, "установить можно в настройках"}, true
	}
	return Uvedomlenie{}, false
}

// Uvedomlyatel помнит прошлое состояние и показывает toast через службу
// уведомлений Wails. Отказ показа это строка в журнале: уведомление
// вторично, состояние на экране первично.
type Uvedomlyatel struct {
	sluzhba *notifications.NotificationService
	ikonka  string
	mu      sync.Mutex
	pred    protokol.StatusOtvet
	nomer   int
}

func novyyUvedomlyatel(sluzhba *notifications.NotificationService, ikonka []byte) *Uvedomlyatel {
	return &Uvedomlyatel{
		sluzhba: sluzhba,
		ikonka:  polozhitIkonku(ikonka),
		pred:    protokol.StatusOtvet{Sostoyanie: protokol.SostSluzhbaMolchit},
	}
}

// polozhitIkonku кладёт иконку на диск и прописывает её же системе.
//
// Своя, а не библиотечная: Wails берёт иконку уведомлений из ресурса номер 3,
// а go-winres кладёт нашу группу под именем "APP" (см. ikonkaOkna в main.go).
// Ресурс не находится, png не пишется, а путь к нему система всё равно
// запоминает. В госте 13.09.2026 это выглядело как уведомление без иконки:
// серый прямоугольник с именем программы.
//
// Отказ здесь это строка в журнале: уведомление без иконки хуже, чем с
// иконкой, но лучше, чем отсутствие программы.
func polozhitIkonku(ikonka []byte) string {
	if len(ikonka) == 0 {
		return ""
	}
	katalog := filepath.Join(os.Getenv("LOCALAPPDATA"), "Affory")
	if err := os.MkdirAll(katalog, 0o755); err != nil {
		log.Printf("каталог иконки уведомлений не создан: %v", err)
		return ""
	}
	put := filepath.Join(katalog, "uvedomlenie.png")
	if err := os.WriteFile(put, ikonka, 0o644); err != nil {
		log.Printf("иконка уведомлений не записана: %v", err)
		return ""
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\Affory`, registry.SET_VALUE)
	if err != nil {
		log.Printf("иконка уведомлений не прописана: %v", err)
		return put
	}
	defer k.Close()
	if err := k.SetStringValue("IconUri", put); err != nil {
		log.Printf("иконка уведомлений не прописана: %v", err)
	}
	return put
}

// Prinyat получает каждое состояние, как трей.
func (u *Uvedomlyatel) Prinyat(st protokol.StatusOtvet) {
	u.mu.Lock()
	pred := u.pred
	u.pred = st
	u.nomer++
	nomer := u.nomer
	u.mu.Unlock()
	uv, est := chtoSoobshchit(pred, st, u.skazali())
	if !est || u.sluzhba == nil {
		return
	}
	if st.Obnovlenie != nil && uv.Zagolovok != "" && st.Obnovlenie.Versiya != "" {
		u.zapomnit(st.Obnovlenie.Versiya)
	}
	go func() {
		// ThreadID НЕ передаём. Wails кладёт его в <header> без обязательного
		// атрибута arguments, Windows такой XML не разбирает и рисует свою
		// заглушку «New notification» вместо наших строк. Проверено в госте
		// 13.09.2026: тот же текст, посланный без header, виден полностью.
		opcii := notifications.NotificationOptions{
			ID: "affory-" + itoa(nomer), Title: uv.Zagolovok, Body: uv.Tekst,
		}
		if u.ikonka != "" {
			opcii.Attachments = []notifications.NotificationAttachment{
				{Path: u.ikonka, Type: "appLogoOverride"},
			}
		}
		if err := u.sluzhba.SendNotification(opcii); err != nil {
			log.Printf("уведомление не показано: %v", err)
		}
	}()
}

// О какой версии человеку уже говорили. Лежит в реестре текущего человека:
// у оболочки своего хранилища нет, а лезть в internal службы ей нельзя
// (granitsa_test.go). Отказ реестра это молчание, а не поломка: худшее, что
// случится, это лишняя всплывашка.
const klyuchOkna = `Software\Affory\Okno`
const znachenieSkazali = "uvedomil_ob_obnovlenii"

func (u *Uvedomlyatel) skazali() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, klyuchOkna, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(znachenieSkazali)
	if err != nil {
		return ""
	}
	return v
}

func (u *Uvedomlyatel) zapomnit(versiya string) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, klyuchOkna, registry.SET_VALUE)
	if err != nil {
		log.Printf("версия обновления не запомнилась: %v", err)
		return
	}
	defer k.Close()
	if err := k.SetStringValue(znachenieSkazali, versiya); err != nil {
		log.Printf("версия обновления не записалась: %v", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
