// Package feature defines the build-time capability layers that compose into
// a vibrator Docker image: language toolchains (python, go, node), browser
// runtime (playwright), CLIs (gh, postgres-client), and tool bundles
// (audit-toolkit).
//
// Features are coarse, image-shaping units. Catalog entries (plugins, MCP
// servers, skills — see internal/catalog) reference features as deps to
// declare "I need Node.js in the image". The Resolve function walks those
// declarations and auto-enables transitive deps with explicit warnings.
//
// Each Feature carries its Dockerfile fragment inline. Fragments may be
// arbitrary Dockerfile directives (RUN, COPY --from, ENV, ...) — not just
// RUN — because several features need multi-stage COPYs (Go, Node, Bun).
package feature

import (
	"fmt"
	"sort"
)

// Feature is a single image capability layer. Identified by ID; the rest is
// documentation, image-sizing hints, and the Dockerfile fragment emitted by
// the generator when the feature is enabled.
type Feature struct {
	ID          string
	Name        string   // display label, e.g. "Python 3.13"
	Description string   // one-line summary for the wizard
	SizeMB      int      // approximate image-size impact (best-effort)
	Deps        []string // direct feature IDs required by this one

	// Dockerfile is the verbatim fragment emitted when this feature is
	// enabled. Multi-line; should be self-contained — including the
	// `&& rm -rf /var/lib/apt/lists/*` cleanup at the end of any apt-get
	// invocation. The generator appends a leading "# --- feature: X ---"
	// banner, so don't include that here.
	Dockerfile string
}

