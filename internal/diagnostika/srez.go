// Пакет diagnostika это подробный журнал для отладки: строка в секунду,
// выключен по умолчанию.
//
// Состав полей выбран не по вкусу, а по разбору живой машины 10.09.2026. Три
// дефекта волны 1.0.3 искались руками, и вот чего для этого не хватило в
// журналах:
//
//   - число дескрипторов пришлось мерить `handle64.exe` со стороны; счётчик
//     раз в секунду показал бы утечку графиком за час, а не за полтора суток;
//   - WSAENOBUFS в журнале это уже последствие, а занятость эфемерных портов
//     видна за двадцать минут до первого отказа;
//   - длительность падения считалась скриптом по двум разным строкам, потому
//     что ни одна запись не называла её целиком;
//   - посекундного ряда байт не было вовсе, а «падение на несколько секунд»
//     без него неотличимо от простоя человека за машиной.
//
// Отсюда правило пакета: срез собирается ЦЕЛИКОМ, а отказавший источник портит
// только своё поле. Диагностика нужна ровно в ту секунду, когда что-то уже
// сломано, и терять её из-за того же самого излома нельзя.
package diagnostika

import (
	"strings"
	"time"
)

// Yadro это всё, что берётся у ядра одним походом в clash_api. Отдельным типом,
// а не пятью возвращаемыми значениями: поход один, и отказывает он тоже целиком.
type Yadro struct {
	Soedineniy int
	// Байты за прошедшую секунду, а не с начала времён. Накопленный итог
	// прячет провал: разница двух больших чисел глазами не читается.
	Vverh     int64
	Vniz      int64
	Zaderzhka time.Duration
}

// Istochniki это швы наружу: всё, что берётся у системы и у ядра. Настоящие
// сборщики платформенные, а тест обязан гоняться на любой машине.
type Istochniki struct {
	Seychas  func() time.Time
	PidYadra func() int
	// Deskriptorov зовётся по pid: своим и ядра. Течь бывает у обоих, и
	// складывать их в одно число значит потерять, у кого именно.
	Deskriptorov func(pid int) (int, error)
	Porty        func() (zanyato, vsego int, err error)
	Yadro        func() (Yadro, error)
	Runtime      func() (gorutin int, pamyatBayt uint64)
}

// Srez это одна строка журнала.
type Srez struct {
	Vremya              time.Time `json:"vremya"`
	DeskriptorovSluzhby int       `json:"deskriptorov_sluzhby"`
	DeskriptorovYadra   int       `json:"deskriptorov_yadra,omitempty"`
	PortovZanyato       int       `json:"portov_zanyato"`
	PortovVsego         int       `json:"portov_vsego"`
	SoedineniyVYadre    int       `json:"soedineniy_v_yadre"`
	BaytVverh           int64     `json:"bayt_vverh"`
	BaytVniz            int64     `json:"bayt_vniz"`
	ZaderzhkaYadraMs    float64   `json:"zaderzhka_yadra_ms"`
	Gorutin             int       `json:"gorutin"`
	PamyatiMB           float64   `json:"pamyati_mb"`
	// Sboi называет источники, которые в эту секунду не ответили. Пусто, когда
	// всё сошлось; отсутствие источника сбоем не считается.
	Sboi string `json:"sboi,omitempty"`
	// Sobytie заполняется только у строк, которые пишутся не по таймеру:
	// разрыв, смена несущего, перезапуск ядра.
	Sobytie string `json:"sobytie,omitempty"`
}

// Snyat собирает срез. Ошибок не возвращает намеренно: см. шапку пакета.
func Snyat(i Istochniki) Srez {
	var s Srez
	var sboi []string
	if i.Seychas != nil {
		s.Vremya = i.Seychas()
	}
	if i.Deskriptorov != nil {
		if n, err := i.Deskriptorov(0); err != nil {
			sboi = append(sboi, "дескрипторы службы: "+err.Error())
		} else {
			s.DeskriptorovSluzhby = n
		}
		// Ядра может не быть вовсе: туннель опущен, и это покой, а не сбой.
		if i.PidYadra != nil {
			if pid := i.PidYadra(); pid > 0 {
				if n, err := i.Deskriptorov(pid); err != nil {
					sboi = append(sboi, "дескрипторы ядра: "+err.Error())
				} else {
					s.DeskriptorovYadra = n
				}
			}
		}
	}
	if i.Porty != nil {
		if zanyato, vsego, err := i.Porty(); err != nil {
			sboi = append(sboi, "порты: "+err.Error())
		} else {
			s.PortovZanyato, s.PortovVsego = zanyato, vsego
		}
	}
	if i.Yadro != nil {
		if y, err := i.Yadro(); err != nil {
			sboi = append(sboi, "ядро: "+err.Error())
		} else {
			s.SoedineniyVYadre = y.Soedineniy
			s.BaytVverh, s.BaytVniz = y.Vverh, y.Vniz
			s.ZaderzhkaYadraMs = float64(y.Zaderzhka) / float64(time.Millisecond)
		}
	}
	if i.Runtime != nil {
		gorutin, pamyat := i.Runtime()
		s.Gorutin = gorutin
		s.PamyatiMB = float64(pamyat) / (1 << 20)
	}
	s.Sboi = strings.Join(sboi, "; ")
	return s
}
