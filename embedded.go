// Package vibrator exposes assets embedded at build time.
//
// This file lives at the module root because //go:embed paths cannot ascend
// above the directive — and the catalog/ + templates/ directories should
// live at the repository root where contributors expect to find them. The
// alternative (moving catalog/ into internal/catalog/data/) would hurt
// discoverability for the most-edited part of the codebase.
//
// Other packages depend on this one by importing github.com/wlame/vibrator
// and using the exported FS variables. The catalog loader, for example, is
// invoked as `catalog.LoadAll(vibrator.CatalogFS)`.
package vibrator

import "embed"

// CatalogFS holds the curated per-harness catalog of plugins, MCP servers,
// skills, subagents, and tools. Layout:
//
//	catalog/<harness>/<id>.md     — markdown + YAML frontmatter
//
// Pass to internal/catalog.LoadAll to materialize the entry set. The fs.FS
// indirection means tests can substitute a synthetic filesystem
// (testing/fstest.MapFS) without going through the embed at all.
//
//go:embed catalog
var CatalogFS embed.FS