// Registry is the canonical list of features. Order is significant — it
// drives wizard rendering and explain output, and dictates the emit order
// in the generated Dockerfile. Keep dependency-friendly order: dependencies
// before their dependents (so transitive deps get installed first).
var Registry = []Feature{
	{
		ID:          "python",
		Name:        "Python 3.13",
		Description: "Python 3.13 via uv; pulls a pre-built CPython from python-build-standalone.",
		SizeMB:      100,
		Dockerfile: `# uv (Python package manager) is the installer for Python itself.
# Install it system-wide so later RUN steps in features that depend on
# Python ("uv tool install ...") all share the same uv binary.
RUN curl -LsSf --retry 3 --retry-delay 5 --retry-all-errors https://astral.sh/uv/install.sh \
      | UV_INSTALL_DIR=/usr/local/bin sh \
 && uv --version
RUN uv python install 3.13 \
 && ln -sf "$(uv python find 3.13)" /usr/local/bin/python3.13 \
 && update-alternatives --install /usr/bin/python3 python3 /usr/local/bin/python3.13 100 \
 && python3 --version`,
	},
	{
		ID:          "go",
		Name:        "Go toolchain",
		Description: "Go compiler + standard library (latest stable).",
		SizeMB:      200,
		// Multi-stage copy avoids hitting go.dev/dl through dl.google.com
		// (commonly blocked in build environments). Architecture-aware via
		// the official multi-arch golang image.
		Dockerfile: `COPY --from=golang:1.26.2 /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"
RUN go version`,
	},
	{
		ID:          "node",
		Name:        "Node.js + Bun",
		Description: "Node.js 22 + Bun. Required by most JS-based MCP servers (claude-mem, playwright).",
		SizeMB:      150,
		// Same multi-stage copy approach — official images are multi-arch.
		// We need npm + npx CLI shims pointing at the embedded npm script.
		Dockerfile: `COPY --from=oven/bun:1 /usr/local/bin/bun /usr/local/bin/bun
COPY --from=node:22-bookworm-slim /usr/local/bin/node /usr/local/bin/node
COPY --from=node:22-bookworm-slim /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -sf /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && ln -sf /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
 && node --version && npm --version && bun --version`,
	},
	{
		ID:          "playwright",
		Name:        "Playwright + Chromium",
		Description: "Chromium binary + Playwright MCP for browser automation.",
		SizeMB:      500,
		Deps:        []string{"node"},
		// Playwright depends on a long list of system libs for Chromium.
		// Keep the apt list current with upstream's recommendation.
		Dockerfile: `RUN apt-get update && apt-get install -y --no-install-recommends \
      libnss3 libnspr4 libdbus-1-3 libdrm2 libxkbcommon0 libxcomposite1 \
      libxdamage1 libxfixes3 libxrandr2 libgbm1 libxshmfence1 libxss1 \
      libatk1.0-0 libatk-bridge2.0-0 libatspi2.0-0 libcups2 libpangocairo-1.0-0 \
      libpango-1.0-0 libcairo2 libasound2-data libasound2t64 \
 && rm -rf /var/lib/apt/lists/*
RUN npm install -g @playwright/mcp \
 && npx --yes playwright install chromium --with-deps || true`,
	},
	{
		ID:          "postgres-client",
		Name:        "Postgres client",
		Description: "psql, pg_dump, pg_restore — for talking to host-side Postgres (e.g., claude-mem).",
		SizeMB:      30,
		Dockerfile: `RUN apt-get update && apt-get install -y --no-install-recommends postgresql-client \
 && rm -rf /var/lib/apt/lists/* \
 && psql --version`,
	},
	{
		ID:          "gh",
		Name:        "GitHub CLI",
		Description: "GitHub CLI for PRs, issues, releases — installs from the official apt repo.",
		SizeMB:      20,
		Dockerfile: `RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
      | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
 && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
      | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
 && apt-get update && apt-get install -y --no-install-recommends gh \
 && rm -rf /var/lib/apt/lists/* \
 && gh --version`,
	},
	{
		ID:          "audit-toolkit",
		Name:        "Production audit toolkit",
		Description: "trivy, syft, grype, semgrep, gitleaks, trufflehog, osv-scanner, checkov, dockle, scc, lizard.",
		SizeMB:      400,
		Deps:        []string{"python"},
		// Pin all third-party tool versions for supply-chain stability.
		// Semgrep + the python-based tools install via uv.
		Dockerfile: `# trivy
RUN ARCH=$(dpkg --print-architecture) && \
    TRIVY_VERSION="0.70.0" && \
    if [ "$ARCH" = "amd64" ]; then TV_ARCH="64bit"; else TV_ARCH="ARM64"; fi && \
    curl -fsSL --retry 3 --retry-delay 5 \
      "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-${TV_ARCH}.tar.gz" \
      -o trivy.tar.gz && \
    tar -xzf trivy.tar.gz trivy && \
    mv trivy /usr/local/bin/ && chmod +x /usr/local/bin/trivy && \
    rm trivy.tar.gz
# syft + grype
RUN ARCH=$(dpkg --print-architecture) && \
    SYFT_VERSION="1.44.0" && GRYPE_VERSION="0.111.1" && \
    curl -fsSL "https://github.com/anchore/syft/releases/download/v${SYFT_VERSION}/syft_${SYFT_VERSION}_linux_${ARCH}.tar.gz" \
      -o syft.tar.gz && tar -xzf syft.tar.gz syft && mv syft /usr/local/bin/ && rm syft.tar.gz && \
    curl -fsSL "https://github.com/anchore/grype/releases/download/v${GRYPE_VERSION}/grype_${GRYPE_VERSION}_linux_${ARCH}.tar.gz" \
      -o grype.tar.gz && tar -xzf grype.tar.gz grype && mv grype /usr/local/bin/ && rm grype.tar.gz
# gitleaks + trufflehog + osv-scanner
RUN ARCH=$(dpkg --print-architecture) && \
    GITLEAKS_VERSION="8.30.1" && \
    if [ "$ARCH" = "amd64" ]; then GL_ARCH="x64"; else GL_ARCH="arm64"; fi && \
    curl -fsSL "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_linux_${GL_ARCH}.tar.gz" \
      -o gitleaks.tar.gz && tar -xzf gitleaks.tar.gz gitleaks && mv gitleaks /usr/local/bin/ && rm gitleaks.tar.gz && \
    curl -sSfL https://raw.githubusercontent.com/trufflesecurity/trufflehog/main/scripts/install.sh \
      | sh -s -- -b /usr/local/bin && \
    OSV_VERSION="2.3.6" && \
    curl -fsSL "https://github.com/google/osv-scanner/releases/download/v${OSV_VERSION}/osv-scanner_linux_${ARCH}" \
      -o /usr/local/bin/osv-scanner && chmod +x /usr/local/bin/osv-scanner
# Python-based audit tools via uv (shared venvs in ~/.local/bin)
RUN uv tool install semgrep \
 && uv tool install bandit \
 && uv tool install pip-audit \
 && uv tool install detect-secrets \
 && uv tool install checkov`,
	},
	{
		ID:          "codex-cli",
		Name:        "OpenAI Codex CLI",
		Description: "Codex binary for cross-model code review (used by /planning:exec).",
		SizeMB:      30,
		Deps:        []string{"node"},
		Dockerfile:  `RUN npm install -g @openai/codex && codex --version`,
	},
	{
		ID:          "ralphex",
		Name:        "ralphex",
		Description: "Autonomous coding loop — executes implementation plans task-by-task in fresh sessions.",
		SizeMB:      20,
		Dockerfile: `RUN ARCH=$(dpkg --print-architecture) && \
    RALPHEX_VERSION="1.0.1" && \
    curl -fsSL --retry 3 --retry-delay 5 \
      "https://github.com/umputun/ralphex/releases/download/v${RALPHEX_VERSION}/ralphex_${RALPHEX_VERSION}_linux_${ARCH}.tar.gz" \
      -o ralphex.tar.gz && \
    tar -xzf ralphex.tar.gz && \
    mv ralphex /usr/local/bin/ && chmod +x /usr/local/bin/ralphex && \
    rm ralphex.tar.gz && \
    ralphex --version`,
	},
	{
		ID:          "aider",
		Name:        "aider AI pair programming",
		Description: "aider-chat via uv tool install. Opt-in — not part of any default profile.",
		SizeMB:      80,
		Deps:        []string{"python"},
		Dockerfile:  `RUN uv tool install aider-chat`,
	},
}

