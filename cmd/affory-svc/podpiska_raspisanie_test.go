package main

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Находка 35. Задал подписку, жмёшь подключить, получаешь «серверов нет».
//
// Список наполняла только отдельная команда refreshSubscription, о которой
// человек знать не обязан. На чистом стенде это был первый же отказ.
func TestSetSubscriptionSrazuNapolnyaetSpisok(t *testing.T) {
	s := podstavnaya(t, nil)
	ctx := kanal.SDopuskom(context.Background(), kanal.Dopusk{Admin: true})

	o := s.Obrabotat(ctx, protokol.Kadr{Id: 1, Imya: "setSubscription",
		Telo: json.RawMessage(`{"adres":"https://panel.example/zhivaya"}`)})
	if o.Oshib != nil {
		t.Fatalf("подписка не принята: %+v", o.Oshib)
	}

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	for _, srv := range n.Servery {
		if srv.Id == vtoroyServer().Id {
			return
		}
	}
	t.Fatalf("список не наполнен: %d серверов, сервера из подписки нет", len(n.Servery))
}

// КОНТРОЛЬ к находке 35: недоступная подписка обязана СОХРАНИТЬСЯ.
//
// Иначе лечение хуже болезни: человек вводит верный адрес в самолёте, получает
// отказ и остаётся вообще без подписки, хотя ввёл всё правильно.
func TestSetSubscriptionSohranyaetsyaDazheKogdaPanelMolchit(t *testing.T) {
	s := podstavnaya(t, nil)
	ctx := kanal.SDopuskom(context.Background(), kanal.Dopusk{Admin: true})

	o := s.Obrabotat(ctx, protokol.Kadr{Id: 1, Imya: "setSubscription",
		Telo: json.RawMessage(`{"adres":"https://panel.example/nedostupno"}`)})
	if o.Oshib == nil {
		t.Fatal("отказ панели не назван вслух: человек считает, что список наполнен")
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.Podpiska == "" {
		t.Fatal("адрес подписки потерян: верный ввод пропал из-за временной сети")
	}
}

// chasyProby это часы и ожидание разом: расписание в реальном времени
// проверить нельзя, а без подмены ожидания тест ждал бы двенадцать часов.
type chasyProby struct {
	mu     sync.Mutex
	teper  time.Time
	zhdali []time.Duration
	shagov int
	stop   int
}

func (c *chasyProby) seychas() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.teper
}

// zhdat двигает часы вперёд ровно на запрошенное, записывает запрос и после
// stop шагов возвращает false, как это делает отмена контекста.
func (c *chasyProby) zhdat(_ context.Context, d time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.zhdali = append(c.zhdali, d)
	c.teper = c.teper.Add(d)
	c.shagov++
	return c.shagov < c.stop
}

func (c *chasyProby) zaprosy() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.zhdali...)
}

