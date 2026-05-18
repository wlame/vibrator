package prereq

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// --- HTTPVerify -----------------------------------------------------------

func TestHTTPVerify_AcceptsAnyServerResponseByDefault(t *testing.T) {
	// HTTPVerify with no ExpectStatus accepts 200..499 because most servers
	// don't expose a stable /health route; a 404 still proves the server is
	// up.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	r := HTTPVerify{URL: srv.URL}.Verify(context.Background())
	if !r.OK {
		t.Errorf("expected OK for 404 with default ExpectStatus, got %v: %s", r.OK, r.Message)
	}
}

func TestHTTPVerify_RespectsExplicitExpectStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := HTTPVerify{URL: srv.URL, ExpectStatus: []int{200}}.Verify(context.Background())
	if r.OK {
		t.Errorf("expected fail when 500 and ExpectStatus=[200], got OK")
	}
	if r.Hint != "" {
		// Default Hint is empty — only set on the constructor when caller provides one
		t.Logf("hint propagation tested separately")
	}
}

func TestHTTPVerify_AddsBearerToken(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	HTTPVerify{URL: srv.URL, BearerToken: "abc"}.Verify(context.Background())
	if got != "Bearer abc" {
		t.Errorf("Authorization header = %q, want %q", got, "Bearer abc")
	}
}

func TestHTTPVerify_FailsOnUnreachable(t *testing.T) {
	// Port 1 is the well-known "definitely-not-listening" sentinel.
	r := HTTPVerify{URL: "http://127.0.0.1:1", Timeout: 200 * time.Millisecond,
		Hint: "is the server running?"}.Verify(context.Background())
	if r.OK {
		t.Errorf("expected fail for unreachable URL, got OK")
	}
	if r.Hint != "is the server running?" {
		t.Errorf("expected Hint to flow through, got %q", r.Hint)
	}
}

func TestHTTPVerify_EmptyURLFails(t *testing.T) {
	r := HTTPVerify{}.Verify(context.Background())
	if r.OK {
		t.Errorf("expected fail for empty URL")
	}
}

// --- CommandVerify --------------------------------------------------------

func TestCommandVerify_SuccessAndFailure(t *testing.T) {
	// Pick portable commands. `true` and `false` exist on every POSIX system
	// we care about. On Windows we don't run this test — vibrate's CLI is
	// Linux/macOS-only.
	if runtime.GOOS == "windows" {
		t.Skip("CommandVerify behavior is verified on POSIX hosts only")
	}

	if r := (CommandVerify{Args: []string{"true"}}).Verify(context.Background()); !r.OK {
		t.Errorf("`true` should pass: %v %q", r.OK, r.Message)
	}

	if r := (CommandVerify{Args: []string{"false"}, Hint: "nope"}).Verify(context.Background()); r.OK {
		t.Errorf("`false` should fail")
	}
}

func TestCommandVerify_EmptyArgs(t *testing.T) {
	r := CommandVerify{}.Verify(context.Background())
	if r.OK {
		t.Errorf("expected fail for empty Args")
	}
}

func TestCommandVerify_BinaryNotFound(t *testing.T) {
	r := CommandVerify{Args: []string{"definitely-not-a-real-binary-vibrate-12345"}}.Verify(context.Background())
	if r.OK {
		t.Errorf("expected fail for missing binary")
	}
}

// --- FileVerify -----------------------------------------------------------

func TestFileVerify_PresentAndAbsent(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(present, []byte("hi"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	absent := filepath.Join(dir, "nope.txt")

	cases := []struct {
		name      string
		f         FileVerify
		wantOK    bool
	}{
		{"present-and-must-exist", FileVerify{Path: present, MustExist: true}, true},
		{"present-but-must-be-absent", FileVerify{Path: present, MustExist: false}, false},
		{"absent-and-must-exist", FileVerify{Path: absent, MustExist: true}, false},
		{"absent-and-must-be-absent", FileVerify{Path: absent, MustExist: false}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.f.Verify(context.Background())
			if r.OK != tc.wantOK {
				t.Errorf("OK = %v, want %v (%s)", r.OK, tc.wantOK, r.Message)
			}
		})
	}
}

// --- VerifierFunc ---------------------------------------------------------

func TestVerifierFunc_Composes(t *testing.T) {
	called := false
	v := VerifierFunc(func(_ context.Context) Result {
		called = true
		return Result{OK: true, Message: "ok"}
	})
	if r := v.Verify(context.Background()); !r.OK || !called {
		t.Errorf("VerifierFunc did not invoke its body")
	}
}
