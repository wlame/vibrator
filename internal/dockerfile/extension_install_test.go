package dockerfile

// Behaviour tests for the extension-install heredoc emitter. These
// tests don't run docker — they only assert the generator emits
// well-formed Dockerfile fragments for typical install patterns,
// especially nested heredocs (which the original `<<'EOF'` outer
// delimiter would have broken).

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteExtensionInstall_SimpleSnippet(t *testing.T) {
	var b bytes.Buffer
	err := writeExtensionInstall(&b, "echo hello\necho world")
	if err != nil {
		t.Fatalf("writeExtensionInstall: %v", err)
	}
	out := b.String()
	// Must use the unique outer delimiter, not bare `EOF`.
	if !strings.Contains(out, "RUN <<'"+extensionRunDelimiter+"'") {
		t.Errorf("missing outer heredoc delimiter:\n%s", out)
	}
	// Must inject set -e for fail-fast semantics.
	if !strings.Contains(out, "set -e\n") {
		t.Errorf("missing 'set -e' injection:\n%s", out)
	}
	// Must include the body lines verbatim.
	if !strings.Contains(out, "echo hello\necho world") {
		t.Errorf("body not preserved:\n%s", out)
	}
}

func TestWriteExtensionInstall_EmptyIsNoOp(t *testing.T) {
	var b bytes.Buffer
	err := writeExtensionInstall(&b, "")
	if err != nil {
		t.Errorf("empty install should not error: %v", err)
	}
	if b.Len() != 0 {
		t.Errorf("empty install produced output: %q", b.String())
	}
	// Whitespace-only too.
	b.Reset()
	err = writeExtensionInstall(&b, "   \n\t\n")
	if err != nil {
		t.Errorf("ws-only install should not error: %v", err)
	}
	if b.Len() != 0 {
		t.Errorf("ws-only install produced output: %q", b.String())
	}
}

func TestWriteExtensionInstall_NestedHeredocSafe(t *testing.T) {
	// This is the case that motivated changing the outer delimiter
	// away from EOF. An install script that contains its own
	// `cat <<'EOF' ... EOF` must NOT terminate the wrapping RUN
	// heredoc early.
	install := `mkdir -p /etc/foo
cat > /etc/foo/conf.json <<'EOF'
{"key": "value"}
EOF
echo "after the inner heredoc"`
	var b bytes.Buffer
	err := writeExtensionInstall(&b, install)
	if err != nil {
		t.Fatalf("nested heredoc install rejected: %v", err)
	}
	out := b.String()

	// Count outer delimiter occurrences — must be exactly 2
	// (open + close), not 4 (which would happen if the inner
	// EOFs were the outer delimiter too).
	if c := strings.Count(out, extensionRunDelimiter); c != 2 {
		t.Errorf("outer delimiter appears %d times, want 2:\n%s", c, out)
	}

	// The line AFTER the inner heredoc must still be present.
	if !strings.Contains(out, `echo "after the inner heredoc"`) {
		t.Errorf("line after inner heredoc missing:\n%s", out)
	}
}

func TestWriteExtensionInstall_RejectsCollidingDelimiter(t *testing.T) {
	// If an install snippet's body literally contains our outer
	// delimiter on a standalone line, that's a real collision the
	// generator can't fix automatically. We return an error rather
	// than emit silently-broken output.
	install := "echo before\n" + extensionRunDelimiter + "\necho after"
	var b bytes.Buffer
	err := writeExtensionInstall(&b, install)
	if err == nil {
		t.Error("expected error for delimiter collision, got nil")
	}
}

func TestWriteExtensionInstall_DelimiterInQuotedStringIsFine(t *testing.T) {
	// A collision only matters when the delimiter appears alone on a
	// line. If it appears as a substring (e.g., inside a single-line
	// shell command), no termination happens — accept it.
	install := `echo "the magic word is ` + extensionRunDelimiter + `"`
	var b bytes.Buffer
	err := writeExtensionInstall(&b, install)
	if err != nil {
		t.Errorf("non-standalone delimiter rejected: %v", err)
	}
}
