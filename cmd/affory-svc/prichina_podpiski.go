package main

import (
	"errors"
	"fmt"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Отказ подписки словами человека (01.10.2026).
//
// До этого в окно уезжал err.Error() целиком: «подписка не загрузилась: dial
// tcp: lookup vpn.example.net: getaddrinfow: This is usually a temporary
// error...». Английский текст Windows ничего не говорит человеку, а шаг, на
// котором всё сорвалось, был спрятан в скобках в самом конце. Теперь человеку
// идёт шаг и что из него следует, а технический текст остаётся в журнале
// службы (obnovitPodpiskuPoId).

// prichinaPodpiski отвечает, почему подписка не загрузилась. Строка короткая и
// со строчной буквы: её же показывает карточка подписки после точки-разделителя.
func prichinaPodpiski(err error) string {
	t := prichinaBezDorog(err)
	var obe otkazObeihDorog
	if errors.As(err, &obe) {
		t += "; через VPN тоже не вышло"
	}
	return t
}

func prichinaBezDorog(err error) string {
	switch {
	case errors.Is(err, ssylki.ErrPodpiskaIstekla):
		return "подписка истекла"
	case errors.Is(err, ssylki.ErrPodpiskaPusta):
		return "подписка не отдала ни одного сервера"
	case errors.Is(err, ssylki.ErrNeSsylki):
		return "подписка отдала файл настроек другого приложения, а не список ссылок"
	case errors.Is(err, ssylki.ErrPodpiskaVelika):
		return "по ссылке лежит что-то слишком большое для списка серверов"
	case errors.Is(err, ssylki.ErrPonizhenieTLS):
		return "сервер подписки увёл на незащищённый адрес, загрузка остановлена"
	case errors.Is(err, ssylki.ErrPodpiskaUstroystvo):
		// Текст собран нами же в ssylki.otkazUstroystva и уже человеческий.
		return err.Error()
	case errors.Is(err, errPodpiskaNeZadana):
		return "подписка не задана"
	}
	switch shagZagruzki(err) {
	case sboi.DNS:
		return "сервер подписки не нашёлся по имени: нет интернета или провайдер закрывает этот адрес"
	case sboi.TCP:
		return "сервер подписки не отвечает на подключение: он выключен или провайдер не пускает к нему"
	case sboi.TLS:
		return "защищённое соединение с сервером подписки сорвалось: его обрывают или подменяют по дороге"
	case sboi.Srok:
		return "сервер подписки не ответил вовремя"
	case sboi.Dostup:
		return "сервер подписки не принял ссылку: она неверная или её отозвали"
	case sboi.Otmena:
		return "обновление прервано"
	case sboi.Otvet:
		return prichinaKoda(kodOtveta(err))
	}
	return "подписка не загрузилась"
}

func kodOtveta(err error) int {
	var o ssylki.OtkazZagruzki
	if errors.As(err, &o) {
		return o.Kod
	}
	return 0
}

func prichinaKoda(kod int) string {
	switch {
	case kod == 404 || kod == 410:
		return "по этой ссылке подписки нет: её убрали или в ссылке ошибка"
	case kod >= 500:
		return fmt.Sprintf("сервер подписки сломался на своей стороне (код %d), попробуй позже", kod)
	case kod > 0:
		return fmt.Sprintf("сервер подписки ответил ошибкой (код %d)", kod)
	}
	return "сервер подписки ответил не тем, что ждали"
}

// otkazSOstatkom несёт вместе с отказом число серверов, которые у этой
// подписки остались от прошлых обновлений. Минус один значит «не знаем».
//
// Нужен ради одной фразы. Раньше окно на любой отказ писало «работают серверы
// из прошлого обновления», в том числе человеку, который только что добавил
// подписку и у которого серверов ноль: он искал работающие серверы, которых
// не было.
type otkazSOstatkom struct {
	err      error
	serverov int
}

func (o otkazSOstatkom) Error() string { return o.err.Error() }
func (o otkazSOstatkom) Unwrap() error { return o.err }

// tekstOtkazaPodpiski это причина плюс то, что из неё следует для списка.
func tekstOtkazaPodpiski(err error) string {
	t := prichinaPodpiski(err)
	var o otkazSOstatkom
	if !errors.As(err, &o) {
		return t
	}
	switch {
	case o.serverov > 0:
		return t + "; прежние серверы из неё сохранены"
	case o.serverov == 0:
		return t + "; серверов из неё пока нет"
	}
	return t
}
