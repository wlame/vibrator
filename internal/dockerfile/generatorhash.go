package dockerfile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"

	vibrator "github.com/wlame/vibrator"
)

// GeneratorHash fingerprints everything vibrator itself contributes to an
// image: the generated Dockerfile for this spec plus every embedded
// template file (entrypoint, shells, scripts — they are COPY'd via the
// build context, so Dockerfile bytes alone would miss them). Images carry
// it as the "vibrator.generator" label; at launch the same hash is
// recomputed and compared, which is how a vibrate upgrade that changes
// generator output gets detected on old images.
//
// VibratorVersion is zeroed before generating so release stamping alone
// (identical content, new version string) never flags an image stale.
func GeneratorHash(spec Spec) (string, error) {
	spec.VibratorVersion = ""
	out, err := Generate(spec)
	if err != nil {
		return "", fmt.Errorf("generator hash: %w", err)
	}

	h := sha256.New()
	h.Write(out)

	if err := hashTemplateFiles(h, vibrator.TemplatesFS, "templates"); err != nil {
		return "", fmt.Errorf("generator hash: walk templates: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

// hashTemplateFiles walks fsys under root and feeds every SHIPPED file's
// path + contents into w (in fs.WalkDir's deterministic lexical order — no
// explicit sorting needed). "Shipped" means skipTemplateFile says no —
// the exact same predicate extractTemplatesTo (buildcontext.go) uses to
// decide what actually reaches the docker build context. That sharing is
// load-bearing: without it, GeneratorHash would fingerprint files (like
// templates/README.md) that never reach the built image, so editing one
// would flip the hash and trigger a permanent false staleness warning.
//
// Takes fsys/root as parameters — rather than hardcoding
// vibrator.TemplatesFS — so tests can exercise the skip behavior against an
// in-memory fs.FS without depending on (or mutating) the real embedded tree.
func hashTemplateFiles(w io.Writer, fsys fs.FS, root string) error {
	return fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if skipTemplateFile(d.Name()) {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(path)); err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		return nil
	})
}
