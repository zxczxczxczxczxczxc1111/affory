// Package skorost measures bounded application throughput, without tuning the tunnel.
package skorost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Provider struct{ ID, Name, Download, Upload string }
type Attempt struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Error    string `json:"error,omitempty"`
}
type Result struct {
	Provider string    `json:"provider"`
	Name     string    `json:"name"`
	Download float64   `json:"download_mbps,omitempty"`
	Upload   float64   `json:"upload_mbps,omitempty"`
	Attempts []Attempt `json:"attempts"`
	Error    string    `json:"error,omitempty"`
}
type Progress struct {
	Provider, Name, Phase string
	Attempt               int
}
type Runner struct {
	Providers []Provider
	Duration  time.Duration
	// Warmup это разгон в начале фазы, который в число не входит.
	Warmup                 time.Duration
	Limit, Chunk, MinBytes int64
	Threads                int
}

// Потолки ограничений. Потоков не больше, чем соединений, которые даёт Client.
const (
	PredelPotokov = 8
	predelSroka   = 15 * time.Second
	predelObyoma  = 256 << 20
	// Окно короче секунды меряет случайность, а не канал.
	minOkno = time.Second
)

// Independent operators, with endpoints published by their own speed-test clients.
// A CDN error page is not a benchmark, regardless of how confidently it arrives.
//
// Параметры пересмотрены 26.09.2026 сравнением на живой машине через туннель:
// прежние 3 потока на 7 секунд вместе с разгоном давали 10-12 Мбит/с приёма
// там, где 8 потоков на 12 секунд без разгона давали 65-74, а Ookla через тот
// же туннель 68. Замер врал вниз впятеро, и человек читал это как беду ключа.
//
// Порядок это выбор «Автоматически». Clouvider в Амстердаме первым: с нашего
// сервера он отдал 407 Мбит/с, а LibreSpeed в Хельсинки, стоявший первым, всего
// 40 даже с сервера в дата-центре, то есть мерил себя, а не канал человека.
// Cloudflare вторым: он отвечает с ближайшего к выходу узла.
//
// Потолок объёма 256 МБ на фазу: на канале до 500 Мбит/с он не наступает
// раньше конца окна, а на гигабите фаза кончается раньше срока и число
// считается по всей фазе.
func Default() Runner {
	return Runner{
		Providers: []Provider{
			{"clouvider", "Clouvider · Amsterdam", "https://ams.speedtest.clouvider.net/backend/garbage.php", "https://ams.speedtest.clouvider.net/backend/empty.php"},
			{"cloudflare", "Cloudflare", "https://speed.cloudflare.com/__down", "https://speed.cloudflare.com/__up"},
			{"librespeed", "LibreSpeed · Helsinki", "https://www.librespeed.fi/backend/garbage.php", "https://www.librespeed.fi/backend/empty.php"},
			{"openspeedtest", "OpenSpeedTest", "https://open.cachefly.net/downloading", "https://fal.openspeedtest.com/upload"},
		}, Duration: 10 * time.Second, Warmup: 2 * time.Second, Limit: predelObyoma, Chunk: 4 << 20, MinBytes: 64 << 10, Threads: 6,
	}
}

func Client(proxy string) (*http.Client, error) {
	tr := &http.Transport{Proxy: nil, DisableCompression: true, MaxIdleConnsPerHost: PredelPotokov, MaxConnsPerHost: PredelPotokov,
		DialContext:         (&net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 5 * time.Second}
	if proxy != "" {
		u, err := url.Parse("http://" + proxy)
		if err != nil || u.Host != proxy {
			return nil, errors.New("неверный адрес локального прокси")
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("сервис перенаправил тест")
	}}, nil
}

func (r Runner) Run(ctx context.Context, c *http.Client, preferred string, progress func(Progress)) Result {
	result := Result{Attempts: []Attempt{}}
	providers := append([]Provider(nil), r.Providers...)
	if preferred != "" {
		found := -1
		for i, p := range providers {
			if p.ID == preferred {
				found = i
				break
			}
		}
		if found < 0 {
			result.Error = "Сервис не найден"
			return result
		}
		p := providers[found]
		providers = append([]Provider{p}, append(providers[:found], providers[found+1:]...)...)
	}
	for i, p := range providers {
		if ctx.Err() != nil {
			result.Error = "Замер отменён"
			return result
		}
		publish := func(phase string) {
			if progress != nil {
				progress(Progress{p.ID, p.Name, phase, i + 1})
			}
		}
		publish("download")
		down, err := r.phase(ctx, c, p.Download, false)
		var up float64
		if err == nil {
			publish("upload")
			up, err = r.phase(ctx, c, p.Upload, true)
		}
		attempt := Attempt{Provider: p.ID, Name: p.Name}
		if err != nil {
			attempt.Error = err.Error()
		} else {
			result.Provider = p.ID
			result.Name = p.Name
			result.Download = down
			result.Upload = up
		}
		result.Attempts = append(result.Attempts, attempt)
		if ctx.Err() != nil {
			result.Download = 0
			result.Upload = 0
			result.Error = "Замер отменён"
			return result
		}
		if err == nil {
			return result
		}
	}
	result.Error = "ни один сервис не завершил приём и отдачу, повтори позже"
	return result
}

func (r Runner) proverit() error {
	if r.Duration <= 0 || r.Duration > predelSroka || r.Warmup < 0 || r.Warmup >= r.Duration ||
		r.Limit <= 0 || r.Limit > predelObyoma || r.Chunk <= 0 || r.Chunk > 4<<20 || r.MinBytes <= 0 ||
		r.Threads < 1 || r.Threads > PredelPotokov {
		return errors.New("некорректные ограничения замера")
	}
	return nil
}

func (r Runner) phase(parent context.Context, c *http.Client, endpoint string, upload bool) (float64, error) {
	if parent.Err() != nil {
		return 0, parent.Err()
	}
	if err := r.proverit(); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(parent, r.Duration)
	defer cancel()
	started := time.Now()
	var reserved, received atomic.Int64
	// Байты считаются ПО ХОДУ передачи, а не по концу запроса: иначе снимок
	// на границе разгона видел бы только законченные куски.
	var oknoMu sync.Mutex
	var nachaloOkna time.Time
	var vNachaleOkna int64
	razgon := time.AfterFunc(r.Warmup, func() {
		oknoMu.Lock()
		nachaloOkna, vNachaleOkna = time.Now(), received.Load()
		oknoMu.Unlock()
	})
	var workers sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	fail := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}
	for i := 0; i < r.Threads; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			chunk := r.Chunk
			if upload {
				chunk = min(chunk, 64<<10)
			}
			for ctx.Err() == nil {
				allocation := reserved.Add(chunk) - chunk
				if allocation >= r.Limit {
					return
				}
				size := min(chunk, r.Limit-allocation)
				err := transfer(ctx, c, endpoint, upload, size, &received)
				if err != nil {
					fail(err)
					return
				}
				if upload {
					chunk = min(chunk*2, r.Chunk)
				}
			}
		}()
	}
	workers.Wait()
	razgon.Stop()
	konec := time.Now()
	if parent.Err() != nil {
		return 0, parent.Err()
	}
	// Недокачанный и недоотправленный на сроке кусок засчитывается тем, что
	// успело пройти. Для отдачи это перемена: прежде считались только
	// подтверждённые тела, и последний кусок каждого потока, до 4 МБ, пропадал
	// целиком. Отказ сервиса по-прежнему не становится числом: любая ошибка,
	// кроме срока, валит фазу ниже.
	if received.Load() < r.MinBytes {
		if firstErr != nil {
			return 0, firstErr
		}
		return 0, errors.New("слишком мало данных для замера")
	}
	if firstErr != nil && !errors.Is(firstErr, context.DeadlineExceeded) {
		return 0, firstErr
	}
	oknoMu.Lock()
	nachalo, vNachale := nachaloOkna, vNachaleOkna
	oknoMu.Unlock()
	return skorostOkna(received.Load(), started, nachalo, vNachale, konec), nil
}

