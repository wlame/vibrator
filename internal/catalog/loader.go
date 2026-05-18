package catalog

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// catalogRoot is the directory name we expect harness subdirectories to live
// under. Loaders interpret this as the prefix inside the supplied fs.FS.
const catalogRoot = "catalog"

// LoadAll walks fsys looking for "catalog/<harness>/<id>.md" files (exactly
// two levels under catalogRoot, ignoring deeper nesting). Each .md is parsed
// — YAML frontmatter into metadata, post-frontmatter content into Body —
// and validated.
//
// Returns a map keyed by Entry.Key() ("<harness>/<id>"). The first
// malformed file aborts the load with an error citing the file path and
// reason — fail-fast is the right policy here because the catalog ships
// inside the binary; a malformed entry is a build-time bug.
func LoadAll(fsys fs.FS) (map[string]*Entry, error) {
	out := make(map[string]*Entry)

	root, err := fs.Sub(fsys, catalogRoot)
	if err != nil {
		// fs.Sub only errors on a bad path argument, which is statically
		// "catalog" here — should never happen in practice. Surface anyway.
		return nil, fmt.Errorf("catalog: %w", err)
	}

	err = fs.WalkDir(root, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		// Only .md files. Anything else is ignored silently (lets contributors
		// stash .gitkeep, README.md at the catalog root, etc.). README.md is
		// handled in the segment count check below.
		if !strings.HasSuffix(p, ".md") {
			return nil
		}

		// Expect exactly "<harness>/<id>.md" — anything else (e.g. a stray
		// .md at the catalog root, or three-level nesting) is skipped.
		parts := strings.Split(p, "/")
		if len(parts) != 2 {
			return nil
		}
		harness := parts[0]
		id := strings.TrimSuffix(parts[1], ".md")

		// Reject "README.md" and similar conventional names at the harness
		// level — they're documentation, not catalog entries.
		if strings.EqualFold(id, "readme") {
			return nil
		}

		data, err := fs.ReadFile(root, p)
		if err != nil {
			return fmt.Errorf("catalog/%s: read: %w", p, err)
		}

		entry, err := parseEntry(harness, id, data)
		if err != nil {
			return fmt.Errorf("catalog/%s: %w", p, err)
		}

		// Defense in depth: two files producing the same key would silently
		// overwrite each other in the map. Detect and surface.
		if existing, dup := out[entry.Key()]; dup {
			return fmt.Errorf("catalog/%s: duplicate key %q (also from %s/%s.md)",
				p, entry.Key(), existing.Harness, existing.ID)
		}
		out[entry.Key()] = entry
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LoadForHarness returns the entries under catalog/<harness>/, sorted by ID.
// Useful for `vibrate catalog list <harness>` and the wizard's per-harness
// multi-select rendering.
func LoadForHarness(fsys fs.FS, harness string) ([]*Entry, error) {
	all, err := LoadAll(fsys)
	if err != nil {
		return nil, err
	}
	var out []*Entry
	for _, e := range all {
		if e.Harness == harness {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get looks up a single entry by harness + id.
func Get(fsys fs.FS, harness, id string) (*Entry, error) {
	all, err := LoadAll(fsys)
	if err != nil {
		return nil, err
	}
	key := harness + "/" + id
	e, ok := all[key]
	if !ok {
		return nil, fmt.Errorf("catalog: entry %q not found", key)
	}
	return e, nil
}

// Harnesses returns the harness directory names present in the catalog, in
// sorted order. Used for `vibrate catalog list` with no argument (Phase 4
// might extend this) and for validating user input against known harnesses.
func Harnesses(fsys fs.FS) ([]string, error) {
	all, err := LoadAll(fsys)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool)
	for _, e := range all {
		set[e.Harness] = true
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out, nil
}

// parseEntry splits a markdown file into YAML frontmatter + body, decodes the
// frontmatter into an Entry, attaches harness/id/body, and validates.
//
// File shape:
//
//	---
//	yaml: fields
//	---
//	# Markdown body
//	...
//
// Tolerant of leading whitespace before the opening `---`. The closing `---`
// must appear on its own line.
func parseEntry(harness, id string, data []byte) (*Entry, error) {
	content := string(data)

	// Trim a UTF-8 BOM if present (rare but happens with Windows editors).
	content = strings.TrimPrefix(content, "\ufeff")

	// Trim leading whitespace/newlines.
	trimmed := strings.TrimLeft(content, " \t\n\r")
	if !strings.HasPrefix(trimmed, "---") {
		return nil, fmt.Errorf("missing frontmatter delimiter (expected leading `---`)")
	}

	// Strip the opening `---` and any newline immediately after it. We
	// don't enforce the exact newline shape because hand-edited files vary.
	rest := strings.TrimPrefix(trimmed, "---")
	rest = strings.TrimLeft(rest, "\r\n")

	// Find the closing `---` on its own line. Anchored search via a literal
	// "\n---" lookup followed by validation that what follows is a line
	// terminator (or EOF).
	closeIdx := -1
	for i := 0; i < len(rest); i++ {
		// Find "\n---" candidates.
		if rest[i] != '\n' {
			continue
		}
		if i+4 > len(rest) {
			break
		}
		if rest[i+1:i+4] != "---" {
			continue
		}
		// Confirm it's a standalone line (next char is \n, \r, or EOF).
		afterIdx := i + 4
		if afterIdx == len(rest) {
			closeIdx = i + 1
			break
		}
		if rest[afterIdx] == '\n' || rest[afterIdx] == '\r' {
			closeIdx = i + 1
			break
		}
	}
	if closeIdx < 0 {
		return nil, fmt.Errorf("missing closing `---` for frontmatter block")
	}

	yamlBytes := []byte(rest[:closeIdx])
	body := rest[closeIdx+3:] // skip the "---"
	body = strings.TrimLeft(body, "\r\n")

	var e Entry
	if err := yaml.Unmarshal(yamlBytes, &e); err != nil {
		return nil, fmt.Errorf("YAML frontmatter: %w", err)
	}

	// If the file's frontmatter declares an `id:`, it must agree with the
	// filename. The filename is authoritative; the frontmatter field is
	// optional belt-and-braces. Mismatch is a build-time bug, not a
	// runtime user error — surface it.
	if e.ID != "" && e.ID != id {
		return nil, fmt.Errorf("frontmatter id %q disagrees with filename id %q", e.ID, id)
	}
	e.ID = id
	e.Harness = harness
	e.Body = strings.TrimRight(body, "\n") + "\n"

	if err := validate(&e); err != nil {
		return nil, err
	}
	return &e, nil
}

// validate enforces the required-field contract for an entry. Optional
// fields are left to the consumer to handle.
func validate(e *Entry) error {
	if e.Name == "" {
		return fmt.Errorf("`name` is required")
	}
	if !e.Kind.Valid() {
		return fmt.Errorf("`kind` must be one of %v (got %q)", AllKinds, e.Kind)
	}
	if e.Source == "" {
		return fmt.Errorf("`source` is required for traceability")
	}
	// Cross-package validation: feature IDs in deps must reference real
	// features. We can't import internal/feature here without creating a
	// cycle (feature → ... → catalog), so this check lives at the package
	// boundary — call ValidateAgainstFeatures from a higher package.
	_ = path.Clean // silence the unused-import linter if path becomes unused
	return nil
}

// ValidateAgainstFeatures cross-checks that every catalog entry's
// Deps.Features list references known feature IDs. Run at startup or in
// tests; if this fails, the catalog ships with a broken dep declaration.
//
// known is a set-membership predicate (e.g., feature.IsKnown). Passing it
// in keeps internal/catalog free of an internal/feature import.
func ValidateAgainstFeatures(entries map[string]*Entry, known func(string) bool) error {
	for _, e := range entries {
		for _, f := range e.Deps.Features {
			if !known(f) {
				return fmt.Errorf("catalog entry %s declares unknown feature dep %q", e.Key(), f)
			}
		}
	}
	return nil
}
