// Package workspace computes the identity of a workspace + spec combination:
// a stable 8-char fingerprint, the Docker image tag, and the per-workspace
// container name.
//
// Two principles drive the design:
//
//  1. Same logical input always produces the same identity, regardless of the
//     order features or catalog entries were enabled. Achieved by sorting +
//     canonical-form serialization before hashing.
//
//  2. Different harness/profile/shell/feature/catalog combinations produce
//     distinct image tags AND distinct container names. So running Claude
//     Code "backend" and Codex "backend" in the same workspace gets two
//     containers, not one over-mounted shared one.
//
// "Profile" is intentionally NOT included in the fingerprint — it's a
// shorthand for a feature bundle. Including it would create artificial
// differences between e.g. `--profile=full` (explicit) and the default
// (which resolves to `full`).
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Spec is the set of user choices that determine the image identity for a
// build. Spec values are normalized (sorted, lower-cased where appropriate)
// before fingerprinting.
type Spec struct {
	Harness  string   // e.g. "claude-code"
	Profile  string   // user-facing label only — NOT part of fingerprint
	Shell    string   // "bash" | "zsh" | "fish"
	Features []string // resolved final feature list (after profile + with + no)
	Catalog  []string // resolved catalog selections (per-harness IDs)
}

// Fingerprint returns an 8-character hex prefix of SHA-256 over the spec's
// canonical form. Two specs that differ only in field order produce the same
// fingerprint.
//
// A wholly empty spec (no harness, no shell, no features, no catalog) returns
// "00000000" — a sentinel that's easy to spot in image tags and won't collide
// with real SHA-256 prefixes in practice.
func Fingerprint(spec Spec) string {
	if isEmptySpec(spec) {
		return "00000000"
	}
	canonical := canonicalSpec(spec)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])[:8]
}

// ImageName returns the docker image tag for the given spec + fingerprint.
// Format: `vb-<harness>-<profile>-<fp8>:latest`.
//
// Harness and profile are sanitized to safe tag chars; fingerprint is hex
// so always safe. The :latest suffix is intentional — image versioning is
// per-workspace by fingerprint, not by floating tag.
func ImageName(spec Spec, fingerprint string) string {
	harness := sanitizeTagSegment(spec.Harness)
	profile := sanitizeTagSegment(spec.Profile)
	if harness == "" {
		harness = "unknown"
	}
	if profile == "" {
		profile = "default"
	}
	return fmt.Sprintf("vb-%s-%s-%s:latest", harness, profile, fingerprint)
}

// ContainerName returns the per-workspace container name for a given
// workspace path and fingerprint.
// Format: `vb-<sanitized-basename>-<wsHash8>-<fp8>`.
//
// wsHash8 is the SHA-256 prefix of the absolute workspace path; it
// disambiguates workspaces with the same basename (e.g., ~/work/foo and
// ~/play/foo). Without it the two would alias on the same container name.
func ContainerName(workspacePath, fingerprint string) string {
	abs := workspacePath
	if a, err := filepath.Abs(workspacePath); err == nil {
		abs = a
	}
	basename := sanitizeNameSegment(filepath.Base(abs))
	if basename == "" {
		basename = "ws"
	}
	wsHash := hashHex(abs, 8)
	return fmt.Sprintf("vb-%s-%s-%s", basename, wsHash, fingerprint)
}

// canonicalSpec produces a deterministic string representation of a spec.
// Fields appear in fixed order; lists are sorted; case-insensitive fields
// are lower-cased. Anything that doesn't affect on-disk image content is
// excluded — currently that's just Profile.
func canonicalSpec(spec Spec) string {
	features := append([]string(nil), spec.Features...)
	sort.Strings(features)
	catalog := append([]string(nil), spec.Catalog...)
	sort.Strings(catalog)

	// Lowercase the small enums so e.g. "Zsh" and "zsh" produce the same hash.
	harness := strings.ToLower(strings.TrimSpace(spec.Harness))
	shell := strings.ToLower(strings.TrimSpace(spec.Shell))

	var b strings.Builder
	fmt.Fprintf(&b, "harness=%s;", harness)
	fmt.Fprintf(&b, "shell=%s;", shell)
	fmt.Fprintf(&b, "features=%s;", strings.Join(features, ","))
	fmt.Fprintf(&b, "catalog=%s", strings.Join(catalog, ","))
	return b.String()
}

func isEmptySpec(spec Spec) bool {
	return strings.TrimSpace(spec.Harness) == "" &&
		strings.TrimSpace(spec.Shell) == "" &&
		len(spec.Features) == 0 &&
		len(spec.Catalog) == 0
}

// sanitizeTagSegment normalizes a string for use in a docker image tag.
// Allowed: a-z 0-9 . _ - . Everything else becomes '-'. Result is lower-cased.
// Docker tag rules: max 128 chars, [a-zA-Z0-9_.-], can't start with . or -.
func sanitizeTagSegment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	out = strings.TrimLeft(out, ".-")
	return out
}

// sanitizeNameSegment normalizes a string for use in a docker container name.
// Allowed: a-z A-Z 0-9 _ -. Container names are case-preserving but we lower
// for consistency with image tags. Same dot/dash-leading rule.
func sanitizeNameSegment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	out = strings.TrimLeft(out, "-_")
	return out
}

// hashHex returns the first n hex chars of sha256(s).
func hashHex(s string, n int) string {
	sum := sha256.Sum256([]byte(s))
	full := hex.EncodeToString(sum[:])
	if n > len(full) {
		n = len(full)
	}
	return full[:n]
}
