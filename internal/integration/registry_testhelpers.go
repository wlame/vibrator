package integration

// resetRegistry clears the global registry and load-error list. Used
// from tests in this package (test files share a package, so this
// helper is reachable but unexported). Production code never calls it.
//
// Lives in a non-_test.go file so internal subpackages with tests can
// also reach it (Go's build tags would otherwise restrict it to this
// package's own tests).
func resetRegistry() {
	regMu.Lock()
	registry = nil
	regMu.Unlock()
	loadErrorsMu.Lock()
	loadErrors = nil
	loadErrorsMu.Unlock()
}