// raspisanieProby собирает службу с подпиской, поддельными часами и счётчиком
// загрузок.
func raspisanieProby(t *testing.T, obnovlena *time.Time, stop int) (*Sluzhba, *chasyProby, func() int) {
	t.Helper()
	s := podstavnaya(t, nil)
	nach := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ch := &chasyProby{teper: nach, stop: stop}
	s.seychas = ch.seychas
	s.zhdat = ch.zhdat
	s.prochitat = func() (sostoyanie.SostoyanieFayla, error) {
		return sostoyanie.SostoyanieFayla{PodpiskaObnovlena: obnovlena}, nil
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	n.Podpiska = "https://panel.example/zhivaya"
	if err := s.zapisatNabor(n); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	zagruzok := 0
	prezhnyaya := s.zagruzitPodpisku
	s.zagruzitPodpisku = func(ctx context.Context, adres string) (ssylki.Razbor, error) {
		mu.Lock()
		zagruzok++
		skolko := zagruzok
		mu.Unlock()
		// Сторож против холостого цикла. Без него снятие отступа после неудачи
		// даёт не красный тест, а вечный цикл, то есть худшую форму красного.
		if skolko > 5 {
			t.Fatalf("холостой цикл: %d загрузок подряд без ожидания", skolko)
		}
		return prezhnyaya(ctx, adres)
	}
	return s, ch, func() int { mu.Lock(); defer mu.Unlock(); return zagruzok }
}

// Находка 15. Расписания обновления подписки не существовало: ни таймера, ни
// загрузки при старте. Комментарий про пять повторов «потому что служба
// стартует раньше сети» описывал загрузку, которой не было.
func TestRaspisaniyeGruzitPodpiskuPriStarte(t *testing.T) {
	s, ch, zagruzok := raspisanieProby(t, nil, 1)

	s.raspisaniePodpiski(context.Background())

	if zagruzok() != 1 {
		t.Fatalf("загрузок %d, ожидалась одна: подписка при старте не тянется", zagruzok())
	}
	if z := ch.zaprosy(); len(z) != 1 || z[0] != ssylki.PeriodObnovleniya {
		t.Fatalf("следующий срок %v, ожидался %v", z, ssylki.PeriodObnovleniya)
	}
}

// Отметка успешного обновления обязана лечь на диск. Без неё период
// отсчитывается от старта службы, и после каждой перезагрузки токен подписки
// светится в сети заново.
func TestRaspisaniyePishetOtmetkuObnovleniya(t *testing.T) {
	s, ch, _ := raspisanieProby(t, nil, 1)
	var leglo *time.Time
	var mu sync.Mutex
	s.zapisat = func(f sostoyanie.SostoyanieFayla) error {
		mu.Lock()
		defer mu.Unlock()
		if f.PodpiskaObnovlena != nil {
			leglo = f.PodpiskaObnovlena
		}
		return nil
	}

	s.raspisaniePodpiski(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if leglo == nil {
		t.Fatal("отметка не записана: период поедет от старта службы")
	}
	if !leglo.Equal(ch.seychas()) && leglo.After(ch.seychas()) {
		t.Fatalf("отметка %v позже текущего времени %v", leglo, ch.seychas())
	}
}

// КОНТРОЛЬ: свежая отметка на диске обязана отложить загрузку, а не запустить
// её заново. Иначе предыдущие два теста зелёные и на службе, которая тянет
// подписку на каждый чих.
func TestRaspisaniyeNeGruzitSvezhuyuPodpisku(t *testing.T) {
	chas := time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)
	s, ch, zagruzok := raspisanieProby(t, &chas, 1)

	s.raspisaniePodpiski(context.Background())

	if zagruzok() != 0 {
		t.Fatalf("загрузок %d, ожидалось ноль: свежая подписка тянется заново", zagruzok())
	}
	z := ch.zaprosy()
	if len(z) != 1 || z[0] != 11*time.Hour {
		t.Fatalf("ожидание %v, ожидалось 11h (12h минус час с прошлого обновления)", z)
	}
}

// Неудача не имеет права превратиться в холостой цикл: отметка на диске не
// двигается, поэтому без собственного отступа следующий заход начинается
// немедленно и съедает процессор вместе с лимитом панели.
func TestRaspisaniyeOtstupaetPosleNeudachi(t *testing.T) {
	s, ch, zagruzok := raspisanieProby(t, nil, 1)
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	n.Podpiska = "https://panel.example/nedostupno"
	if err := s.zapisatNabor(n); err != nil {
		t.Fatal(err)
	}

	s.raspisaniePodpiski(context.Background())

	if zagruzok() != 1 {
		t.Fatalf("загрузок %d, ожидалась одна до отступа", zagruzok())
	}
	z := ch.zaprosy()
	if len(z) != 1 || z[0] != otstupPodpiski {
		t.Fatalf("после неудачи ждём %v, ожидался отступ %v", z, otstupPodpiski)
	}
}

// Расписание обязано умирать вместе со службой, причём Zavershit обязан его
// ДОЖДАТЬСЯ, а не только отменить. Горутина без такого владельца это класс F
// ворот Ш8.
//
// Задержка на выходе не украшение: без неё тест зелёный и на службе, которая
// отменяет и уходит, не дожидаясь. Именно так горутина восстановления
// переживала процесс и писала общие переменные уже следующего теста.
func TestRaspisaniyeUmiraetSoSluzhboy(t *testing.T) {
	s, _, _ := raspisanieProby(t, nil, 1)
	var vyshlo atomic.Bool
	// Первый заход расписания не ждёт вовсе: он и есть загрузка при старте.
	// Без этой отсечки отмена успевала раньше входа в ожидание, и тест был
	// зелёным или красным в зависимости от порядка тестов в пакете.
	voshli := make(chan struct{})
	odin := sync.Once{}
	s.zhdat = func(ctx context.Context, _ time.Duration) bool {
		odin.Do(func() { close(voshli) })
		<-ctx.Done()
		time.Sleep(300 * time.Millisecond)
		vyshlo.Store(true)
		return false
	}
	s.ZapustitRaspisanie()
	select {
	case <-voshli:
	case <-time.After(10 * time.Second):
		t.Fatal("расписание не дошло до ожидания за 10 секунд")
	}

	gotovo := make(chan struct{})
	go func() { s.Zavershit(); close(gotovo) }()
	select {
	case <-gotovo:
	case <-time.After(10 * time.Second):
		t.Fatal("Zavershit не дождался расписания: горутина переживает службу")
	}
	if !vyshlo.Load() {
		t.Fatal("Zavershit вернулся раньше, чем расписание вышло: отмена без ожидания")
	}
}

// Класс F: сброс состояния стирал файл целиком, а у отметки обновления
// появился ВТОРОЙ читатель, расписание. Отключение туннеля не имеет права
// стоить лишнего похода за подпиской: токен светится в сети чаще, чем нужно.
func TestSbrosSostoyaniyaHranitOtmetkuPodpiski(t *testing.T) {
	s := podstavnaya(t, nil)
	var leglo sostoyanie.SostoyanieFayla
	s.zapisat = func(f sostoyanie.SostoyanieFayla) error { leglo = f; return nil }

	otmetka := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.PodpiskaObnovlena = &otmetka })
	if err := s.sbrositSost(); err != nil {
		t.Fatal(err)
	}

	if leglo.PodpiskaObnovlena == nil {
		t.Fatal("отметка обновления стёрта отключением туннеля: подписка потянется заново")
	}
	if !leglo.PodpiskaObnovlena.Equal(otmetka) {
		t.Fatalf("отметка поехала: %v вместо %v", leglo.PodpiskaObnovlena, otmetka)
	}
	// КОНТРОЛЬ: данные ядра сброс обязан стереть, иначе тест защищает не то.
	// Устаревший индекс адаптера опаснее пустого.
	if leglo.IndeksTun != 0 || leglo.PortClash != 0 {
		t.Fatal("сброс перестал стирать данные ядра: тест защитил лишнее")
	}
}

