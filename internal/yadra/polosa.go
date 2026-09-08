package yadra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"
)

// Замер полосы скачиванием. Задача 8, часть В.
//
// Число нужно ровно затем, чтобы объявлять его hysteria2: у него объявление
// полосы это единственный переключатель Brutal, а Brutal шлёт РОВНО с
// объявленной скоростью и за завышение наказывает. Замерено на стенде:
// завышение на 30% стоило 10% полосы и 30% задержки, восемь провалов против
// нуля. Значит источник числа обязан быть измерением, а не полем ввода.
//
// Ноль здесь нигде не превращается в результат. Мишень, не отдавшая ни байта,
// это ОТКАЗ: объявленный ноль включил бы Brutal с нулевой оценкой канала.

var ErrPolosaNeIzmerena = errors.New("полоса не измерена")

// Границы входа. Не вкусовщина: замер идёт по каналу человека и тратит его
// трафик, а на мобильном тарифе это заметно.
const (
	PotokovMax = 16
	SrokMax    = 30 * time.Second
)

type VhodPolosy struct {
	// Adres мишени. Задаётся ЯВНО и умолчания не имеет: клиент не ходит
	// самовольно на чужой хост и не тратит трафик без спроса.
	Adres string
	// Proksi это адрес нашего же входящего mixed, «хост:порт». Пусто значит
	// прямой путь, и это законно: так меряется канал БЕЗ туннеля, то есть
	// потолок, с которым сравнивают.
	Proksi  string
	Potokov int
	Srok    time.Duration
}

type ItogPolosy struct {
	Mbit  float64       `json:"mbit"`
	Bayt  int64         `json:"bayt"`
	Srok  time.Duration `json:"-"`
	Potok int           `json:"potokov"`
}

func (v VhodPolosy) proverit() error {
	if v.Adres == "" {
		return fmt.Errorf("%w: мишень не задана", ErrPolosaNeIzmerena)
	}
	u, err := url.Parse(v.Adres)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%w: мишень должна быть http или https", ErrPolosaNeIzmerena)
	}
	if v.Potokov < 1 || v.Potokov > PotokovMax {
		return fmt.Errorf("%w: потоков %d вне 1..%d", ErrPolosaNeIzmerena, v.Potokov, PotokovMax)
	}
	if v.Srok <= 0 || v.Srok > SrokMax {
		return fmt.Errorf("%w: срок %v вне (0, %v]", ErrPolosaNeIzmerena, v.Srok, SrokMax)
	}
	return nil
}

// Размер порции заливки. Тело с ИЗВЕСТНОЙ длиной, а не бесконечный chunked:
// мишень, требующая Content-Length, отвергает chunked целиком. Проверено на
// живой мишени 07.09.2026: speed.cloudflare.com/__up принял мегабайт именно с
// длиной.
const razmerPorcii = 8 << 20

// schyot общий на все потоки замера.
//
// Кода ответа мало для диагноза, но без него отказ мишени неотличим от мёртвого
// канала, а это разные починки: первое чинится другой мишенью, второе сетью.
type schyot struct {
	bayt      atomic.Int64
	kodOtveta atomic.Int64
}

// otvergla запоминает ПЕРВЫЙ неуспешный код: последующие это уже следствия.
func (s *schyot) otvergla(kod int) { s.kodOtveta.CompareAndSwap(0, int64(kod)) }

type potok func(ctx context.Context, klient *http.Client, adres string, s *schyot)

type vidZamera struct {
	rabota   potok
	pustoy   string
	otvergla string
}

// ZamerPolosy качает мишень в несколько потоков и возвращает среднее за срок.
func ZamerPolosy(ctx context.Context, v VhodPolosy) (ItogPolosy, error) {
	return zamer(ctx, v, vidZamera{
		rabota:   kachat,
		pustoy:   "мишень не отдала ни байта",
		otvergla: "мишень не отдала байт, ответив",
	})
}

// ZamerOtdachi заливает на мишень и меряет обратное направление.
//
// Второе число нужно затем же, зачем первое: hysteria2 объявляет полосу в ОБЕ
// стороны, и Brutal наказывает за завышение любой из них. Ставить отдаче число
// приёма значит врать: у домашнего канала отдача обычно втрое ниже.
//
// Мишень заливки это не то же, что мишень скачивания. Файл на CDN отдаёт байты
// всякому, а принимает их редкий сервер, и на POST он ответит 405. Такой ответ
// обязан быть ОТКАЗОМ: ноль, поданный как измерение, уехал бы в объявление, а
// объявленный ноль это Brutal с нулевой оценкой канала.
func ZamerOtdachi(ctx context.Context, v VhodPolosy) (ItogPolosy, error) {
	return zamer(ctx, v, vidZamera{
		rabota:   zalivat,
		pustoy:   "мишень не приняла ни байта",
		otvergla: "мишень не принимает заливку, ответив",
	})
}

