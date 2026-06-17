package dockerfile

import (
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/harness"
	_ "github.com/wlame/vibrator/internal/harness/all" // register built-in harnesses
)

// minimalSpec builds the smallest Spec that passes validate — a registered
// harness plus a supported shell — mirroring the construction the
// dockerfile_test.go golden tests use.
func minimalSpec(t *testing.T) Spec {
	t.Helper()
	h, ok := harness.ByID("claude-code")
	if !ok {
		t.Fatalf("harness %q not registered", "claude-code")
	}
	return Spec{
		Harness: h,
		Shell:   "zsh",
	}
}

func TestValidateUsername(t *testing.T) {
	valid := []string{"", "wlame", "_svc", "a-b_c9", "x"}
	for _, u := range valid {
		if err := ValidateUsername(u); err != nil {
			t.Errorf("ValidateUsername(%q) = %v, want nil", u, err)
		}
	}
	invalid := []string{
		"Wlame", // uppercase
		"1abc",  // leading digit
		"with space",
		"a$b",
		strings.Repeat("x", 33),                // > 32 chars (useradd NAME_REGEX cap)
		"evil\nRUN echo pwned > /tmp/pwned\n#", // Dockerfile injection PoC
	}
	for _, u := range invalid {
		if err := ValidateUsername(u); err == nil {
			t.Errorf("ValidateUsername(%q) = nil, want error", u)
		}
	}
}

func TestGenerate_RejectsInjectedUsername(t *testing.T) {
	spec := minimalSpec(t)
	spec.Username = "evil\nRUN echo pwned\n#"
	if _, err := Generate(spec); err == nil {
		t.Fatal("Generate accepted a Dockerfile-injecting username")
	}
}
