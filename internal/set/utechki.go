package set

import (
	"context"
	"fmt"
)

// Проверка утечек (задача 6.3). Служба проверяет то, что может увидеть сама,
// и НАЗЫВАЕТ то, чего не видит. Судья пакетов на проводе это pktmon в госте
// (stend\proverit-utechki.ps1), а не эта функция: зелёный ответ здесь значит
// «по тем признакам, которые службе доступны», и пункты перечисляют признаки.

type ItogPunkta string

const (
	ItogOk         ItogPunkta = "ok"
	ItogUtechka    ItogPunkta = "utechka"
	ItogNeIzmereno ItogPunkta = "ne_izmereno"
	ItogNeVidim    ItogPunkta = "ne_vidim"
)

type PunktProverki struct {
	Imya  string     `json:"imya"`
	Itog  ItogPunkta `json:"itog"`
	Tekst string     `json:"tekst"`
}

type RezultatProverki struct {
	Punkty []PunktProverki `json:"punkty"`
	// Адрес выхода через туннель, если измерен: главный экран показывает его
	// под объектом, и мерить дважды незачем.
	AdresVyhoda string `json:"adres_vyhoda,omitempty"`
}

// VhodProverki это всё, что нужно знать о машине. Функции параметрами ради
// тестов: настоящие ходят в сеть и в netsh.
type VhodProverki struct {
	Podnyat       bool
	PortProksi    int
	AdresServera  string
	Endpoint      string
	SprositVyhod  func(ctx context.Context, endpoint string, portProksi int) (string, error)
	IPv6Zaglushen func() (bool, error)
}

func ProveritUtechki(ctx context.Context, v VhodProverki) RezultatProverki {
	var r RezultatProverki
	r.Punkty = append(r.Punkty, r.punktVyhoda(ctx, v))
	r.Punkty = append(r.Punkty, punktIPv6(v))
	r.Punkty = append(r.Punkty,
		PunktProverki{Imya: "DNS", Itog: ItogNeVidim,
			Tekst: "запросы к системному резолверу перехватывает hijack-dns внутри TUN; куда уходят пакеты, служба не видит, это меряет стенд по pktmon"},
		PunktProverki{Imya: "DoH браузера", Itog: ItogNeVidim,
			Tekst: "браузер с включённым DoH резолвит сам, мимо системного резолвера; правила по доменам его не видят, проверка тоже"},
	)
	return r
}

func (r *RezultatProverki) punktVyhoda(ctx context.Context, v VhodProverki) PunktProverki {
	p := PunktProverki{Imya: "адрес выхода"}
	switch {
	case !v.Podnyat:
		p.Itog, p.Tekst = ItogNeIzmereno, "туннель не поднят, сравнивать не с чем"
		return p
	case v.PortProksi == 0:
		// Напрямую из службы мерить нельзя: свои процессы идут мимо туннеля по
		// правилу петли, и домашний адрес тут был бы не утечкой, а замером
		// не того.
		p.Itog, p.Tekst = ItogNeIzmereno, "локальный прокси не поднят, через туннель спросить нечем"
		return p
	}
	cherez, err := v.SprositVyhod(ctx, v.Endpoint, v.PortProksi)
	if err != nil {
		p.Itog, p.Tekst = ItogNeIzmereno, fmt.Sprintf("эндпоинт не ответил через туннель: %v", err)
		return p
	}
	r.AdresVyhoda = cherez
	napryamuyu, err := v.SprositVyhod(ctx, v.Endpoint, 0)
	if err != nil {
		// Контрольная половина не ответила: сравнить нельзя, но адрес через
		// туннель уже есть и совпадение с сервером само по себе довод.
		if cherez == v.AdresServera {
			p.Itog, p.Tekst = ItogOk, fmt.Sprintf("интернет видит %s, это адрес сервера", cherez)
		} else {
			p.Itog, p.Tekst = ItogNeIzmereno, fmt.Sprintf("через туннель %s, напрямую эндпоинт не ответил: %v", cherez, err)
		}
		return p
	}
	if cherez == napryamuyu {
		p.Itog, p.Tekst = ItogUtechka, fmt.Sprintf("через туннель и напрямую один адрес %s: трафик идёт мимо сервера", cherez)
		return p
	}
	p.Itog = ItogOk
	if cherez == v.AdresServera {
		p.Tekst = fmt.Sprintf("интернет видит %s, это адрес сервера; напрямую %s", cherez, napryamuyu)
	} else {
		p.Tekst = fmt.Sprintf("интернет видит %s, напрямую %s", cherez, napryamuyu)
	}
	return p
}

func punktIPv6(v VhodProverki) PunktProverki {
	p := PunktProverki{Imya: "IPv6"}
	if !v.Podnyat {
		p.Itog, p.Tekst = ItogNeIzmereno, "правило ставится при подъёме туннеля"
		return p
	}
	est, err := v.IPv6Zaglushen()
	switch {
	case err != nil:
		p.Itog, p.Tekst = ItogNeIzmereno, fmt.Sprintf("брандмауэр не ответил: %v", err)
	case est:
		p.Itog, p.Tekst = ItogOk, "исходящий IPv6 закрыт правилом брандмауэра, мимо туннеля по нему не уйти"
	default:
		p.Itog, p.Tekst = ItogUtechka, "правила "+ImyaPravilaIPv6+" нет: IPv6 может уйти мимо туннеля"
	}
	return p
}
