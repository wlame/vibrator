package integration

import "sync"

// The registry is a package-level slice + RWMutex. Reads (All, Get)
// outnumber writes (Register, which happens once per integration at
// init time), so RWMutex is the right pick over a plain Mutex.
var (
	regMu    sync.RWMutex
	registry []*Integration
)

// Register adds an Integration to the global registry. Typically called
// from a sub-package's init() function.
//
// Panics on nil/missing ID or on duplicate ID — both are programmer
// errors that should fail loudly at program start, not be silently
// swallowed at runtime.
func Register(i *Integration) {
	if i == nil || i.ID == "" {
		panic("integration: Register called with nil or unidentified Integration")
	}
	regMu.Lock()
	defer regMu.Unlock()
	for _, existing := range registry {
		if existing.ID == i.ID {
			panic("integration: duplicate ID " + i.ID)
		}
	}
	registry = append(registry, i)
}

// All returns a snapshot of all registered integrations. The returned
// slice is safe to range over and re-order; modifications don't affect
// the registry. The Integration pointers themselves are shared with
// the registry — callers should treat the descriptors as read-only.
func All() []*Integration {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]*Integration, len(registry))
	copy(out, registry)
	return out
}

// Get looks up an Integration by ID. Returns (nil, false) when not
// found.
func Get(id string) (*Integration, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	for _, i := range registry {
		if i.ID == id {
			return i, true
		}
	}
	return nil, false
}
