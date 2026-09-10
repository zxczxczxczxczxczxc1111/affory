package main

import (
	"context"
	"errors"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
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
	// Адрес читается ВНЕ замка набора: следом идёт поход в сеть, и держать
	// набор запертым всё время загрузки значит подвесить любую команду
	// человека на время, которое задаёт чужая панель.
	n, err := s.nabor()
	if err != nil {
		return ssylki.Razbor{}, 0, oshibkaNabora{err}
	}
	if n.Podpiska == "" {
		return ssylki.Razbor{}, 0, errPodpiskaNeZadana
	}
	r, err := s.zagruzitPodpisku(ctx, n.Podpiska)
	if err != nil {
		return r, 0, err
	}
	// Слияние идёт по СВЕЖЕМУ набору, прочитанному под замком: пока панель
	// отвечала, человек успевает добавить сервер руками, и слияние по набору
	// из первой строки стёрло бы его добавление.
	var uderzhany []string
	serverov := 0
	if err := s.pravitNabor(func(n *Nabor) error {
		prezhnie := n.Servery
		n.Servery = ssylki.Slit(prezhnie, r.Servery)
		// Пропавшие из публикации удерживаются, пока ядро их держит. Отказ здесь
		// остановил бы обновление подписки на всё время подключения, а новые узлы из
		// публикации при этом не доехали бы вовсе.
		uderzhany = s.uderzhatZhivyh(prezhnie, n)
		serverov = len(n.Servery)
		return nil
	}); err != nil {
		return r, 0, oshibkaSohraneniya{err}
	}
	if len(uderzhany) > 0 {
		// Событие, а не отказ: следующее обновление при опущенном ядре уберёт их
		// само, и это надо показать, а не спрятать.
		s.izvestit("serversRetained", map[string]any{"imena": uderzhany})
	}
	teper := s.seychas()
	s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.PodpiskaObnovlena = &teper })
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
		if _, _, err := s.obnovitPodpisku(ctx); err != nil {
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

// uderzhatZhivyh возвращает в набор те пропавшие серверы, которых ядро ещё
// держит кандидатами, и отдаёт их имена.
//
// Метод, а не свободная функция: решение зависит от dostupKKlash(), то есть от
// состояния службы. При мёртвом ядре не делает ничего, и это не оптимизация:
// удерживать нечего, ядро список уже забыло.
//
// Расписанию нельзя отвечать отказом, как человеку: команду никто не вызывал,
// ошибку никто не прочитает, и подписка просто перестала бы обновляться на всё
// время подключения.
func (s *Sluzhba) uderzhatZhivyh(prezhnie []protokol.Server, n *Nabor) []string {
	if adres, _ := s.dostupKKlash(); adres == "" {
		return nil
	}
	est := make(map[string]bool, len(n.Servery))
	for _, srv := range n.Servery {
		est[srv.Id] = true
	}
	var imena []string
	for _, srv := range prezhnie {
		if !est[srv.Id] {
			n.Servery = append(n.Servery, srv)
			imena = append(imena, srv.Id)
		}
	}
	return imena
}
