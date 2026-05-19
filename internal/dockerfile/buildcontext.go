package dockerfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	vibrator "github.com/wlame/vibrator"
)

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
//   - Files matching templateContextSkip (README.md, etc.) are filtered
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
	return dir, cleanup, nil
}

// templateContextSkip lists path components inside templates/ that
// should NOT be copied into the build context. README files exist for
// human readers, not the container. `.gitkeep` is a git placeholder
// for empty dirs. Both end in noise inside the image if shipped.
var templateContextSkip = map[string]struct{}{
	"README.md": {},
	".gitkeep":  {},
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
		if _, skip := templateContextSkip[d.Name()]; skip {
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

