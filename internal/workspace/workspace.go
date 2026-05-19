// Package workspace computes the identity of a workspace + spec combination:
// a stable 8-char fingerprint, the Docker image tag, and the per-workspace
// container name.
//
// Two principles drive the design:
//
//  1. Same logical input always produces the same identity, regardless of the
//     order features or extension entries were enabled. Achieved by sorting +
//     canonical-form serialization before hashing.
//
//  2. Different harness/profile/shell/feature/extensions combinations produce
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
	"os"
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
	Extensions  []string // resolved extensions selections (per-harness IDs)

	// Username is the unprivileged user baked into the image. Affects
	// fingerprint AND appears as a visible segment in the image tag, so
	// multi-user hosts don't share images (the image's USER stage hard-
	// codes the user's UID/GID — sharing across UIDs is incorrect).
	Username string
}

// Fingerprint returns an 8-character hex prefix of SHA-256 over the spec's
// canonical form. Two specs that differ only in field order produce the same
// fingerprint.
//
// A wholly empty spec (no harness, no shell, no features, no extensions) returns
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
// Format: `vb-<harness>-<profile>-<user>-<fp8>:latest` when a username is
// set; `vb-<harness>-<profile>-<fp8>:latest` otherwise (back-compat for
// callers that don't yet pass Username).
//
// Harness, profile, and username are sanitized to safe tag chars;
// fingerprint is hex so always safe. The :latest suffix is intentional —
// image versioning is per-workspace by fingerprint, not by floating tag.
//
// Username is included to prevent multi-user-host image collisions: the
// image's USER stage hard-codes a specific UID/GID, so two users
// sharing an image tag would end up with one rebuilding over the other's
// (when their UIDs differ) — surprising and wasteful. Different users
// get distinct image tags by construction.
func ImageName(spec Spec, fingerprint string) string {
	harness := sanitizeTagSegment(spec.Harness)
	profile := sanitizeTagSegment(spec.Profile)
	user := sanitizeTagSegment(spec.Username)
	if harness == "" {
		harness = "unknown"
	}
	if profile == "" {
		profile = "default"
	}
	if user != "" {
		return fmt.Sprintf("vb-%s-%s-%s-%s:latest", harness, profile, user, fingerprint)
	}
	return fmt.Sprintf("vb-%s-%s-%s:latest", harness, profile, fingerprint)
}

// Hostname returns the value passed to `docker run --hostname` for a
// given workspace path. Format: `vibrate-<sanitized-basename>`. The
// "vibrate-" prefix is what makes the shell prompt visibly different
// from a host shell — users can spot at a glance that they're inside
// a container, replacing the older `[vb]` PS1 prefix.
//
// RFC 1123: a hostname label is letters, digits, and hyphens; can't
// start or end with a hyphen; max 63 chars per label. We sanitize
// accordingly. Empty basename falls back to "vibrate-workspace".
func Hostname(workspacePath string) string {
	abs := workspacePath
	if a, err := filepath.Abs(workspacePath); err == nil {
		abs = a
	}
	base := sanitizeHostnameSegment(filepath.Base(abs))
	if base == "" {
		base = "workspace"
	}
	out := "vibrate-" + base
	// Truncate to 63 chars (RFC 1123 single-label limit). The "vibrate-"
	// prefix is 8 chars so the basename portion can be up to 55 chars.
	if len(out) > 63 {
		out = out[:63]
	}
	// Trim a trailing hyphen if truncation left one — invalid per RFC.
	out = strings.TrimRight(out, "-")
	return out
}

// ContainerName returns the per-workspace container name for a given
// workspace path and fingerprint.
// Format: `vb-<sanitized-basename>-<wsHash8>-<fp8>`.
//
// wsHash8 is a hash of the absolute workspace path AND (when uid > 0)
// the host UID; that combination disambiguates workspaces with the
// same basename (e.g., ~/work/foo vs ~/play/foo) AND prevents
// multi-user collisions (alice's ~/dev/foo and bob's ~/dev/foo are
// distinct containers, not one stolen across UIDs). Without it the
// host UID, today's container ownership semantics would let a second
// user's `vibrate` reuse the first user's container — which would
// leak files cross-user and surface the wrong USER inside.
func ContainerName(workspacePath, fingerprint string) string {
	abs := workspacePath
	if a, err := filepath.Abs(workspacePath); err == nil {
		abs = a
	}
	basename := sanitizeNameSegment(filepath.Base(abs))
	if basename == "" {
		basename = "ws"
	}
	// Mix host UID into the hash. os.Getuid() is cheap and process-
	// scoped; using it here rather than threading uid through the call
	// chain keeps the signature unchanged.
	wsHash := hashHex(fmt.Sprintf("%s|uid=%d", abs, os.Getuid()), 8)
	return fmt.Sprintf("vb-%s-%s-%s", basename, wsHash, fingerprint)
}

// canonicalSpec produces a deterministic string representation of a spec.
// Fields appear in fixed order; lists are sorted; case-insensitive fields
// are lower-cased. Anything that doesn't affect on-disk image content is
// excluded — currently that's just Profile.
//
// Username IS included because the image's USER stage embeds the host
// UID/GID, so an image built for alice (uid 501) is materially different
// from one built for bob (uid 502) even with identical features.
func canonicalSpec(spec Spec) string {
	features := append([]string(nil), spec.Features...)
	sort.Strings(features)
	extensions := append([]string(nil), spec.Extensions...)
	sort.Strings(extensions)

	// Lowercase the small enums so e.g. "Zsh" and "zsh" produce the same hash.
	harness := strings.ToLower(strings.TrimSpace(spec.Harness))
	shell := strings.ToLower(strings.TrimSpace(spec.Shell))
	user := strings.ToLower(strings.TrimSpace(spec.Username))

	var b strings.Builder
	fmt.Fprintf(&b, "harness=%s;", harness)
	fmt.Fprintf(&b, "shell=%s;", shell)
	fmt.Fprintf(&b, "features=%s;", strings.Join(features, ","))
	fmt.Fprintf(&b, "extensions=%s;", strings.Join(extensions, ","))
	fmt.Fprintf(&b, "user=%s", user)
	return b.String()
}

func isEmptySpec(spec Spec) bool {
	return strings.TrimSpace(spec.Harness) == "" &&
		strings.TrimSpace(spec.Shell) == "" &&
		strings.TrimSpace(spec.Username) == "" &&
		len(spec.Features) == 0 &&
		len(spec.Extensions) == 0
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

// sanitizeHostnameSegment normalizes a string for use in a hostname
// label per RFC 1123. Allowed: a-z 0-9 -. NO underscores (RFC forbids
// them even though some resolvers accept them in practice). Result is
// lower-cased; leading/trailing hyphens are stripped.
func sanitizeHostnameSegment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
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
