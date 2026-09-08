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
