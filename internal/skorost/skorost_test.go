package skorost

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFallbackRequiresBothDirectionsFromSameProvider(t *testing.T) {
	var failedUploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if r.URL.Path == "/bad" {
				failedUploads.Add(1)
				w.WriteHeader(405)
				return
			}
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(200)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(make([]byte, 4096))
	}))
	defer server.Close()
	runner := Runner{Providers: []Provider{{ID: "bad", Name: "Bad", Download: server.URL, Upload: server.URL + "/bad"}, {ID: "good", Name: "Good", Download: server.URL, Upload: server.URL}}, Duration: time.Second, Limit: 4096, Chunk: 4096, MinBytes: 1024, Threads: 1}
	result := runner.Run(context.Background(), server.Client(), "bad", nil)
	if result.Provider != "good" || result.Download <= 0 || result.Upload <= 0 || len(result.Attempts) != 2 || failedUploads.Load() != 1 {
		t.Fatalf("mixed or missing result: %+v", result)
	}
}

func TestHTMLAndRejectedUploadNeverBecomeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(make([]byte, 4096))
	}))
	defer server.Close()
	runner := Runner{Providers: []Provider{{ID: "html", Name: "HTML", Download: server.URL, Upload: server.URL}}, Duration: time.Second, Limit: 4096, Chunk: 4096, MinBytes: 1024, Threads: 1}
	result := runner.Run(context.Background(), server.Client(), "", nil)
	if result.Download != 0 || result.Upload != 0 || result.Error == "" {
		t.Fatalf("HTML measured: %+v", result)
	}
}

func TestCancellationStopsRequestsAndDoesNotTryAnotherProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); cancel(); <-r.Context().Done() }))
	defer server.Close()
	runner := Runner{Providers: []Provider{{ID: "one", Download: server.URL}, {ID: "two", Download: server.URL}}, Duration: time.Second, Limit: 4096, Chunk: 4096, MinBytes: 1024, Threads: 1}
	result := runner.Run(ctx, server.Client(), "", nil)
	if result.Download != 0 || result.Upload != 0 || requests.Load() != 1 || ctx.Err() == nil {
		t.Fatalf("cancellation failed: %+v", result)
	}
}

// Разгон в число не входит: за первые 2 секунды из 10 прошло 5 МБ, за
// остальные 8 ещё 80. Прежний расчёт по всей фазе дал бы 68 Мбит/с вместо 80.
func TestSkorostSchitaetsyaPoOknuPosleRazgona(t *testing.T) {
	nachalo := time.Unix(1000, 0)
	okno := nachalo.Add(2 * time.Second)
	konec := nachalo.Add(10 * time.Second)
	const mb = 1_000_000
	if got := skorostOkna(85*mb, nachalo, okno, 5*mb, konec); got < 79.99 || got > 80.01 {
		t.Fatalf("скорость окна %.2f, а должна 80", got)
	}
	// Окна не случилось (фаза кончилась до конца разгона) или оно короче
	// секунды: тогда число по всей фазе, а не по доле секунды.
	if got := skorostOkna(20*mb, nachalo, time.Time{}, 0, nachalo.Add(time.Second)); got < 159.99 || got > 160.01 {
		t.Fatalf("без окна %.2f, а должна 160", got)
	}
	if got := skorostOkna(20*mb, nachalo, konec.Add(-500*time.Millisecond), 19*mb, konec); got < 15.99 || got > 16.01 {
		t.Fatalf("короткое окно %.2f, а должна 16 по всей фазе", got)
	}
}

func TestParametryPoUmolchaniyuProhodyatSvoyuProverku(t *testing.T) {
	r := Default()
	if err := r.proverit(); err != nil {
		t.Fatalf("умолчания не проходят свою же проверку: %+v", r)
	}
	c, err := Client("")
	if err != nil {
		t.Fatal(err)
	}
	// Поток без своего соединения стоит в очереди и тянет число вниз: ровно
	// это прятали прежние 3 соединения на хост.
	if tr := c.Transport.(*http.Transport); tr.MaxConnsPerHost < r.Threads {
		t.Fatalf("соединений на хост %d меньше потоков %d", tr.MaxConnsPerHost, r.Threads)
	}
	// «Автоматически» начинает с Clouvider: LibreSpeed в Хельсинки даже с
	// сервера в дата-центре отдавал 40 Мбит/с и мерил себя, а не канал.
	if r.Providers[0].ID != "clouvider" {
		t.Fatalf("первым стоит %s", r.Providers[0].ID)
	}
	// И площадку выбирает по задержке, а не берёт одну на весь мир.
	if n := len(r.Providers[0].Tochki); n < 2 {
		t.Fatalf("у Clouvider %d площадок, выбирать не из чего", n)
	}
	for _, p := range r.Providers[0].Tochki {
		if p.Name == "" || p.Download == "" || p.Upload == "" || p.Ping == "" {
			t.Fatalf("площадка без адреса: %+v", p)
		}
	}
}

