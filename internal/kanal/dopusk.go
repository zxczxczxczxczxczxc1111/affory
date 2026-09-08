package kanal

import "context"

// Допуск едет в КОНТЕКСТЕ, а не в поле службы.
//
// Служба одна на все соединения, и поле пустило бы права одного клиента
// другому: админ подключился, поле стало true, админ отключился, поле осталось.
// Гонка при этом тихая и воспроизводится раз в месяц.
type klyuchDopuska struct{}

func SDopuskom(ctx context.Context, d Dopusk) context.Context {
	return context.WithValue(ctx, klyuchDopuska{}, d)
}

// DopuskIz возвращает допуск. Отсутствие допуска это НЕ админ: контекст без
// значения бывает только там, где проверку не проводили, и трактовать это в
// пользу вызывающего значит раздавать права по недосмотру.
func DopuskIz(ctx context.Context) Dopusk {
	d, _ := ctx.Value(klyuchDopuska{}).(Dopusk)
	return d
}
