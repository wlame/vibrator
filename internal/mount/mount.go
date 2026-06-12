// Package mount parses, validates, and fingerprints the user's extra
// host-folder mount requests (the --mount flag / .vb `mounts` list).
//
// It is intentionally PURE: no import of internal/docker and no container
// knowledge. The app layer maps a Resolved into a docker bind mount and
// into the harness's --add-dir arguments. Mounts always bind at the same
// absolute path on host and container, so a Resolved carries a single Path.
package mount

import (
	"fmt"
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
