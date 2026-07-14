package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "missing", want: 0},
		{name: "seconds", value: "3", want: 3 * time.Second},
		{name: "zero", value: "0", want: 0},
		{name: "negative", value: "-4", want: 0},
		{name: "invalid", value: "soon", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &http.Response{Header: http.Header{}}
			if tt.value != "" {
				response.Header.Set("Retry-After", tt.value)
			}
			if got := retryAfter(response); got != tt.want {
				t.Fatalf("retryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClientDoRetriesAndReplaysBody(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"question":"hello"}` {
			t.Errorf("body = %q", body)
		}
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultRetryConfig()
	config.BaseDelay = time.Millisecond
	config.MaxDelay = time.Millisecond
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader(`{"question":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := NewClient(WithRetry(config)).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}
}

func TestClientDoStopsRetryWhenContextIsCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	config := DefaultRetryConfig()
	config.BaseDelay = time.Second
	config.MaxDelay = time.Second
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := NewClient(WithRetry(config)).Do(req); done <- err }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Do() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Do() did not stop after context cancellation")
	}
}

func TestClientDoRefusesUnreplayableBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	config := DefaultRetryConfig()
	config.BaseDelay = 0
	config.MaxDelay = 0
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, io.NopCloser(strings.NewReader("{}")))
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewClient(WithRetry(config)).Do(req)
	if !errors.Is(err, ErrRequestBodyNotReplayable) {
		t.Fatalf("Do() error = %v, want %v", err, ErrRequestBodyNotReplayable)
	}
}
