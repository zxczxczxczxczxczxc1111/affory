package afforyprocess

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrSharedUnavailable is separate from an inaccessible/exited process: a
// failed tracker must not silently fall back to a different default route.
var ErrSharedUnavailable = errors.New("shared process tracker unavailable")

type identityRequest struct {
	PID     uint32 `json:"pid"`
	Created uint64 `json:"created"`
}
type identityResponse struct {
	Paths   []string `json:"paths"`
	Unknown bool     `json:"unknown,omitempty"`
}

type SharedServer struct {
	Address, Secret string
	server          *http.Server
	done            chan error
	closeOnce       sync.Once
	closeErr        error
}

// NewSharedServer serves only loopback with an ephemeral unguessable token.
// It stores no history on disk and exposes no command line or credentials.
func NewSharedServer(find func(uint32, uint64) ([]string, error)) (*SharedServer, error) {
	if find == nil {
		return nil, errors.New("missing process finder")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		listener.Close()
		return nil, err
	}
	s := &SharedServer{Address: "http://" + listener.Addr().String(), Secret: hex.EncodeToString(token[:]), done: make(chan error, 1)}
	active := make(chan struct{}, 32)
	s.server = &http.Server{ReadHeaderTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 4096}
	s.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Secret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/family" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		select {
		case active <- struct{}{}:
			defer func() { <-active }()
		default:
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		var q identityRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&q); err != nil || q.PID == 0 || q.Created == 0 {
			http.Error(w, "invalid identity", http.StatusBadRequest)
			return
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			http.Error(w, "trailing input", http.StatusBadRequest)
			return
		}
		paths, err := find(q.PID, q.Created)
		if errors.Is(err, ErrSharedUnavailable) {
			http.Error(w, "tracker unavailable", http.StatusServiceUnavailable)
			return
		}
		// An exited/reused/protected process is unknown. Never return paths from
		// the previous owner of the same PID or an executable-name guess.
		response := identityResponse{Paths: paths, Unknown: err != nil}
		if err != nil {
			response.Paths = nil
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			return
		}
	})
	go func() {
		err := s.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.done <- err
	}()
	return s, nil
}

func (s *SharedServer) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.server.Close()
		if err := <-s.done; err != nil {
			s.closeErr = err
		}
	})
	return s.closeErr
}

type SharedClient struct {
	address, secret string
	client          *http.Client
	transport       *http.Transport
}

func NewSharedClient(address, secret string) (*SharedClient, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || len(secret) != 64 {
		return nil, errors.New("invalid shared process tracker endpoint")
	}
	if _, err := hex.DecodeString(secret); err != nil {
		return nil, errors.New("invalid shared process tracker token")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, MaxConnsPerHost: 32, MaxIdleConnsPerHost: 4, IdleConnTimeout: 15 * time.Second, ResponseHeaderTimeout: time.Second}
	return &SharedClient{address: address, secret: secret, transport: transport, client: &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("tracker redirect refused") }}}, nil
}
func (c *SharedClient) Close() error { c.transport.CloseIdleConnections(); return nil }
func (c *SharedClient) Find(pid uint32) ([]string, error) {
	p, err := readProcess(pid)
	if err != nil {
		return nil, err
	}
	return c.find(context.Background(), p)
}
func (c *SharedClient) find(ctx context.Context, p Process) ([]string, error) {
	body, err := json.Marshal(identityRequest{PID: p.PID, Created: p.Created})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.address+"/family", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: request: %v", ErrSharedUnavailable, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSharedUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrSharedUnavailable, resp.StatusCode)
	}
	var result identityResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: invalid response", ErrSharedUnavailable)
	}
	if result.Unknown {
		return []string{p.Path}, nil
	}
	if len(result.Paths) == 0 || len(result.Paths) > maxDepth || !strings.EqualFold(result.Paths[0], p.Path) {
		return nil, fmt.Errorf("%w: mismatched identity", ErrSharedUnavailable)
	}
	for _, path := range result.Paths {
		if path == "" || len(path) > 131072 {
			return nil, fmt.Errorf("%w: invalid path", ErrSharedUnavailable)
		}
	}
	return result.Paths, nil
}
