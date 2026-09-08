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
	Providers              []Provider
	Duration               time.Duration
	Limit, Chunk, MinBytes int64
	Threads                int
}

// Independent operators, with endpoints published by their own speed-test clients.
// A CDN error page is not a benchmark, regardless of how confidently it arrives.
func Default() Runner {
	return Runner{
		Providers: []Provider{
			{"librespeed", "LibreSpeed · Helsinki", "https://www.librespeed.fi/backend/garbage.php", "https://www.librespeed.fi/backend/empty.php"},
			{"clouvider", "Clouvider · Amsterdam", "https://ams.speedtest.clouvider.net/backend/garbage.php", "https://ams.speedtest.clouvider.net/backend/empty.php"},
			{"openspeedtest", "OpenSpeedTest", "https://open.cachefly.net/downloading", "https://fal.openspeedtest.com/upload"},
			{"cloudflare", "Cloudflare", "https://speed.cloudflare.com/__down", "https://speed.cloudflare.com/__up"},
		}, Duration: 7 * time.Second, Limit: 64 << 20, Chunk: 4 << 20, MinBytes: 64 << 10, Threads: 3,
	}
}

func Client(proxy string) (*http.Client, error) {
	tr := &http.Transport{Proxy: nil, DisableCompression: true, MaxIdleConnsPerHost: 3, MaxConnsPerHost: 3,
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
	result.Error = "Ни один сервис не завершил приём и отдачу. Повторите позже."
	return result
}

func (r Runner) phase(parent context.Context, c *http.Client, endpoint string, upload bool) (float64, error) {
	if parent.Err() != nil {
		return 0, parent.Err()
	}
	if r.Duration <= 0 || r.Duration > 10*time.Second || r.Limit <= 0 || r.Limit > 64<<20 || r.Chunk <= 0 || r.Chunk > 4<<20 || r.MinBytes <= 0 || r.Threads < 1 || r.Threads > 3 {
		return 0, errors.New("некорректные ограничения замера")
	}
	ctx, cancel := context.WithTimeout(parent, r.Duration)
	defer cancel()
	started := time.Now()
	var reserved, received atomic.Int64
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
				count, err := transfer(ctx, c, endpoint, upload, size)
				received.Add(count)
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
	if parent.Err() != nil {
		return 0, parent.Err()
	}
	// An unfinished last block may count on download; an unacknowledged upload never does.
	if received.Load() < r.MinBytes {
		if firstErr != nil {
			return 0, firstErr
		}
		return 0, errors.New("слишком мало данных для замера")
	}
	if firstErr != nil && !errors.Is(firstErr, context.DeadlineExceeded) {
		return 0, firstErr
	}
	return float64(received.Load()) * 8 / time.Since(started).Seconds() / 1e6, nil
}

func transfer(ctx context.Context, c *http.Client, endpoint string, upload bool, size int64) (int64, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return 0, errors.New("неверный адрес сервиса")
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
	source := &payload{remaining: size}
	if upload {
		method = http.MethodPost
		body = source
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return 0, errors.New("не удалось создать запрос")
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
			return 0, ctx.Err()
		}
		return 0, errors.New("сервис не отвечает или недоступен")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return 0, fmt.Errorf("сервис ответил HTTP %d", response.StatusCode)
	}
	if response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" {
		return 0, errors.New("сервис вернул сжатый ответ")
	}
	contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if !upload && contentType != "application/octet-stream" && contentType != "application/binary" {
		return 0, errors.New("сервис вернул страницу вместо тестовых данных")
	}
	if upload {
		n, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 8193))
		if strings.Contains(contentType, "html") && n != 0 {
			return 0, errors.New("сервис вернул страницу вместо подтверждения")
		}
		if readErr != nil || n > 8192 || source.read.Load() != size {
			return 0, errors.New("сервис не подтвердил полную отдачу")
		}
		return size, nil
	}
	n, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, size))
	if readErr != nil {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		return 0, errors.New("приём данных прервался")
	}
	if n == 0 {
		return 0, errors.New("сервис вернул пустой ответ")
	}
	return n, nil
}

type payload struct {
	remaining int64
	read      atomic.Int64
}

func (p *payload) Read(b []byte) (int, error) {
	if p.remaining == 0 {
		return 0, io.EOF
	}
	n := int(min(int64(len(b)), p.remaining))
	clear(b[:n])
	p.remaining -= int64(n)
	p.read.Add(int64(n))
	return n, nil
}
