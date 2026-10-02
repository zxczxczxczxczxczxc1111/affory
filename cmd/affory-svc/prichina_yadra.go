package main

import (
	"context"
	"errors"
	"time"

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
