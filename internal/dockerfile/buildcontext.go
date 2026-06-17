package dockerfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/integration"
)

// IntegrationsManifestFilename is the basename of the per-harness
// integration manifest written into the build context and COPYed into
// the container image at /etc/vibrator/integrations.json. Kept as a
// package-level constant so the dockerfile generator and the build
// runner agree on the same name without string duplication.
const IntegrationsManifestFilename = "integrations.json"

// PrepareBuildContext materializes a per-build docker build context in
// a temp directory. The returned cleanup function MUST be called when
// the build is finished (or has failed) — defer it at the caller.
//
// Contents written to the tempdir:
//
//   - Everything in TemplatesFS (templates/shells/*, templates/scripts/*)
//     — but with the leading "templates/" prefix stripped, so the
//     Dockerfile can `COPY shells/zshrc /etc/skel/.zshrc` instead of
//     `COPY templates/shells/zshrc …` (less noise in the Dockerfile).
//   - An empty integrations.json placeholder — overwritten by
//     WriteIntegrationsManifest once the caller knows the harness.
//     Always present so the Dockerfile's unconditional COPY works
//     even if the caller forgets to fill it in (defense in depth).
//   - Files matched by skipTemplateFile (README.md, etc.) are filtered
//     out — they document the layout for contributors but have no
//     business in the container image.
//
// Why a tempdir at all: docker build's "context" is the directory it
// streams to the daemon. Previously vibrator passed the user's
// workspace (cwd), which (a) sent potentially many megabytes of
// unrelated files to the daemon on every build and (b) gave us no
// place to drop our own template files for COPY. A purpose-built
// tempdir solves both.
func PrepareBuildContext() (string, func(), error) {
	dir, err := os.MkdirTemp("", "vibrator-build-")
	if err != nil {
		return "", nil, fmt.Errorf("create build context tempdir: %w", err)
	}

	cleanup := func() { _ = os.RemoveAll(dir) }

	if err := extractTemplatesTo(dir); err != nil {
		cleanup()
		return "", nil, err
	}

	// Empty manifest placeholder — the dockerfile generator emits an
	// unconditional COPY for this file, so it MUST exist even on the
	// degenerate "no integrations registered for this harness" path.
	if err := os.WriteFile(filepath.Join(dir, IntegrationsManifestFilename), []byte("[]\n"), 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write placeholder manifest: %w", err)
	}
	return dir, cleanup, nil
}

// WriteIntegrationsManifest writes the per-harness integration manifest
// (an array of integration.ManifestEntry) into the build context at
// the well-known filename. Call AFTER PrepareBuildContext and BEFORE
// the docker build runs.
//
// The harness ID filters which Wiring entries apply (Wiring.Harness ==
// harness OR "*"). See internal/integration/manifest.go for the schema
// and templates/scripts/claude-exec.sh for the consumer.
func WriteIntegrationsManifest(ctxDir, harness string) error {
	path := filepath.Join(ctxDir, IntegrationsManifestFilename)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}
	defer f.Close()
	if err := integration.WriteManifest(f, harness); err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	return nil
}

// skipTemplateFile reports whether a file under the embedded templates/
// tree (matched by basename) should NOT reach the shipped image. README
// files exist for human readers, not the container; `.gitkeep` is a git
// placeholder for empty dirs. Both are noise inside the image if shipped.
//
// This is the SINGLE source of truth for "what actually ships" — both
// extractTemplatesTo (below, populates the real docker build context) and
// dockerfile.GeneratorHash (internal/dockerfile/generatorhash.go, fingerprints
// what a vibrate build would produce) call this exact predicate. Without
// that sharing, GeneratorHash would fingerprint files extractTemplatesTo
// filters out — editing templates/README.md would then flip the hash and
// trigger a permanent false "image is stale" warning despite the built
// image being byte-for-byte identical.
func skipTemplateFile(name string) bool {
	switch name {
	case "README.md", ".gitkeep":
		return true
	default:
		return false
	}
}

// extractTemplatesTo walks the embedded `templates/` tree and writes
// each non-skipped file under dst. The "templates/" prefix is stripped
// from every output path so Dockerfile `COPY` directives reference
// `shells/zshrc` rather than `templates/shells/zshrc`.
func extractTemplatesTo(dst string) error {
	const root = "templates"

	return fs.WalkDir(vibrator.TemplatesFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip the root itself; relative path would be ".".
		if path == root {
			return nil
		}
		// Path-component skip — apply to the basename so README.md
		// anywhere in the tree gets filtered.
		if skipTemplateFile(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Compute output path with the "templates/" prefix removed.
		rel := strings.TrimPrefix(path, root+"/")
		out := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}

		data, err := fs.ReadFile(vibrator.TemplatesFS, path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		// Parent dir is created by the directory-walk visit; defensive
		// MkdirAll covers the case where Walk emits files before dirs
		// (it shouldn't, but the cost is negligible).
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		// 0644 for plain files; scripts will need +x via the
		// Dockerfile COPY directive or a chmod RUN step (set per-
		// template at the point of use, not here — we don't know
		// the file's intended permissions from the FS alone).
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		return nil
	})
}
