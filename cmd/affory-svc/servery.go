package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Nabor это всё, что служба помнит про серверы между запусками.
//
// Лежит внутри блоба DPAPI одним куском: разложить по трём файлам значило бы
// завести три момента, в которые они могут разъехаться, и ни одного места, где
// это заметно.
type Nabor struct {
	Servery []protokol.Server `json:"servery"`
	// Podpiska это АДРЕС, то есть секрет класса ключа. Наружу он не отдаётся
	// никогда, только признак «задана» и имя узла для экрана.
	Podpiska string `json:"podpiska,omitempty"`
	Vybran   string `json:"vybran,omitempty"`
	// Пустое поле это НЕ «неизвестно», а «человек про режим ничего не говорил».
	// Пишут его ровно три места, и каждое это след явного действия человека:
	// setRouteMode, zapomnitVybor и removeServer при чистке выбора. Бланкетная
	// запись при каждом сохранении набора сделала бы будущую смену умолчания
	// недоставляемой: расписание подписки пишет набор раз в 12 часов, и поле
	// проштамповалось бы в каждой установке.
	Rezhim protokol.Rezhim `json:"rezhim,omitempty"`
	// Правила исключений (волна 5). Рядом с серверами, а не отдельным файлом:
	// один путь записи, один заслон, одно хранилище.
	Pravila PravilaNabora `json:"pravila"`
}

// rezhimNabora отвечает, каким режимом маршрута жить набору.
//
// Выбранный вручную сервер означает ручной режим: иначе после перезапуска
// человек оказывается на «авто» вместо того, что выбрал, и решит, что настройка
// не сохранилась. Это записанное решение, см. TestVybrannyyServerStanovitsyaUmolchaniem
// в genkonfig.
func rezhimNabora(n Nabor) protokol.Rezhim {
	if n.Rezhim != "" {
		return n.Rezhim
	}
	if n.Vybran != "" {
		return protokol.RezhimRuchnoy
	}
	return rezhimPoUmolchaniyu
}

// Авто, переключено 02.09.2026 задачей Ш7-8 по решению владельца, после того
// как заслоны Ш7-1..Ш7-6 встали. Пока умолчание было ручным, расхождение
// выбранного и несущего не стреляло, потому что они совпадали; теперь стреляет,
// и всё, что об этом говорит человеку, уже написано.
//
// Юнит-тесты эту константу НЕ охраняют: ни один из них не читает поле default
// собранного конфига. Охраняют два места, оба заведены раньше:
// TestVAvtoUmolchanieSelektoraEtoAvto в генераторе и живой прогон оснастки.
const rezhimPoUmolchaniyu = protokol.RezhimAvto

var (
	ErrNetServerov  = errors.New("серверов нет")
	ErrServerNeNayd = errors.New("сервер не найден")
)

// naborIzHranilishcha читает набор из хранилища секретов.
//
// Имя не совпадает с полем s.nabor намеренно: поле это ШОВ, а это его настоящее
// значение по умолчанию. Одно имя на оба Go и не позволил бы.
func (s *Sluzhba) naborIzHranilishcha() (Nabor, error) {
	telo, err := s.sekretyChitat()
	if err != nil {
		return Nabor{}, err
	}
	var n Nabor
	if len(telo) == 0 {
		// Пустое хранилище это первый запуск, а не поломка. Отказ здесь показал
		// бы secrets-unreadable человеку, который просто ещё не добавил сервер.
		return n, nil
	}
	if err := json.Unmarshal(telo, &n); err != nil {
		return Nabor{}, fmt.Errorf("набор серверов не разбирается: %w", err)
	}
	// Неизвестный режим это НЕ отказ читать набор: блоб мог прийти импортом
	// чужого профиля, а отказ оставил бы человека без единого сервера из-за
	// одной строки. Приводим к пустому, дальше его посчитает rezhimNabora.
	//
	// Проверка живёт ЗДЕСЬ, а не в команде, которая режим ставит: importProfilya
	// пишет расшифрованный блоб в sekretyPisat как есть, без разбора в Nabor.
	// Замерено: без этой строки мусорное значение проходит насквозь.
	if n.Rezhim != protokol.RezhimAvto && n.Rezhim != protokol.RezhimRuchnoy {
		n.Rezhim = ""
	}
	return n, nil
}

func (s *Sluzhba) zapisatNabor(n Nabor) error {
	telo, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("набор серверов не сериализуется: %w", err)
	}
	return s.sekretyPisat(telo)
}

// VybrannyyServer отдаёт сервер, к которому подключаемся.
//
// Если выбранного нет (первый запуск, либо выбранный убрали из подписки), берём
// первый по списку, а не отказываем: человек, у которого сервер один, не должен
// сначала «выбирать» его.
func (n Nabor) VybrannyyServer() (protokol.Server, error) {
	if len(n.Servery) == 0 {
		return protokol.Server{}, ErrNetServerov
	}
	for _, s := range n.Servery {
		if s.Id == n.Vybran {
			return s, nil
		}
	}
	return n.Servery[0], nil
}

// dlyaEkrana убирает из сервера всё, что не должно уезжать в канал.
//
// Канал пускает INTERACTIVE осознанно (интерфейс не требует админа), а список
// серверов при этом содержит uuid, публичный ключ, shortId и пароли. Отдавать
// их наружу значит раздавать доступ к VPN всякому, кто дотянулся до канала, при
// том что показывать на экране их всё равно нечего.
func dlyaEkrana(s protokol.Server) protokol.Server {
	return protokol.Server{
		Id: s.Id, Imya: s.Imya, Transport: s.Transport,
		Host: s.Host, Port: s.Port,
		IzPodpiski:              s.IzPodpiski,
		NebezopasnyyIgnorirovan: s.NebezopasnyyIgnorirovan,
		SPinom:                  s.Pin != "",
	}
}

// dobavitServer кладёт сервер в набор, заменяя одноимённый.
//
// Замена, а не второй экземпляр: повторное добавление той же ссылки это обычное
// «человек скопировал ещё раз», и список из двух одинаковых серверов ему только
// мешает. Ключи при замене обновляются, а прежние сохраняются на поколение
// назад тем же правилом, что и при обновлении подписки.
func dobavitServer(n Nabor, novyy protokol.Server) Nabor {
	n.Servery = ssylki.Slit(n.Servery, dopolnit(n.Servery, novyy))
	return n
}

// dopolnit строит «новый список» для Slit: прежние плюс добавляемый.
//
// Slit убирает из подписки то, чего в новом списке нет, поэтому передать ему
// один добавляемый сервер значило бы стереть все остальные.
func dopolnit(bylo []protokol.Server, novyy protokol.Server) []protokol.Server {
	out := make([]protokol.Server, 0, len(bylo)+1)
	zamenili := false
	for _, s := range bylo {
		if s.Id == novyy.Id {
			out = append(out, novyy)
			zamenili = true
			continue
		}
		out = append(out, s)
	}
	if !zamenili {
		out = append(out, novyy)
	}
	return out
}
