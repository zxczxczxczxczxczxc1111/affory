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
	// «Автоматически» начинает с Амстердама: LibreSpeed в Хельсинки даже с
	// сервера в дата-центре отдавал 40 Мбит/с и мерил себя, а не канал.
	if r.Providers[0].ID != "clouvider" {
		t.Fatalf("первым стоит %s", r.Providers[0].ID)
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
