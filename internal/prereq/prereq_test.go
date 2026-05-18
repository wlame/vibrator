package prereq

import (
	"context"
	"testing"
)

func TestRegistry_RegisterAndByID(t *testing.T) {
	// We don't want this test to mutate the package-level Registry, so save
	// and restore. Tests that share global state need to clean up after
	// themselves — Go does not isolate package vars between tests.
	saved := Registry
	defer func() { Registry = saved }()
	Registry = map[string]*Prereq{}

	p := &Prereq{ID: "demo", Name: "Demo", Verifier: VerifierFunc(func(context.Context) Result {
		return Result{OK: true, Message: "ok"}
	})}
	Register(p)

	got, ok := ByID("demo")
	if !ok || got != p {
		t.Errorf("ByID(demo) = (%v, %v), want (%v, true)", got, ok, p)
	}
	if _, ok := ByID("missing"); ok {
		t.Errorf("ByID(missing) should report not found")
	}
}

func TestRegistry_DuplicateRegisterPanics(t *testing.T) {
	saved := Registry
	defer func() { Registry = saved }()
	Registry = map[string]*Prereq{}

	defer func() {
		if recover() == nil {
			t.Errorf("expected panic on duplicate Register")
		}
	}()
	p := &Prereq{ID: "dup", Verifier: VerifierFunc(func(context.Context) Result {
		return Result{OK: true}
	})}
	Register(p)
	Register(p) // second call must panic
}

func TestRegister_NilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic on nil register")
		}
	}()
	Register(nil)
}

func TestRegister_EmptyIDPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic on empty-id register")
		}
	}()
	Register(&Prereq{Name: "no id"})
}
