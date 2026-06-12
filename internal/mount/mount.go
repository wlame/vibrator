// Package mount parses, validates, and fingerprints the user's extra
// host-folder mount requests (the --mount flag / .vb `mounts` list).
//
// It is intentionally PURE: no import of internal/docker and no container
// knowledge. The app layer maps a Resolved into a docker bind mount and
// into the harness's --add-dir arguments. Mounts always bind at the same
// absolute path on host and container, so a Resolved carries a single Path.
package mount

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Spec is a parsed --mount entry before path resolution. HostPath is the
// raw path exactly as the user typed it (possibly relative); ReadOnly is
// derived from the optional :ro/:rw suffix (read-only is the default).
type Spec struct {
	HostPath string
	ReadOnly bool
}

// Parse turns a raw "PATH[:ro|:rw]" entry into a Spec. The mode suffix is
// recognized only when it is exactly ":ro" or ":rw"; any other ":..." tail
// is an error (a typo must never be silently folded into the path). A bare
// path is read-only. An empty path is an error.
func Parse(raw string) (Spec, error) {
	path, readOnly := raw, true
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		switch raw[i+1:] {
		case "ro":
			path, readOnly = raw[:i], true
		case "rw":
			path, readOnly = raw[:i], false
		default:
			return Spec{}, fmt.Errorf("--mount %q: unknown mode suffix %q (use :ro or :rw)", raw, raw[i+1:])
		}
	}
	if path == "" {
		return Spec{}, fmt.Errorf("--mount %q: empty path", raw)
	}
	return Spec{HostPath: path, ReadOnly: readOnly}, nil
}

// Resolved is a validated, absolute mount. Path is the same on host and
// container (vibrator mounts every folder at its host path so paths in
// errors, stack traces, and `pwd` match the user's muscle memory).
type Resolved struct {
	Path     string
	ReadOnly bool
}

// ResolveAll parses each raw entry, resolves it to an absolute, cleaned
// path (relative paths are taken against the current working directory),
// and validates that it exists and is a directory. It is fail-fast: the
// first bad entry returns an error and no container action should proceed.
//
// Two cleanups make the result canonical and docker-safe:
//   - a path equal to wsDir is dropped (the workspace is already mounted;
//     re-binding the same path is a docker error), with no error.
//   - exact duplicates collapse; the same path requested with conflicting
//     read-only/read-write modes is an error (ambiguous intent).
func ResolveAll(raws []string, wsDir string) ([]Resolved, error) {
	wsAbs, _ := filepath.Abs(wsDir)
	seen := make(map[string]bool) // path -> readOnly
	var out []Resolved

	for _, raw := range raws {
		spec, err := Parse(raw)
		if err != nil {
			return nil, err
		}
		abs, err := filepath.Abs(spec.HostPath)
		if err != nil {
			return nil, fmt.Errorf("--mount %q: %w", raw, err)
		}
		abs = filepath.Clean(abs)

		if abs == filepath.Clean(wsAbs) {
			continue // workspace already mounted
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("--mount %s: no such directory", abs)
			}
			return nil, fmt.Errorf("--mount %s: %w", abs, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("--mount %s: not a directory", abs)
		}
		if was, ok := seen[abs]; ok {
			if was != spec.ReadOnly {
				return nil, fmt.Errorf("--mount %s: requested both read-only and read-write", abs)
			}
			continue // exact duplicate
		}
		seen[abs] = spec.ReadOnly
		out = append(out, Resolved{Path: abs, ReadOnly: spec.ReadOnly})
	}
	return out, nil
}

// Fingerprint returns a short, stable hash of a resolved mount set, or ""
// when the set is empty. It is order-independent (entries are sorted) so
// the same set in any order yields the same value. The app layer stamps
// this on the container as a label and recreates the container when it
// changes — bind mounts can't be added to a live container.
func Fingerprint(rs []Resolved) string {
	if len(rs) == 0 {
		return ""
	}
	lines := make([]string, len(rs))
	for i, r := range rs {
		mode := "rw"
		if r.ReadOnly {
			mode = "ro"
		}
		lines[i] = r.Path + ":" + mode
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:8])
}
