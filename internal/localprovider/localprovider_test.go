package localprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

// --- Registry -------------------------------------------------------------

func TestRegistry_BuiltinsPresent(t *testing.T) {
	for _, id := range []string{"ollama", "lmstudio"} {
		if _, ok := ByID(id); !ok {
			t.Errorf("expected built-in provider %q to be registered", id)
		}
	}
}

func TestIDs_Sorted(t *testing.T) {
	ids := IDs()
	// lmstudio < ollama alphabetically.
	if len(ids) < 2 || ids[0] != "lmstudio" {
		t.Errorf("expected sorted IDs starting with lmstudio, got %v", ids)
	}
}

func TestRegister_PanicsOnNilOrEmpty(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic on nil register")
		}
	}()
	Register(nil)
}

// --- Ollama provider ------------------------------------------------------

func TestOllamaProbe_Reachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	if err := (ollamaProvider{}).Probe(context.Background(), srv.URL); err != nil {
		t.Errorf("Probe should succeed against a 200-responding /api/tags: %v", err)
	}
}

func TestOllamaProbe_FailsWhenUnreachable(t *testing.T) {
	err := (ollamaProvider{}).Probe(context.Background(), "http://127.0.0.1:1")
	if err == nil {
		t.Errorf("Probe should fail for closed port")
	}
}

func TestOllamaListLocalModels_ParsesSortedNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Names in unsorted order — provider must sort.
		_, _ = w.Write([]byte(`{
			"models": [
				{"name": "qwen3:32b"},
				{"name": "llama3:70b"},
				{"name": "mistral:7b"}
			]
		}`))
	}))
	defer srv.Close()

	got, err := (ollamaProvider{}).ListLocalModels(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ListLocalModels: %v", err)
	}
	want := []string{"llama3:70b", "mistral:7b", "qwen3:32b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestOllamaEnsureRunning_FastPathWhenAlreadyUp(t *testing.T) {
	// Server responds 200 to /api/tags from the start — EnsureRunning
	// should not invoke commandStarter at all.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	var spawned atomic.Int32
	orig := commandStarter
	commandStarter = func(string, ...string) error {
		spawned.Add(1)
		return nil
	}
	defer func() { commandStarter = orig }()

	if err := (ollamaProvider{}).EnsureRunning(context.Background(), srv.URL, ""); err != nil {
		t.Errorf("EnsureRunning should succeed: %v", err)
	}
	if spawned.Load() != 0 {
		t.Errorf("commandStarter should not be called when server is already up")
	}
}

func TestOllamaEnsureRunning_ErrorsWhenBinaryMissing(t *testing.T) {
	// Use a definitely-unreachable URL; this will force EnsureRunning into
	// its slow path. The slow path checks PATH for the `ollama` binary.
	// On the test container `ollama` is unlikely to be installed, so we
	// expect a clear error message.
	err := (ollamaProvider{}).EnsureRunning(context.Background(), "http://127.0.0.1:1", "")
	if err == nil {
		t.Errorf("expected EnsureRunning to fail without binary")
	}
}

func TestOllamaEnsureRunning_PollsUntilHealthy(t *testing.T) {
	// Verifies the polling behavior of EnsureRunning's slow path *in
	// isolation* — covering specifically that waitForServer correctly
	// transitions from "probe returns error" to "probe returns nil"
	// once the server flips to ready.
	//
	// Note: we can't exercise the full slow path (LookPath + spawn) from
	// a unit test without faking the host binary too. That coverage will
	// come from a smoke test on a host that actually has `ollama`.
	var ready atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			// Pre-ready: respond 500 so ollamaGetJSON returns an error.
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	// Flip the server to ready after 50ms, *outside* commandStarter
	// (which never gets called here).
	go func() {
		time.Sleep(50 * time.Millisecond)
		ready.Store(true)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := waitForServer(ctx,
		func(ctx context.Context) error { return (ollamaProvider{}).Probe(ctx, srv.URL) },
		2*time.Second, 25*time.Millisecond)
	if err != nil {
		t.Errorf("waitForServer should transition to ready: %v", err)
	}
}

// --- LM Studio provider ---------------------------------------------------

func TestLMStudioListLocalModels_ParsesOpenAIShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// OpenAI-compatible shape: data[].id
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "qwen3-coder-32b", "object": "model"},
				{"id": "phi-3.5-mini",    "object": "model"}
			]
		}`))
	}))
	defer srv.Close()

	got, err := (lmstudioProvider{}).ListLocalModels(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ListLocalModels: %v", err)
	}
	want := []string{"phi-3.5-mini", "qwen3-coder-32b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLMStudioProbe_RespectsCustomURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	if err := (lmstudioProvider{}).Probe(context.Background(), srv.URL); err != nil {
		t.Errorf("Probe should succeed against test server: %v", err)
	}
}

// --- Shared HTTP helper ---------------------------------------------------

func TestOllamaGetJSON_DecodesIntoDst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	}))
	defer srv.Close()

	var got map[string]string
	if err := ollamaGetJSON(context.Background(), srv.URL, &got, 1*time.Second); err != nil {
		t.Fatalf("ollamaGetJSON: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("decode mismatch: %v", got)
	}
}

func TestOllamaGetJSON_NilDstJustProbes(t *testing.T) {
	called := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := ollamaGetJSON(context.Background(), srv.URL, nil, 1*time.Second); err != nil {
		t.Errorf("probe with nil dst should succeed: %v", err)
	}
	if called.Load() != 1 {
		t.Errorf("server should be hit exactly once, got %d", called.Load())
	}
}

func TestOllamaGetJSON_ErrorOn4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := ollamaGetJSON(context.Background(), srv.URL, nil, 1*time.Second)
	if err == nil {
		t.Errorf("expected error on 404")
	}
}

// --- waitForServer --------------------------------------------------------

func TestWaitForServer_SucceedsWhenProbeReturnsNil(t *testing.T) {
	attempts := atomic.Int32{}
	probe := func(context.Context) error {
		if attempts.Add(1) < 3 {
			return errors.New("not yet")
		}
		return nil
	}
	err := waitForServer(context.Background(), probe, 1*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Errorf("waitForServer should succeed: %v", err)
	}
	if attempts.Load() < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attempts.Load())
	}
}

func TestWaitForServer_TimesOutAndReturnsLastError(t *testing.T) {
	probe := func(context.Context) error { return errors.New("always-fail") }
	err := waitForServer(context.Background(), probe, 100*time.Millisecond, 25*time.Millisecond)
	if err == nil {
		t.Errorf("expected timeout error")
	}
}

func TestWaitForServer_RespectsContextCancel(t *testing.T) {
	probe := func(context.Context) error { return errors.New("nope") }
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before we start
	err := waitForServer(ctx, probe, 5*time.Second, 25*time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