// indexByID lets ByID / IsKnown / Resolve avoid linear scans.
var indexByID = func() map[string]Feature {
	m := make(map[string]Feature, len(Registry))
	for _, f := range Registry {
		m[f.ID] = f
	}
	return m
}()

// ByID returns the feature with the given ID. The bool is false when the ID
// is unknown — callers should treat that as a user error and surface the
// list of valid IDs.
func ByID(id string) (Feature, bool) {
	f, ok := indexByID[id]
	return f, ok
}

// IsKnown reports whether id is a registered feature.
func IsKnown(id string) bool {
	_, ok := indexByID[id]
	return ok
}

// IDs returns all known feature IDs in Registry order.
func IDs() []string {
	out := make([]string, len(Registry))
	for i, f := range Registry {
		out[i] = f.ID
	}
	return out
}

// ResolveResult holds the outcome of dependency resolution. AutoEnabled is
// the subset of Enabled that was implicitly turned on to satisfy a dep,
// suitable for emitting a "feature X required Y, auto-enabling" warning.
type ResolveResult struct {
	Enabled     []string // final enabled set, in Registry order
	AutoEnabled []string // IDs implicitly enabled to satisfy deps, sorted
}

// Resolve computes the final feature set from a profile's initial features
// plus user `with` additions, minus `no` removals, transitively pulling in
// any missing deps. Missing deps are auto-enabled with the AutoEnabled
// signal so the caller can warn.
//
// Conflict policy: when `--no=X --with=Y` and Y depends on X, X gets
// auto-re-enabled. The bash version did the same — auto-enabling deps is
// considered more user-friendly than failing. To truly disable X, the user
// must also pass `--no=Y`.
//
// The returned Enabled list is sorted by Registry order — so dependencies
// always emit before their dependents in the generated Dockerfile.
//
// Returns an error if any ID in initial/with/no is unknown.
func Resolve(initial, with, no []string) (ResolveResult, error) {
	// Validate all IDs up front so we can fail fast with a clear message.
	for _, set := range []struct {
		name string
		ids  []string
	}{
		{"initial", initial},
		{"with", with},
		{"no", no},
	} {
		for _, id := range set.ids {
			if !IsKnown(id) {
				return ResolveResult{}, fmt.Errorf("unknown feature %q in %s (valid: %v)",
					id, set.name, IDs())
			}
		}
	}

	// Apply user choices: start from initial, add `with`, remove `no`.
	enabled := make(map[string]bool, len(initial))
	for _, id := range initial {
		enabled[id] = true
	}
	for _, id := range with {
		enabled[id] = true
	}
	for _, id := range no {
		delete(enabled, id)
	}

	// Transitive dep resolution. We iterate until nothing new gets added,
	// because a dep may itself have deps. Track which IDs were auto-enabled
	// (i.e., not in the post-with/no set but pulled in by a dep walk).
	autoSet := make(map[string]bool)
	for {
		changed := false
		for id := range enabled {
			f, _ := indexByID[id] // safe: validated above
			for _, dep := range f.Deps {
				if !enabled[dep] {
					enabled[dep] = true
					autoSet[dep] = true
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	// Emit in Registry order so dependencies always come before dependents
	// in the resulting list. The generator relies on this for deterministic,
	// dep-friendly Dockerfile output.
	var enabledOrdered []string
	for _, f := range Registry {
		if enabled[f.ID] {
			enabledOrdered = append(enabledOrdered, f.ID)
		}
	}

	return ResolveResult{
		Enabled:     enabledOrdered,
		AutoEnabled: sortedKeys(autoSet),
	}, nil
}

// sortedKeys returns the keys of m sorted lexicographically.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
