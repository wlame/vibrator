package cli

import (
	"bytes"
	"testing"

	"github.com/wlame/vibrator/internal/app"
	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/harness"
	_ "github.com/wlame/vibrator/internal/harness/all" // register built-in harnesses
)

// Regression for the bug where `vibrate build` produced the same image tag
// as the `vibrate`/`vibrate run` launch flow but never stamped the
// generator label — so a pre-built image would read as stale (or
// "unknown/pre-label") on every subsequent launch, even when it was in
// fact perfectly current. See app.GeneratorLabelKey and
// dockerfile.GeneratorHash.
func TestBuildImageSpec_AttachesGeneratorLabel(t *testing.T) {
	h, ok := harness.ByID("claude-code")
	if !ok {
		t.Fatal("harness \"claude-code\" not registered")
	}
	df := dockerfile.Spec{Harness: h, Shell: "zsh"}

	want, err := dockerfile.GeneratorHash(df)
	if err != nil {
		t.Fatalf("GeneratorHash: %v", err)
	}

	spec, err := buildImageSpec(df, []byte("FROM scratch"), "/ctx", "vb-test:latest", false, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildImageSpec: %v", err)
	}

	got, ok := spec.Labels[app.GeneratorLabelKey]
	if !ok {
		t.Fatalf("Labels missing key %q entirely — build.go's docker.BuildSpec must carry it so a launch-time staleness check can compare against it", app.GeneratorLabelKey)
	}
	if got != want {
		t.Errorf("Labels[%q] = %q, want %q", app.GeneratorLabelKey, got, want)
	}
}

// The hash must be insensitive to which tag/context/no-cache values the
// caller passes — those are build mechanics, not generator content — so two
// otherwise-identical specs always agree on the label regardless of the
// image tag used.
func TestBuildImageSpec_LabelIndependentOfTagAndContext(t *testing.T) {
	h, ok := harness.ByID("claude-code")
	if !ok {
		t.Fatal("harness \"claude-code\" not registered")
	}
	df := dockerfile.Spec{Harness: h, Shell: "zsh"}

	a, err := buildImageSpec(df, []byte("FROM scratch"), "/ctx-a", "tag-a:latest", false, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildImageSpec: %v", err)
	}
	b, err := buildImageSpec(df, []byte("FROM scratch"), "/ctx-b", "tag-b:latest", true, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildImageSpec: %v", err)
	}

	if a.Labels[app.GeneratorLabelKey] != b.Labels[app.GeneratorLabelKey] {
		t.Errorf("generator label differs across build mechanics: %q vs %q",
			a.Labels[app.GeneratorLabelKey], b.Labels[app.GeneratorLabelKey])
	}
}
