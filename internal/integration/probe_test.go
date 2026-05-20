package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProbe_Check_Reachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := HTTPProbe{URL: srv.URL}
	if err := p.Check(context.Background()); err != nil {
		t.Errorf("Check on reachable server: %v", err)
	}
}

func TestHTTPProbe_Check_4xxIsSuccess(t *testing.T) {
	// 4xx means the server is up — just doesn't like our request.
	// Probe contract: anything < 500 = reachable.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	p := HTTPProbe{URL: srv.URL}
	if err := p.Check(context.Background()); err != nil {
		t.Errorf("Check on 404 server: %v (expected nil — 4xx still means reachable)", err)
	}
}

func TestHTTPProbe_Check_5xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	p := HTTPProbe{URL: srv.URL}
	if err := p.Check(context.Background()); err == nil {
		t.Error("Check on 500 server: expected error, got nil")
	}
}

func TestHTTPProbe_Check_Unreachable(t *testing.T) {
	// 127.0.0.1:1 should refuse the connection on every reasonable
	// system — there's nothing listening on that low port.
	p := HTTPProbe{URL: "http://127.0.0.1:1", Timeout: 100 * time.Millisecond}
	if err := p.Check(context.Background()); err == nil {
		t.Error("Check on closed port: expected error, got nil")
	}
}

func TestHTTPProbe_Describe(t *testing.T) {
	p := HTTPProbe{URL: "http://example.test/mcp"}
	if got := p.Describe(); got != "http://example.test/mcp" {
		t.Errorf("Describe = %q, want http://example.test/mcp", got)
	}
}

func TestHTTPProbe_Wait_SucceedsImmediately(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := HTTPProbe{URL: srv.URL}
	start := time.Now()
	if err := p.Wait(context.Background(), 5*time.Second); err != nil {
		t.Errorf("Wait on reachable server: %v", err)
	}
	if dur := time.Since(start); dur > 2*time.Second {
		t.Errorf("Wait took %v on reachable server — should be near-instant", dur)
	}
}

func TestHTTPProbe_Wait_TimesOut(t *testing.T) {
	p := HTTPProbe{URL: "http://127.0.0.1:1", Timeout: 100 * time.Millisecond}
	if err := p.Wait(context.Background(), 200*time.Millisecond); err == nil {
		t.Error("Wait on closed port: expected timeout error, got nil")
	}
}

func TestHTTPProbe_Wait_RespectsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	p := HTTPProbe{URL: "http://127.0.0.1:1"}
	start := time.Now()
	err := p.Wait(ctx, 10*time.Second)
	if err == nil {
		t.Error("Wait with cancelled ctx: expected error")
	}
	// Wait may take a bit longer than 50ms because Probe.Check has
	// its own internal timeout and may run once before noticing
	// cancellation. Give it generous slack but flag a runaway.
	if dur := time.Since(start); dur > 3*time.Second {
		t.Errorf("Wait took %v after 50ms ctx cancel — should exit fast", dur)
	}
}
