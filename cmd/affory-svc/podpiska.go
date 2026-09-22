package main

import (
	"context"
	"errors"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Отступ после неудачи. Отметка успеха на диске при неудаче не двигается,
// поэтому без собственного отступа следующий заход начинался бы немедленно:
// холостой цикл на весь процессор и лимит панели заодно.
const otstupPodpiski = 30 * time.Minute

var errPodpiskaNeZadana = errors.New("подписка не задана")

// oshibkaNabora и oshibkaSohraneniya отделяют «не смогли прочитать секреты» и
// «не смогли записать» от «панель не ответила». Без разделения все три отказа
// уезжали бы в subscription-unreachable, и человек чинил бы сеть вместо диска.
type oshibkaNabora struct{ err error }

func (o oshibkaNabora) Error() string { return o.err.Error() }
func (o oshibkaNabora) Unwrap() error { return o.err }

type oshibkaSohraneniya struct{ err error }

func (o oshibkaSohraneniya) Error() string { return o.err.Error() }
func (o oshibkaSohraneniya) Unwrap() error { return o.err }

// obnovitPodpisku это ЕДИНСТВЕННЫЙ путь обновления списка из подписки.
//
// Сюда ходят все трое: команда refreshSubscription, задание адреса и
// расписание. Три копии одного и того же неизбежно разъехались бы, а разъезд
// был бы тихим: список наполнялся бы одним путём и не наполнялся другим, ровно
// как в находке 35.
func (s *Sluzhba) obnovitPodpisku(ctx context.Context) (ssylki.Razbor, int, error) {
	n, err := s.nabor()
	if err != nil {
		return ssylki.Razbor{}, 0, oshibkaNabora{err}
	}
	return s.obnovitPodpiskuPoId(ctx, n.Aktivnaya)
}

// Ответ сети принадлежит исходной подписке, даже если за время загрузки
// человек переключился на другую. Удалённую запись ответ не воскрешает.
func (s *Sluzhba) obnovitPodpiskuPoId(ctx context.Context, id string) (ssylki.Razbor, int, error) {
	n, err := s.nabor()
	if err != nil {
		return ssylki.Razbor{}, 0, oshibkaNabora{err}
	}
	z := n.zapisPodpiski(id)
	if z == nil {
		return ssylki.Razbor{}, 0, errPodpiskaNeZadana
	}
	adres := z.Adres
	r, err := s.zagruzitPodpisku(ctx, adres)
	if err != nil {
		s.otmetitOtkazPodpiski(id, err)
		return r, 0, err
	}
	serverov := 0
	aktivnaya := false
	teper := s.seychas()
	if err := s.pravitNabor(func(n *Nabor) error {
		z := n.zapisPodpiski(id)
		if z == nil || z.Adres != adres {
			return errors.New("подписка удалена во время обновления")
		}
		aktivnaya = n.Aktivnaya == id
		if aktivnaya {
			n.Servery = ssylki.Slit(n.Servery, r.Servery)
			serverov = len(n.Servery)
		} else {
			z.Servery = ssylki.Slit(nil, r.Servery)
			serverov = len(z.Servery)
		}
		n.OtmetitObnovlenie(id, teper)
		return nil
	}); err != nil {
		return r, 0, oshibkaSohraneniya{err}
	}
	if aktivnaya {
		s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.PodpiskaObnovlena = &teper })
	}
	return r, serverov, nil
}

// ZapustitRaspisanie заводит расписание с владельцем, который умеет и отменить
// его, и дождаться. Горутина без такого владельца это класс F ворот Ш8.
// Add и Done в ОДНОМ месте, у владельца. Пока Done жил внутри цикла, прямой
// вызов цикла из теста уводил счётчик в минус, и Wait в уборке висел навсегда:
// красный превращался в зависание, то есть в худшую форму красного.
func (s *Sluzhba) ZapustitRaspisanie() {
	s.fon.Add(1)
	go func() {
		defer s.fon.Done()
		s.raspisaniePodpiski(s.fonCtx)
	}()
	s.fon.Add(1)
	go func() {
		defer s.fon.Done()
		s.raspisanieObnovleniy(s.fonCtx)
	}()
	// Подробный журнал крутится ВСЕГДА и молчит, пока настройка выключена.
	// Заводить и гасить горутину по щелчку настройки значит завести гонку там,
	// где такт стоит одного сравнения.
	s.fon.Add(1)
	go func() {
		defer s.fon.Done()
		s.sobiratDiagnostiku(s.fonCtx)
	}()
}

// raspisaniePodpiski тянет подписку при старте и дальше раз в период.
//
// Отметка берётся С ДИСКА: без неё период отсчитывается от старта службы, то
// есть после каждой перезагрузки токен подписки светится в сети заново.
func (s *Sluzhba) raspisaniePodpiski(ctx context.Context) {
	var poslednyaya time.Time
	if f, err := s.prochitat(); err == nil && f.PodpiskaObnovlena != nil {
		poslednyaya = *f.PodpiskaObnovlena
	}
	var povtor time.Time

	for {
		if pauza := s.srokObnovleniya(poslednyaya, povtor).Sub(s.seychas()); pauza > 0 {
			if !s.zhdat(ctx, pauza) {
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		// Обход ВСЕХ подписок, а не только активной: запасная, за которой не
		// ходят, это адрес, а не подписка, и переключение на неё означало бы
		// поход в сеть ровно в ту минуту, когда человеку нужен туннель.
		// Отступ считается по отказу АКТИВНОЙ: молчащая запасная панель не
		// повод долбить остальные чаще положенного.
		if _, _, err := s.obnovitVsePodpiski(ctx); err != nil {
			povtor = s.seychas().Add(otstupPodpiski)
			continue
		}
		poslednyaya, povtor = s.seychas(), time.Time{}
	}
}

// srokObnovleniya отвечает на один вопрос: когда следующий заход.
func (s *Sluzhba) srokObnovleniya(poslednyaya, povtor time.Time) time.Time {
	if !povtor.IsZero() {
		return povtor
	}
	if poslednyaya.IsZero() {
		// Ни одного успешного обновления: тянем сразу. Это и есть загрузка при
		// старте, которой не существовало.
		return s.seychas()
	}
	return poslednyaya.Add(ssylki.PeriodObnovleniya)
}

func zhdatPoChasam(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