// skorostOkna считает скорость по окну после разгона. Окно, которого не
// случилось или которое короче minOkno, заменяется всей фазой: быстрый канал
// выбирает потолок объёма раньше конца разгона, и честнее отдать число по всей
// фазе, чем по доле секунды.
func skorostOkna(vsego int64, nachalo, nachaloOkna time.Time, vNachaleOkna int64, konec time.Time) float64 {
	if !nachaloOkna.IsZero() && konec.Sub(nachaloOkna) >= minOkno && vsego > vNachaleOkna {
		return float64(vsego-vNachaleOkna) * 8 / konec.Sub(nachaloOkna).Seconds() / 1e6
	}
	return float64(vsego) * 8 / konec.Sub(nachalo).Seconds() / 1e6
}

// schetchik добавляет прошедшие байты в общий счёт фазы.
type schetchik struct{ n *atomic.Int64 }

func (s schetchik) Write(b []byte) (int, error) {
	s.n.Add(int64(len(b)))
	return len(b), nil
}

func transfer(ctx context.Context, c *http.Client, endpoint string, upload bool, size int64, schet *atomic.Int64) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("неверный адрес сервиса")
	}
	q := u.Query()
	q.Set("affory", strconv.FormatInt(time.Now().UnixNano(), 10))
	if !upload {
		q.Set("bytes", strconv.FormatInt(size, 10))
		q.Set("ckSize", strconv.FormatInt((size+(1<<20)-1)/(1<<20), 10))
	}
	u.RawQuery = q.Encode()
	method := http.MethodGet
	var body io.Reader
	source := &payload{remaining: size, schet: schet}
	if upload {
		method = http.MethodPost
		body = source
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return errors.New("не удалось создать запрос")
	}
	req.Header.Set("Cache-Control", "no-cache, no-store")
	req.Header.Set("User-Agent", "Affory-SpeedTest/1")
	if upload {
		req.ContentLength = size
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	response, err := c.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("сервис не отвечает или недоступен")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("сервис ответил HTTP %d", response.StatusCode)
	}
	if response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" {
		return errors.New("сервис вернул сжатый ответ")
	}
	contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if !upload && contentType != "application/octet-stream" && contentType != "application/binary" {
		return errors.New("сервис вернул страницу вместо тестовых данных")
	}
	if upload {
		n, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 8193))
		if strings.Contains(contentType, "html") && n != 0 {
			return errors.New("сервис вернул страницу вместо подтверждения")
		}
		if readErr != nil || n > 8192 || source.read.Load() != size {
			return errors.New("сервис не подтвердил полную отдачу")
		}
		return nil
	}
	n, readErr := io.Copy(schetchik{schet}, io.LimitReader(response.Body, size))
	if readErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("приём данных прервался")
	}
	if n == 0 {
		return errors.New("сервис вернул пустой ответ")
	}
	return nil
}

type payload struct {
	remaining int64
	read      atomic.Int64
	// schet это общий счёт фазы: отдача считается по мере того, как тело
	// уходит в соединение.
	schet *atomic.Int64
}

func (p *payload) Read(b []byte) (int, error) {
	if p.remaining == 0 {
		return 0, io.EOF
	}
	n := int(min(int64(len(b)), p.remaining))
	clear(b[:n])
	p.remaining -= int64(n)
	p.read.Add(int64(n))
	if p.schet != nil {
		p.schet.Add(int64(n))
	}
	return n, nil
}