// ploshchadka отвечает как площадка LibreSpeed: пустой ответ на пинг, данные
// на приём, подтверждение на отдачу. Задержка добавляется к каждому пингу.
func ploshchadka(t *testing.T, zaderzhka time.Duration, priyomov *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			time.Sleep(zaderzhka)
			w.WriteHeader(200)
		case "/garbage":
			priyomov.Add(1)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(make([]byte, 4096))
		default:
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(200)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func tochkaIz(imya string, s *httptest.Server) Tochka {
	return Tochka{Name: imya, Download: s.URL + "/garbage", Upload: s.URL + "/up", Ping: s.URL + "/ping"}
}

// Замер идёт с площадки, которая ближе по задержке, даже если в списке она не
// первая: иначе с американского выхода мерился бы путь через океан.
func TestPloshchadkaVybiraetsyaPoZaderzhke(t *testing.T) {
	var sDalney, sBlizhney atomic.Int32
	dalnyaya := ploshchadka(t, 60*time.Millisecond, &sDalney)
	blizhnyaya := ploshchadka(t, 0, &sBlizhney)
	runner := Runner{Providers: []Provider{{ID: "op", Name: "Оператор", Tochki: []Tochka{
		tochkaIz("Дальняя", dalnyaya), tochkaIz("Ближняя", blizhnyaya),
	}}}, Duration: time.Second, Limit: 4096, Chunk: 4096, MinBytes: 1024, Threads: 1}
	var imena []string
	result := runner.Run(context.Background(), &http.Client{}, "", func(p Progress) { imena = append(imena, p.Name) })
	if result.Error != "" || result.Name != "Ближняя" || result.Download <= 0 || result.Upload <= 0 {
		t.Fatalf("замер не с ближней площадки: %+v", result)
	}
	if sDalney.Load() != 0 || sBlizhney.Load() == 0 {
		t.Fatalf("приём шёл с дальней %d раз, с ближней %d", sDalney.Load(), sBlizhney.Load())
	}
	// Окно показывает, откуда мерили, а не имя оператора целиком.
	for _, imya := range imena {
		if imya != "Ближняя" {
			t.Fatalf("в ходе замера показано %q", imya)
		}
	}
}

// Мёртвая площадка не отменяет выбор среди живых и не держит его дольше срока.
func TestMyortvayaPloshchadkaNeMeshaetVyboru(t *testing.T) {
	var priyomov atomic.Int32
	zhivaya := ploshchadka(t, 0, &priyomov)
	myortvaya := httptest.NewServer(http.NotFoundHandler())
	myortvaya.Close()
	nachalo := time.Now()
	t2, err := blizhayshaya(context.Background(), &http.Client{}, []Tochka{tochkaIz("Мёртвая", myortvaya), tochkaIz("Живая", zhivaya)})
	if err != nil || t2.Name != "Живая" {
		t.Fatalf("выбрана %q, ошибка %v", t2.Name, err)
	}
	if proshlo := time.Since(nachalo); proshlo > predelPinga {
		t.Fatalf("выбор занял %v при сроке %v", proshlo, predelPinga)
	}
}

// Ни одна площадка не ответила: это отказ оператора, и замер идёт к
// следующему сервису, а не кончается ошибкой.
func TestBezPloshchadokZamerIdyotKSleduyushchemu(t *testing.T) {
	var priyomov atomic.Int32
	zapasnoy := ploshchadka(t, 0, &priyomov)
	myortvaya := httptest.NewServer(http.NotFoundHandler())
	myortvaya.Close()
	runner := Runner{Providers: []Provider{
		{ID: "op", Name: "Оператор", Tochki: []Tochka{tochkaIz("Мёртвая", myortvaya)}},
		{ID: "zapas", Name: "Запасной", Download: zapasnoy.URL + "/garbage", Upload: zapasnoy.URL + "/up"},
	}, Duration: time.Second, Limit: 4096, Chunk: 4096, MinBytes: 1024, Threads: 1}
	result := runner.Run(context.Background(), &http.Client{}, "", nil)
	if result.Provider != "zapas" || len(result.Attempts) != 2 || result.Attempts[0].Error != "ни одна площадка не ответила" {
		t.Fatalf("без площадок замер не ушёл к запасному: %+v", result)
	}
}

func TestUploadCountsOnlyFullyAcceptedBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	// A server can say OK before reading. It does not get to invent sent bytes.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Runner{Duration: time.Second, Limit: 4096, Chunk: 4096, MinBytes: 1024, Threads: 1}
	if _, err := r.phase(ctx, server.Client(), server.URL, true); err == nil {
		t.Fatal("cancelled upload succeeded")
	}
}
