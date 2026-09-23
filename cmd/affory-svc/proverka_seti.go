package main

import (
	"context"
	"fmt"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/proby"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Раздельная проверка слоёв сети (A3, 22.09.2026).
//
// Наблюдатель спрашивает одно: отвечает ли HTTP через выбранный исходящий.
// Такая проба зелена при сломанном DNS и при съеденном UDP, потому что идёт
// другим путём. Измерено в госте 22.09.2026: местный резолвер недоступен,
// российские имена не разрешаются по 12 секунд на имя, а продукт всё это время
// отвечает «поднят» и ошибки не показывает.
//
// Слои разведены и спрашиваются порознь: туннель, имя, местный резолвер, UDP.
// Проверка идёт ПО ЗАПРОСУ, а не постоянно: серия UDP-пакетов и поход к
// резолверу стоят трафика, и платить за них каждые полминуты незачем.

// Имена и адреса проб. Контрольная цель UDP публичная и та же, к которой ходит
// туннельный резолвер конфига: если UDP не ходит к ней, не ходит и DNS ядра.
const (
	imyaProbyImeni = "cp.cloudflare.com"
	// Имя для пути МИМО туннеля: российский домен, который ядро направляет на
	// местный резолвер. Любое из семи проверенных в госте вело себя одинаково;
	// взято самое устойчивое.
	imyaProbyMimoVPN = "yandex.ru"
	celProbyUDP      = "1.1.1.1:53"
	// srokProverkiSeti это потолок всей проверки: пять проб подряд, и у каждой
	// свой худший случай. Срок ОТВЕТА в протоколе обязан быть больше него,
	// иначе канал оборвёт работающую команду; сверяется тестом.
	srokProverkiSeti = 30 * time.Second
)

// sloyProverki это один слой с его ответом. Vid уезжает в окно и не переводится
// там обратно в человеческие слова: подпись слоя приходит отсюда.
type sloyProverki struct {
	Vid        string `json:"vid"`
	Podpis     string `json:"podpis"`
	Proshlo    bool   `json:"proshlo"`
	Podrobno   string `json:"podrobno"`
	Millisekun int64  `json:"ms"`
}

func sloy(vid, podpis string, i proby.Itog) sloyProverki {
	return sloyProverki{Vid: vid, Podpis: podpis, Proshlo: i.Proshlo,
		Podrobno: i.Podrobno, Millisekun: i.Dlitelnost.Milliseconds()}
}

// checkNetwork прогоняет пробы слоёв и отвечает по строке на каждый.
//
// Отказ ОДНОГО слоя это не отказ проверки: человеку нужен именно список, где
// видно, что цело и что нет. Поэтому команда всегда отвечает успехом, а
// приговор каждому слою стоит внутри.
func (s *Sluzhba) checkNetwork(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	ctx, otmena := context.WithTimeout(ctx, srokProverkiSeti)
	defer otmena()
	return otvet(k.Id, k.Imya, map[string]any{
		"vremya": s.seychas().UTC().Format(time.RFC3339),
		"sloi":   s.proveritSloi(ctx),
	})
}

func (s *Sluzhba) proveritSloi(ctx context.Context) []sloyProverki {
	sloi := []sloyProverki{s.sloyTunnelya(ctx), s.sloyYadra(ctx)}
	sloi = append(sloi, sloy("imya", "Имена сайтов через VPN", proby.Imya(ctx, imyaProbyImeni)))
	sloi = append(sloi, s.sloyMestnogoRezolvera(ctx))
	u := proby.UDP(ctx, celProbyUDP)
	sloi = append(sloi, sloy("udp", "Голос и видео", u.Itog))
	return sloi
}

// sloyTunnelya смотрит на САМ адаптер, а не на трафик через него. Туннеля нет,
// адреса нет, маршрута через него нет - всё это разные беды, и лечатся они
// по-разному.
func (s *Sluzhba) sloyTunnelya(context.Context) sloyProverki {
	nach := s.seychas()
	dlit := func() time.Duration { return s.seychas().Sub(nach) }
	s.mu.Lock()
	sost, tun := s.sost, s.tun
	s.mu.Unlock()
	if sost != protokol.SostPodnyat {
		return sloy("tunnel", "VPN на этом компьютере", proby.Itog{Podrobno: "VPN выключен", Dlitelnost: dlit()})
	}
	adaptery, err := s.adaptery()
	if err != nil {
		return sloy("tunnel", "VPN на этом компьютере", proby.Itog{Podrobno: "список адаптеров недоступен: " + err.Error(), Dlitelnost: dlit()})
	}
	for _, a := range adaptery {
		if a.Indeks != tun.Indeks {
			continue
		}
		if len(a.Adresa) == 0 {
			return sloy("tunnel", "VPN на этом компьютере", proby.Itog{Podrobno: a.Imya + " включён, но без адреса", Dlitelnost: dlit()})
		}
		// Маршрут по умолчанию через туннель это и есть «трафик идёт туда».
		// Его отсутствие при живом адаптере значит, что чужая программа или
		// своя же прошлая копия перетянули маршрут на себя.
		if !a.Umolchanie {
			return sloy("tunnel", "VPN на этом компьютере", proby.Itog{
				Podrobno: a.Imya + " включён, но трафик идёт мимо него", Dlitelnost: dlit()})
		}
		return sloy("tunnel", "VPN на этом компьютере", proby.Itog{Proshlo: true, Dlitelnost: dlit(),
			Podrobno: fmt.Sprintf("%s, адрес %s", a.Imya, a.Adresa[0])})
	}
	return sloy("tunnel", "VPN на этом компьютере", proby.Itog{
		Podrobno: "сетевого подключения Affory нет в системе", Dlitelnost: dlit()})
}

// sloyYadra спрашивает само ядро через clash_api: живо ли оно и отвечает ли
// выбранный исходящий. Это прежняя проба наблюдателя, поставленная в ряд с
// остальными, а не вместо них.
func (s *Sluzhba) sloyYadra(ctx context.Context) sloyProverki {
	nach := s.seychas()
	adres, sekret := s.dostupKKlash()
	if adres == "" {
		return sloy("yadro", "Связь с сервером", proby.Itog{
			Podrobno: "ядро не запущено", Dlitelnost: s.seychas().Sub(nach)})
	}
	t, err := s.zamerit(ctx, adres, sekret, tegDlyaZamera())
	if err != nil {
		return sloy("yadro", "Связь с сервером", proby.Itog{
			Podrobno: "сервер не ответил: " + err.Error(), Dlitelnost: s.seychas().Sub(nach)})
	}
	return sloy("yadro", "Связь с сервером", proby.Itog{Proshlo: true, Dlitelnost: t,
		Podrobno: fmt.Sprintf("ответ за %s", proby.Millisekundy(t))})
}

// sloyMestnogoRezolvera проверяет путь МИМО туннеля: российское имя через
// системный резолвер.
//
// Прямого вопроса к адресу из конфига тут быть не может, и это измерено в
// госте 22.09.2026: на поднятом туннеле такой вопрос не возвращается вовсе
// (12.2 с к ЖИВОМУ 192.168.0.1), потому что пакет уходит в TUN и его забирает
// ядро. Слой краснел на здоровой сети.
//
// Российское имя выбрано потому, что ядро направляет его на местный резолвер
// (`rule_set=ru => route(mestnyy)`). Тем же опытом: при живом резолвере семь
// российских имён отвечали за 0 с, при мёртвом - ни одно, по 12 секунд на имя.
// Адрес резолвера уезжает в подробности: он лечится, а имя это только признак.
func (s *Sluzhba) sloyMestnogoRezolvera(ctx context.Context) sloyProverki {
	s.mu.Lock()
	adres := s.rezolverKonfiga
	s.mu.Unlock()
	if !adres.IsValid() {
		// Туннель опущен: конфига нет. Берём текущий адрес системы, чтобы
		// подробности и до подключения называли, кого именно спрашивали.
		if a, err := s.mestnyyBezTunnelya(); err == nil {
			adres = a
		}
	}
	i := s.probaImeni(ctx, imyaProbyMimoVPN)
	gde := "сервер имён этой сети"
	if adres.IsValid() {
		gde = adres.String()
	}
	if i.Proshlo {
		i.Podrobno += fmt.Sprintf(" (через %s)", gde)
	} else {
		// Один отказ в строке человеку ничего не говорит: надо назвать, ЧТО
		// именно перестанет открываться и где это чинить.
		i.Podrobno += fmt.Sprintf("; через %s открываются российские сайты и всё, что идёт мимо VPN", gde)
	}
	return sloy("mestnyy-dns", "Имена сайтов мимо VPN", i)
}

// prichinaRazryva называет, ЧТО именно сломалось, перед аварийным разрывом.
//
// Решения не меняет: рвать или нет, решено выше и по прежним правилам. Меняет
// ровно одно - человек получает причину вместо «VPN перестал нести трафик».
// Разница не косметическая: чужой маршрут поверх туннеля, умерший адаптер и
// недоступный резолвер чинятся по-разному, а выглядели одинаково.
//
// Пробы берутся только быстрые. Серия UDP занимает секунду, и платить ею за
// строку в сообщении, пока человек сидит без связи, незачем.
func (s *Sluzhba) prichinaRazryva(ctx context.Context) string {
	if tun := s.sloyTunnelya(ctx); !tun.Proshlo && tun.Podrobno != "" {
		return tun.Podrobno
	}
	s.mu.Lock()
	adres := s.rezolverKonfiga
	s.mu.Unlock()
	if !adres.IsValid() {
		return ""
	}
	if r := s.probaImeni(ctx, imyaProbyMimoVPN); !r.Proshlo {
		return r.Podrobno
	}
	return ""
}
