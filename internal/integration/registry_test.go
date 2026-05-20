package integration

import (
	"sync"
	"testing"
)

func TestRegister_AddsToRegistry(t *testing.T) {
	resetRegistry()

	Register(&Integration{ID: "alpha", Name: "Alpha"})
	Register(&Integration{ID: "beta", Name: "Beta"})

	got := All()
	if len(got) != 2 {
		t.Fatalf("All() = %d entries, want 2", len(got))
	}
	if got[0].ID != "alpha" || got[1].ID != "beta" {
		t.Errorf("registry order = [%s, %s], want [alpha, beta]",
			got[0].ID, got[1].ID)
	}
}

func TestRegister_DuplicateIDPanics(t *testing.T) {
	resetRegistry()

	Register(&Integration{ID: "dup", Name: "First"})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate ID, got none")
		}
	}()
	Register(&Integration{ID: "dup", Name: "Second"})
}

func TestRegister_NilOrEmptyIDPanics(t *testing.T) {
	cases := []struct {
		name string
		in   *Integration
	}{
		{"nil", nil},
		{"empty-id", &Integration{ID: "", Name: "X"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetRegistry()
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected panic, got none")
				}
			}()
			Register(tc.in)
		})
	}
}

func TestGet_FindsRegistered(t *testing.T) {
	resetRegistry()
	Register(&Integration{ID: "foo", Name: "Foo"})

	got, ok := Get("foo")
	if !ok {
		t.Fatal("Get(foo) returned ok=false")
	}
	if got.Name != "Foo" {
		t.Errorf("Get(foo).Name = %q, want Foo", got.Name)
	}
}

func TestGet_MissingReturnsFalse(t *testing.T) {
	resetRegistry()
	if _, ok := Get("missing"); ok {
		t.Error("Get(missing) returned ok=true on empty registry")
	}
}

func TestAll_ReturnsSnapshot(t *testing.T) {
	// Modifying the returned slice MUST NOT affect the registry — the
	// registry is the source of truth, callers get a copy.
	resetRegistry()
	Register(&Integration{ID: "a", Name: "A"})

	snap := All()
	snap[0] = &Integration{ID: "MUTATED"}

	got, _ := Get("a")
	if got.Name != "A" {
		t.Errorf("registry was mutated through All() snapshot — got Name=%q", got.Name)
	}
}

func TestRegister_ConcurrentSafe(t *testing.T) {
	// RWMutex protects concurrent Register/All. Hammer it from many
	// goroutines and verify no panic + correct count.
	resetRegistry()

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			Register(&Integration{ID: idxToID(idx), Name: "X"})
		}(i)
	}
	wg.Wait()

	if got := len(All()); got != n {
		t.Errorf("registry has %d entries after concurrent register, want %d", got, n)
	}
}

func idxToID(i int) string {
	return "id-" + intToString(i)
}

func intToString(i int) string {
	if i == 0 {
		return "0"
	}
	var s string
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
