// Package vibrator exposes assets embedded at build time.
//
// This file lives at the module root because //go:embed paths cannot ascend
// above the directive — and the extensions/ + templates/ directories should
// live at the repository root where contributors expect to find them. The
// alternative (moving extensions/ into internal/extensions/data/) would hurt
// discoverability for the most-edited part of the codebase.
//
// Other packages depend on this one by importing github.com/wlame/vibrator
// and using the exported FS variables. The extensions loader, for example,
// is invoked as `extensions.LoadAll(vibrator.ExtensionsFS)`.
package vibrator

import "embed"

// ExtensionsFS holds the curated per-harness collection of plugins, MCP
// servers, skills, subagents, and other things that extend the agent's
// capabilities inside the container. Layout:
//
//	extensions/<harness>/<id>.md     — markdown + YAML frontmatter
//
// Pass to internal/extensions.LoadAll to materialize the entry set. The
// fs.FS indirection means tests can substitute a synthetic filesystem
// (testing/fstest.MapFS) without going through the embed at all.
//
//go:embed extensions
var ExtensionsFS embed.FS

// TemplatesFS holds static files that get copied into the container
// image at docker-build time: shell rc files, the welcome banner,
// entrypoint scripts. Layout:
//
//	templates/shells/{bashrc,zshrc,config.fish}
//	templates/scripts/{welcome.sh,entrypoint.sh,claude-exec.sh}
//
// Consumed by internal/dockerfile.PrepareBuildContext, which extracts
// the tree into a per-build tempdir; the generated Dockerfile then
// COPYs from there. See templates/README.md for the editing
// conventions.
//
//go:embed templates
var TemplatesFS embed.FS
