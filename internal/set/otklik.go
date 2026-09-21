package set

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Цель замера отклика. Тот же адрес, которым ядро меряет свои urltest: 204 без
// тела, отвечает быстро и стоит рядом с любым выходом.
const CelOtklikaPoUmolchaniyu = "https://www.gstatic.com/generate_204"

// Срок на оба круга вместе. Отклик, не уложившийся в него, для экрана всё
// равно значит «плохо», а разница между 8 и 30 секундами не меняет ничего.
const srokOtklika = 8 * time.Second

// Otklik меряет ОДИН круг до интернета через туннель.
//
// Разница с задержкой, которую отдаёт ядро, не в точности, а в вопросе.
// Ядро меряет urltest: дозвон через сервер вместе с рукопожатием протокола,
// TLS-рукопожатие с целью и сам запрос, то есть три-пять кругов до другой
// страны разом. Человек сравнивает эту цифру с пингом в игре или в Discord, а
// там один круг по уже открытому соединению, и при исправной связи наше число
// выходило втрое больше чужого (владелец, 21.09.2026: 169 против 65).
//
// Здесь соединение сначала прогревается запросом, который НЕ считается, и
// меряется второй запрос по нему же. Приём тот же, что unified-delay у
// Clash.Meta.
//
// portProksi это локальный mixed-вход sing-box. Через него запрос идёт тем же
// путём, что трафик приложений: правило маршрутизации отправляет этот вход в
// селектор. Прямой запрос из службы ушёл бы мимо туннеля и померил бы домашний
// канал.
func Otklik(ctx context.Context, cel string, portProksi int) (time.Duration, error) {
	if portProksi <= 0 {
		return 0, fmt.Errorf("локальный прокси не поднят, через VPN мерить нечем")
	}
	if cel == "" {
		cel = CelOtklikaPoUmolchaniyu
	}
	do, otm := context.WithTimeout(ctx, srokOtklika)
	defer otm()

	u := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", portProksi)}
	// Keep-alive включён НАМЕРЕННО, в этом весь замер: второй запрос обязан
	// пойти по соединению первого. Транспорт живёт ровно один замер и
	// закрывается здесь же: транспорт, собранный на вызов и брошенный, уносит
	// соединение в свой пул простоя и хоронит его там (разбор у klientKlash в
	// internal/yadra).
	tr := &http.Transport{Proxy: http.ProxyURL(u), MaxIdleConns: 1, IdleConnTimeout: srokOtklika}
	defer tr.CloseIdleConnections()
	kl := &http.Client{Transport: tr}

	if err := krug(do, kl, cel); err != nil {
		return 0, err
	}
	nachalo := time.Now()
	if err := krug(do, kl, cel); err != nil {
		return 0, err
	}
	proshlo := time.Since(nachalo)
	// Ноль на экране читается как «мгновенно». Через туннель он невозможен, но
	// часы Windows умеют отдавать одно и то же время дважды.
	if proshlo <= 0 {
		proshlo = time.Nanosecond
	}
	return proshlo, nil
}

// krug это один запрос HEAD и полное дочитывание ответа.
//
// Тело у HEAD пустое, но закрыть его обязательно: без этого соединение не
// вернётся в пул, и второй запрос пойдёт по НОВОМУ, то есть померит
// рукопожатие заново - ровно то, от чего мы уходим.
func krug(ctx context.Context, kl *http.Client, cel string) error {
	z, err := http.NewRequestWithContext(ctx, http.MethodHead, cel, nil)
	if err != nil {
		return fmt.Errorf("запрос замера не собран: %w", err)
	}
	o, err := kl.Do(z)
	if err != nil {
		return fmt.Errorf("замер не прошёл: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(o.Body, 4<<10))
	o.Body.Close()
	// Отказ цели это не отклик. Страница «407 требуется авторизация» от чужого
	// клиента на нашем порту пришла бы за миллисекунду и встала бы на экран
	// как отличная задержка.
	if o.StatusCode >= 400 {
		return fmt.Errorf("цель замера ответила кодом %d", o.StatusCode)
	}
	return nil
}