// Расписание ходит по таймеру, без человека и без прав. Отказ здесь остановил
// бы обновление подписки на всё время подключения, и узнать об этом было бы
// неоткуда: команда никем не вызывалась, ошибку никто не читает.
func TestRaspisaniePodpiskiUderzhivaetZhivyh(t *testing.T) {
	s := podstavnaya(t, nil)
	// IzPodpiski: true ОБЯЗАТЕЛЬНО. Slit удаляет только серверы, помеченные как
	// пришедшие из публикации; ручные он удерживает сам, и без этой пометки
	// тест зелен с нуля и не судит ничего.
	izPodpiski := serverProby()
	izPodpiski.IzPodpiski = true
	n := Nabor{Servery: []protokol.Server{izPodpiski}, Vybran: "nl", Podpiska: "https://x/y"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	// Публикация БЕЗ serverProby(): узел из неё пропал, а он сейчас в ядре.
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		return ssylki.Razbor{Servery: []protokol.Server{vtoroyServer()}}, nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.obnovitPodpisku(context.Background()); err != nil {
		t.Fatalf("обновление подписки упало вместо удержания: %v", err)
	}
	if len(n.Servery) != 2 {
		t.Fatalf("в наборе %d серверов: пропавший из публикации обязан удержаться, пока ядро живо", len(n.Servery))
	}
}
