package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Причина отказа пробы живого ядра (02.10.2026).
//
// Проба через Clash API на любой отказ звонка получает 503 без причины: ядро
// её выбрасывает. До этой правки 503 называлось «сервер не принял ключ» и
// вело чинить подписку, а 02.10.2026 так выглядел истёкший сертификат
// сервера при исправном ключе. Причину ядро всё-таки пишет, но только в
// журнал и только на звонки трафика: строкой с группой vybor, через которую
// идёт и проба. Если в эти минуты такая строка с узнанной причиной была,
// проба называет её.
//
// Граница «не раньше» обязательна. Группа vybor сервер не называет, и после
// переключения её прежние жалобы говорят о прежнем сервере.

// srokZhalobyYadra это насколько старая жалоба ещё про ту же беду. Наблюдатель
// судит по двум провалам подряд с шагом в полминуты, и строки трафика за это
// время успевают прийти; старше двух минут жалоба могла быть про другое.
const srokZhalobyYadra = 2 * time.Minute

// otkazZashchity значит: проба не прошла, а ядро в эти минуты назвало, почему
// не устанавливается защищённое соединение с сервером.
//
// Своим типом и без Unwrap к ErrServerOtvergKlyuchi намеренно: тот признак
// ведёт к коду server-auth-failed и кнопке «Обновить подписку», а здесь ключ
// ни при чём.
type otkazZashchity struct {
	prichina sboi.PrichinaYadra
}

func (o *otkazZashchity) Error() string { return o.prichina.Tekst() }

func estOtkazZashchity(err error) bool {
	var o *otkazZashchity
	return errors.As(err, &o)
}

// zameritYadro это проба живого ядра с причиной из его журнала. posle
// отсекает жалобы, которые к этой пробе не относятся.
func (s *Sluzhba) zameritYadro(ctx context.Context, adres, sekret, teg string, posle time.Time) (time.Duration, error) {
	d, err := s.zamerit(ctx, adres, sekret, teg)
	if err == nil || !errors.Is(err, yadra.ErrServerOtvergKlyuchi) {
		return d, err
	}
	zh, est := s.zhalobaVyhoda(putKonfigaTun(), teg, posle)
	if !est {
		return d, err
	}
	return d, &otkazZashchity{prichina: zh.Prichina}
}

// srokUtochneniya это сколько доспрос причины может добавить к отказу подъёма
// или переключения. Ядро замера встаёт за секунду-две, а отказ защищённого
// соединения приходит за доли секунды. Срок ответа connect и setServer (60 с)
// обязан пережить и подъём с двумя попытками по 15 с, и этот доспрос.
const srokUtochneniya = 10 * time.Second

// utochnitPrichinu спрашивает причину у ядра замера, когда проба живого ядра
// получила безымянный 503, а в журнале про этот выход ничего узнаваемого нет.
//
// Найдено приёмкой 1.9.3 в госте 02.10.2026: живое переключение на сервер с
// просроченным сертификатом падало за 0,1 с и тут же откатывалось. Через новый
// сервер не успевало пройти ни одного соединения человека, ядру не о чем было
// написать, и человек снова читал «сервер не принял ключ». Ядро замера звонит
// серверу своим исходящим srv-<id> и называет причину строкой с этим тегом, не
// дожидаясь чужого трафика.
//
// Зовётся, только когда проба шла через один известный сервер: в «авто» 503
// группы не говорит, какой из серверов отказал.
func (s *Sluzhba) utochnitPrichinu(ctx context.Context, err error, id string) error {
	if estOtkazZashchity(err) || !errors.Is(err, yadra.ErrServerOtvergKlyuchi) {
		return err
	}
	ctx, otm := context.WithTimeout(ctx, srokUtochneniya)
	defer otm()
	p, est := s.prichinaYadraZamera(ctx, id)
	if !est {
		return err
	}
	log.Printf("ядро замера назвало причину отказа сервера %s: %s", id, p.Korotko())
	return &otkazZashchity{prichina: p}
}

// prichinaYadraZamera меряет один сервер ядром замера и отдаёт причину, которую
// ядро написало про его исходящий. Ничего, если сервер ответил: тогда 503
// живого ядра был не про защищённое соединение.
func (s *Sluzhba) prichinaYadraZamera(ctx context.Context, id string) (sboi.PrichinaYadra, bool) {
	// Те же ворота, что у пинга списка: конфиг ядра замера лежит одним файлом.
	select {
	case s.vorotaPinga <- struct{}{}:
		defer func() { <-s.vorotaPinga }()
	case <-ctx.Done():
		log.Printf("причина отказа сервера %s не спрошена: идёт замер пинга", id)
		return sboi.NeNazvana, false
	}
	srv, err := s.serverPoId(id)
	if err != nil {
		log.Printf("причина отказа сервера %s не спрошена: %v", id, err)
		return sboi.NeNazvana, false
	}
	nachalo := time.Now()
	y, err := s.yadroZamera(ctx)
	if err != nil {
		log.Printf("причина отказа сервера %s не спрошена: %v", id, err)
		return sboi.NeNazvana, false
	}
	z := s.pingOdnogo(ctx, srv, y)
	// Жалобы спрашиваются после остановки: она ждёт, пока вывод ядра дочитан
	// (см. measureDelays).
	y.ostanovit()
	if z.PingMs != nil || y.isklyucheny[id] {
		return sboi.NeNazvana, false
	}
	zh, est := s.zhalobaVyhoda(y.konfig, genkonfig.TegKandidata(id), nachalo)
	if !est {
		return sboi.NeNazvana, false
	}
	return zh.Prichina, true
}

// zhalobyNePrezhe это граница для проб, у которых своего начала нет
// (наблюдатель, проверка сети): последние srokZhalobyYadra, но не раньше
// последней смены выбора.
func (s *Sluzhba) zhalobyNePrezhe() time.Time {
	granica := time.Now().Add(-srokZhalobyYadra)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vyborSmenyon.After(granica) {
		return s.vyborSmenyon
	}
	return granica
}

// otmetitSmenuVybora зовётся, когда группа vybor могла начать вести трафик
// через другой сервер, не перезапуская ядра.
func (s *Sluzhba) otmetitSmenuVybora(kogda time.Time) {
	s.mu.Lock()
	s.vyborSmenyon = kogda
	s.mu.Unlock()
}
