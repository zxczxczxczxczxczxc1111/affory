package set

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Сверка часов машины с точным временем (1.10.0).
//
// Сбитые часы ломают подключение молча: VMess отвергает запрос старше
// полутора минут, Shadowsocks 2022 строже, а TLS при сбитых на сутки часах
// называет любой сертификат просроченным. Человек видел «ни один сервер не
// отвечает» и искал беду в сети.
//
// Время берётся из заголовка Date ответа по HTTP, без TLS: TLS при сбитых
// часах падает раньше заголовков, и сверить было бы нечем. Обе цели это
// адреса проверки связи, которые Windows и Android дёргают сами, то есть
// доступны почти в любой сети.

// CeliChasov это адреса сверки, ответ первого из них решает.
var CeliChasov = []string{
	"http://www.msftconnecttest.com/connecttest.txt",
	"http://connectivitycheck.gstatic.com/generate_204",
}

// ErrNetDaty: сервер ответил без заголовка Date или с негодным.
var ErrNetDaty = errors.New("ответ без годного заголовка Date")

// RashozhdenieChasov отдаёт, на сколько часы машины впереди точного времени:
// отрицательное значит «отстают». Точность около секунды: заголовок Date
// секундный, а середина запроса берётся за момент ответа.
func RashozhdenieChasov(ctx context.Context, celi []string) (time.Duration, error) {
	if len(celi) == 0 {
		return 0, errors.New("нет адресов сверки часов")
	}
	klient := &http.Client{
		// Прокси системы мимо: служба спрашивает сама, и чужой прокси с
		// перехватом подставил бы своё время.
		Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
		// Перенаправление портала входа тоже ответ с Date, ходить дальше незачем.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	type itog struct {
		d   time.Duration
		err error
	}
	ctx, otmena := context.WithCancel(ctx)
	defer otmena()
	otvety := make(chan itog, len(celi))
	for _, cel := range celi {
		go func(cel string) {
			d, err := sveritS(ctx, klient, cel)
			otvety <- itog{d, err}
		}(cel)
	}
	var oshibki []error
	for range celi {
		o := <-otvety
		if o.err == nil {
			return o.d, nil
		}
		oshibki = append(oshibki, o.err)
	}
	return 0, fmt.Errorf("часы не сверены: %w", errors.Join(oshibki...))
}

func sveritS(ctx context.Context, klient *http.Client, cel string) (time.Duration, error) {
	zapros, err := http.NewRequestWithContext(ctx, http.MethodGet, cel, nil)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", cel, err)
	}
	nachalo := time.Now()
	otvet, err := klient.Do(zapros)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", cel, err)
	}
	konec := time.Now()
	otvet.Body.Close()
	return rashozhdenieIz(otvet.Header.Get("Date"), nachalo, konec)
}

// rashozhdenieIz считает расхождение по заголовку Date и времени запроса.
// Чистая функция: проверяется без сети.
func rashozhdenieIz(data string, nachalo, konec time.Time) (time.Duration, error) {
	tochnoe, err := http.ParseTime(data)
	if err != nil || tochnoe.IsZero() {
		return 0, fmt.Errorf("%w: %q", ErrNetDaty, data)
	}
	// Середина запроса. Сравнение по стенным часам, а не по монотонным:
	// вопрос ровно про стенные.
	seredina := nachalo.Add(konec.Sub(nachalo) / 2).Round(0)
	// Date режет доли секунды вниз, поэтому точное время в среднем на полсекунды
	// позже записанного.
	return seredina.Sub(tochnoe.Add(500 * time.Millisecond)), nil
}