// zamer это общий каркас обоих направлений: срок, потоки, отмена, счёт.
//
// Проверка входа стоит ДО первого запроса намеренно: замер с нулём потоков
// «померил» бы ноль и выдал его за полосу канала, а отличить такой ноль от
// честного измерения мёртвой мишени потом нечем.
func zamer(ctx context.Context, v VhodPolosy, vid vidZamera) (ItogPolosy, error) {
	if err := v.proverit(); err != nil {
		return ItogPolosy{}, err
	}
	klient, err := klientPolosy(v.Proksi)
	if err != nil {
		return ItogPolosy{}, err
	}

	// Свой срок поверх чужого контекста: отмена человеком должна работать, но
	// и без неё замер обязан кончиться сам. Родительский держим отдельно, чтобы
	// потом отличить «срок вышел» от «человек отменил».
	roditel := ctx
	ctx, stop := context.WithTimeout(ctx, v.Srok)
	defer stop()

	// Потоки не дожидаются: они и так кончаются по контексту, а ждать их значит
	// добавить к замеру время на закрытие соединений. Счётчик атомарный, и
	// опоздавшая запись в него после снятия показаний уже никого не касается.
	var s schyot
	for i := 0; i < v.Potokov; i++ {
		go vid.rabota(ctx, klient, v.Adres, &s)
	}
	nachalo := time.Now()
	<-ctx.Done()
	proshlo := time.Since(nachalo)

	// Отмена это отказ ВСЕГДА, даже когда байты уже насчитаны. Человек нажал
	// «стоп», а получил бы число за неполный срок, поданное как полоса его
	// канала. Оно ушло бы в объявление hysteria2, где заниженное объявление
	// работает ограничителем, а завышенное рвёт соединение.
	if roditel.Err() != nil {
		return ItogPolosy{}, fmt.Errorf("%w: замер отменён", ErrPolosaNeIzmerena)
	}
	vsego := s.bayt.Load()
	if vsego == 0 {
		if kod := s.kodOtveta.Load(); kod != 0 {
			return ItogPolosy{}, fmt.Errorf("%w: %s %d", ErrPolosaNeIzmerena, vid.otvergla, kod)
		}
		return ItogPolosy{}, fmt.Errorf("%w: %s", ErrPolosaNeIzmerena, vid.pustoy)
	}
	if proshlo <= 0 {
		return ItogPolosy{}, fmt.Errorf("%w: срок вышел нулевым", ErrPolosaNeIzmerena)
	}
	return ItogPolosy{
		Mbit:  float64(vsego) * 8 / proshlo.Seconds() / 1e6,
		Bayt:  vsego,
		Srok:  proshlo,
		Potok: v.Potokov,
	}, nil
}

func kachat(ctx context.Context, klient *http.Client, adres string, s *schyot) {
	bufer := make([]byte, 64*1024)
	for ctx.Err() == nil {
		zapros, err := http.NewRequestWithContext(ctx, http.MethodGet, adres, nil)
		if err != nil {
			return
		}
		otvet, err := klient.Do(zapros)
		if err != nil {
			return
		}
		// Код ответа проверяется ДО чтения. Страница «404» это тоже байты, и
		// без проверки мишень, отдающая ошибку, показала бы бодрую полосу.
		if otvet.StatusCode != http.StatusOK && otvet.StatusCode != http.StatusPartialContent {
			s.otvergla(otvet.StatusCode)
			otvet.Body.Close()
			return
		}
		for {
			n, err := otvet.Body.Read(bufer)
			if n > 0 {
				s.bayt.Add(int64(n))
			}
			if err != nil {
				break
			}
			if ctx.Err() != nil {
				break
			}
		}
		otvet.Body.Close()
	}
}

// zalivat шлёт порции, пока не кончится срок.
//
// Байты порции идут в общий счёт ТОЛЬКО после успешного ответа мишени. Иначе
// отвергнутая заливка насчитала бы полную порцию и выдала бы 405 за измерение.
// Побочно это занижает результат на недосланную последнюю порцию, и занижение
// здесь безопасно: заниженное объявление работает ограничителем, завышенное
// рвёт соединение.
func zalivat(ctx context.Context, klient *http.Client, adres string, s *schyot) {
	for ctx.Err() == nil {
		telo := &nuli{ostatok: razmerPorcii}
		zapros, err := http.NewRequestWithContext(ctx, http.MethodPost, adres, telo)
		if err != nil {
			return
		}
		// Длина ставится руками: для произвольного Reader её никто не выведет,
		// и запрос ушёл бы chunked.
		zapros.ContentLength = razmerPorcii
		zapros.Header.Set("Content-Type", "application/octet-stream")
		otvet, err := klient.Do(zapros)
		if err != nil {
			return
		}
		// Тело ответа дочитывается до конца ради повторного использования
		// соединения: брошенное тело стоит нового рукопожатия на каждой порции.
		io.Copy(io.Discard, otvet.Body)
		otvet.Body.Close()
		if otvet.StatusCode < 200 || otvet.StatusCode > 299 {
			s.otvergla(otvet.StatusCode)
			return
		}
		s.bayt.Add(razmerPorcii - telo.ostatok)
	}
}

// nuli это источник тела заданной длины.
//
// Тело нужно любое, лишь бы известной длины: мерится канал, а не содержимое.
// Сжатие отключено транспортом, поэтому нули не схлопнутся по дороге.
type nuli struct{ ostatok int64 }

func (n *nuli) Read(p []byte) (int, error) {
	if n.ostatok <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > n.ostatok {
		p = p[:n.ostatok]
	}
	for i := range p {
		p[i] = 0
	}
	n.ostatok -= int64(len(p))
	return len(p), nil
}

func klientPolosy(proksi string) (*http.Client, error) {
	tr := &http.Transport{
		// Соединений на хост столько же, сколько потоков максимум: умолчание
		// в две штуки превратило бы шестнадцать потоков в два.
		MaxIdleConnsPerHost: PotokovMax,
		MaxConnsPerHost:     PotokovMax,
		DisableCompression:  true,
	}
	if proksi != "" {
		u, err := url.Parse("http://" + proksi)
		if err != nil {
			return nil, fmt.Errorf("%w: адрес прокси не разбирается", ErrPolosaNeIzmerena)
		}
		tr.Proxy = http.ProxyURL(u)
	}
	// Срока у клиента нет намеренно: замер длится ровно столько, сколько велел
	// контекст, а Timeout оборвал бы поток на середине и потерял бы счёт.
	return &http.Client{Transport: tr}, nil
}
