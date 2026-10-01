package genkonfig

import "encoding/hex"

// Пинг по каждому серверу (01.10.2026).
//
// Прежде список серверов показывал задержку urltest из clash API: дозвон через
// сервер, рукопожатие протокола, TLS с целью и сам запрос, то есть три-пять
// кругов разом. Человек сравнивает число с пингом в игре или в Discord, где круг
// один, и наше выходило втрое больше (166-797 мс против привычных 60).
//
// Строка «Задержка» на главном экране меряет по-честному: прогревает соединение
// через локальный прокси и считает второй запрос. Но прокси ведёт в селектор, то
// есть только в выбранный сервер. Этот вход ведёт в ЛЮБОЙ: логин прокси называет
// сервер, и правило маршрута отправляет такой запрос в его исходящий.

// TegZamerVhod это тег входа для замеров.
const TegZamerVhod = "zamer-in"

// VhodZamera это порт входа для замеров и общий пароль всех его логинов.
// Пароль нужен не от человека, а от соседних программ: без него любая из них
// ходила бы через выбранный ею сервер мимо правил маршрута.
type VhodZamera struct {
	Port  int
	Parol string
}

// PolzovatelZamera это логин входа для сервера id. Идентификатор берётся в
// шестнадцатеричном виде: в нём бывают любые знаки, а двоеточие в логине HTTP
// прокси разрезало бы его надвое.
func PolzovatelZamera(id string) string { return "z" + hex.EncodeToString([]byte(id)) }

// vhodZamera строит вход для замеров. Вход слушает только петлю, как и прокси
// рядом с туннелем.
func vhodZamera(v Vhod, idy []string) (map[string]any, bool) {
	if v.Zamer == nil || v.Zamer.Port <= 0 || len(idy) == 0 {
		return nil, false
	}
	polzovateli := make([]any, 0, len(idy))
	for _, id := range idy {
		polzovateli = append(polzovateli, map[string]any{"username": PolzovatelZamera(id), "password": v.Zamer.Parol})
	}
	return map[string]any{
		"type": "mixed", "tag": TegZamerVhod,
		"listen": "127.0.0.1", "listen_port": v.Zamer.Port,
		"users": polzovateli,
	}, true
}

// pravilaZamera ведут логин сервера в его исходящий. Стоят первыми в списке:
// замер обязан идти ровно через названный сервер, и ни обход, ни правило
// процессов, ни реклама не должны увести его в сторону. Последнее правило
// отбивает всё, что пришло на этот вход без известного логина: молча уйти в
// селектор значило бы померить не тот сервер.
func pravilaZamera(v Vhod, idy []string) []any {
	if _, est := vhodZamera(v, idy); !est {
		return nil
	}
	p := make([]any, 0, len(idy)+1)
	for _, id := range idy {
		p = append(p, map[string]any{
			"inbound":   []string{TegZamerVhod},
			"auth_user": []string{PolzovatelZamera(id)},
			"outbound":  TegKandidata(id),
		})
	}
	return append(p, map[string]any{"inbound": []string{TegZamerVhod}, "action": "reject"})
}
