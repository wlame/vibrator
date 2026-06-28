// Package dockerfile assembles a deterministic Dockerfile from a resolved
// Spec. Same input → byte-identical output, which is what enables:
//
//   - `vibrate build-dockerfile FILE` for inspection / debugging
//   - golden-file tests for regression detection
//   - image-tag fingerprints based on the generated content
//
// The Dockerfile has five logical stages:
//
//  1. base     — Ubuntu 24.04 LTS + always-on substrate (jq, rg, vim, ...)
//  2. features — per-feature install fragments from internal/feature
//  3. harness  — per-harness install from internal/harness
//  4. extensions  — per-entry install snippets from extensions markdown
//  5. runtime  — user setup, labels, CMD
//
// All five stages target the same final image; we use multi-stage tagging
// for clarity in `docker history` and to give layer-cache invalidation
// natural boundaries (feature changes don't rebuild the base toolkit).
package dockerfile

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/feature"
	"github.com/wlame/vibrator/internal/harness"
)

// Spec is the fully-resolved input to the generator. Every field is
// expected to be pre-validated by the caller (CLI / wizard).
type Spec struct {
	// Harness identifies which agent harness gets installed in Stage 3.
	Harness harness.Harness

	// Profile is the named profile the user picked. Informational only —
	// included in the header comment so the generated file documents
	// itself, but not in the layer-content (which is feature-driven).
	Profile string

	// Shell is the default user shell inside the container.
	Shell string

	// Features is the resolved feature list, in Registry order (deps first).
	Features []feature.Feature

	// Extensions are the per-harness extensions selections, in stable
	// (alphabetical-by-ID) order.
	Extensions []*extensions.Entry

	// Username for the unprivileged user inside the container.
	// Defaults to "vibrate" if empty.
	Username string

	// HostUID / HostGID are baked as build-args so file permissions on
	// bind-mounted workspace paths match the host caller.
	HostUID int
	HostGID int

	// VibratorVersion is stamped into a LABEL for traceability.
	VibratorVersion string
}

// buildID is a session-stamped identifier baked into every generated image.
// Update this manually at the start of each vibrator dev session so that
// containers built by outdated images can be identified:
//
//	cat /etc/vibrator/build            # inside the container
//	docker inspect <name> | grep BUILD # from outside
const buildID = "2026-05-27-1558"

// supportedShells documents which Shell values the generator handles.
// Adding a new shell = adding an apt-get line in stageBase + a Cmd switch.
var supportedShells = map[string]struct{}{
	"bash": {},
	"zsh":  {},
	"fish": {},
}

// SupportedShell reports whether s is a shell the generator (and the
// launch path) accepts. Exported so pin validation can reject a bad
// `.vb` shell value BEFORE it reaches any exec path — resolveLaunchCmd
// builds "/bin/"+shell, and container-reuse launches skip Generate's
// own validation entirely.
func SupportedShell(s string) bool {
	_, ok := supportedShells[s]
	return ok
}

// Generate emits the full Dockerfile as bytes. Returns an error on invalid
// spec (unknown shell, missing harness, …) — callers should treat any
// error as a programming bug and surface to the user.
func Generate(spec Spec) ([]byte, error) {
	if err := validate(spec); err != nil {
		return nil, err
	}

	var b bytes.Buffer
	writeHeader(&b, spec)
	writeBaseStage(&b, spec)
	writeFeaturesStage(&b, spec)
	writeHarnessStage(&b, spec)
	if err := writeExtensionsStage(&b, spec); err != nil {
		return nil, err
	}
	writeRuntimeStage(&b, spec)

	return b.Bytes(), nil
}

func validate(spec Spec) error {
	if spec.Harness == nil {
		return fmt.Errorf("dockerfile: Spec.Harness is required")
	}
	if _, ok := supportedShells[spec.Shell]; !ok {
		return fmt.Errorf("dockerfile: unsupported shell %q (supported: bash, zsh, fish)", spec.Shell)
	}
	if err := ValidateUsername(spec.Username); err != nil {
		return err
	}
	return nil
}

// usernameRE is the Linux useradd convention: lowercase letters, digits,
// underscore, dash; must start with a letter or underscore. Anything
// outside it is rejected rather than sanitized — spec.Username is spliced
// into the generated Dockerfile (ARG USERNAME=...), so a permissive value
// would become a literal Dockerfile instruction.
var usernameRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

// ValidateUsername reports whether u is safe to bake into the Dockerfile
// and acceptable to useradd. Empty is allowed — Generate falls back to
// the "vibrate" default.
func ValidateUsername(u string) error {
	if u == "" {
		return nil
	}
	if len(u) > 32 || !usernameRE.MatchString(u) {
		return fmt.Errorf("dockerfile: invalid container username %q (want ^[a-z_][a-z0-9_-]*$, max 32 chars)", u)
	}
	return nil
}

// writeHeader emits the leading comment block. Includes a reproducibility
// hint — the exact CLI invocation that would re-derive this spec.
func writeHeader(b *bytes.Buffer, spec Spec) {
	featureIDs := featureIDsCSV(spec.Features)
	extensionIDs := extensionsIDsCSV(spec.Extensions)

	// BuildKit `# syntax=` directive — MUST be the very first line of the
	// file. Enables heredoc RUN blocks (`RUN <<'EOF' ... EOF`) which we use
	// in the extensions stage so multi-line shell snippets don't get parsed
	// as Dockerfile instructions.
	b.WriteString("# syntax=docker/dockerfile:1.7\n")
	b.WriteString("# Generated by vibrate ")
	b.WriteString(versionOrDev(spec.VibratorVersion))
	b.WriteString(" — do not edit by hand.\n")
	b.WriteString("#\n")
	fmt.Fprintf(b, "# Harness:  %s (%s)\n", spec.Harness.ID(), spec.Harness.Name())
	fmt.Fprintf(b, "# Profile:  %s\n", spec.Profile)
	fmt.Fprintf(b, "# Shell:    %s\n", spec.Shell)
	fmt.Fprintf(b, "# Features: %s\n", featureIDs)
	fmt.Fprintf(b, "# Extensions:  %s\n", extensionIDs)
	b.WriteString("#\n")
	b.WriteString("# Reproduce this Dockerfile with:\n")
	b.WriteString("#   vibrate build-dockerfile")
	fmt.Fprintf(b, " --harness=%s", spec.Harness.ID())
	fmt.Fprintf(b, " --profile=%s", spec.Profile)
	fmt.Fprintf(b, " --shell=%s", spec.Shell)
	if extensionIDs != "(none)" {
		fmt.Fprintf(b, " --extensions=%s", extensionIDs)
	}
	b.WriteString("\n\n")
}

// writeBaseStage emits Stage 1: Ubuntu LTS + always-on substrate. Apt-installs
// the small, universally-useful CLIs. Binary downloads (rg, fd) are pulled in
// later via Stage features so they don't pollute the base if disabled.
func writeBaseStage(b *bytes.Buffer, spec Spec) {
	b.WriteString(`# ============================================================================
# Stage 1 — base: Ubuntu 24.04 LTS + always-on substrate
# ============================================================================
FROM ubuntu:24.04 AS base

ENV DEBIAN_FRONTEND=noninteractive

# Apt cache layer — always-on CLIs that every profile gets. Kept small.
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl wget \
      git gpg openssh-client \
      sudo vim less tree htop \
      jq sqlite3 dnsutils \
      unzip xz-utils \
      build-essential \
      locales \
      zsh-autosuggestions \
      zsh-syntax-highlighting \
      bash-completion \
`)
	// Append the user's chosen shell so it's installed in the base stage.
	// bash is already present on Ubuntu — listing it explicitly is harmless.
	fmt.Fprintf(b, "      %s \\\n", spec.Shell)
	b.WriteString(` && locale-gen en_US.UTF-8 \
 && rm -rf /var/lib/apt/lists/*

`)

	// --- latest git (always present) ---
	// Ubuntu's git is already installed in the apt block above (a safe
	// baseline), but it lags upstream. The official git-core PPA tracks the
	// newest stable git, so we add it and upgrade — every image ships a
	// current git regardless of profile/harness. The agent relies on git for
	// repo operations, so "git is missing/old" should never be a failure mode.
	b.WriteString(`# --- latest git from the official git-core PPA ---
RUN apt-get update \
 && apt-get install -y --no-install-recommends software-properties-common \
 && add-apt-repository -y ppa:git-core/ppa \
 && apt-get update && apt-get install -y --no-install-recommends git \
 && apt-get purge -y software-properties-common && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/* \
 && git --version

`)

	// --- always-on docker CLI client ---
	// The docker CLI (client only, no daemon) is baked into the base image
	// for EVERY variant. This is deliberate: it makes Docker-in-Docker a
	// pure run-time decision. `--dind` only changes whether the host socket
	// is bind-mounted at `docker run` time — it never changes image content,
	// so toggling --dind reuses the existing image instead of triggering a
	// from-scratch rebuild. Without the socket (no --dind), the client is
	// present but simply has nothing to talk to.
	//
	// docker-ce-cli comes from Docker's official apt repo so the version
	// tracks what Colima/Docker Desktop expose. gpg/curl/ca-certificates are
	// already installed above.
	b.WriteString(`# --- docker CLI client (always present; activated at run time via --dind) ---
RUN install -m 0755 -d /etc/apt/keyrings \
 && curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
      | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
 && chmod a+r /etc/apt/keyrings/docker.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
      https://download.docker.com/linux/ubuntu noble stable" \
      > /etc/apt/sources.list.d/docker.list \
 && apt-get update && apt-get install -y --no-install-recommends docker-ce-cli \
 && rm -rf /var/lib/apt/lists/*
# sudo wrapper at /usr/local/bin/docker (earlier in PATH than /usr/bin/docker)
# so docker always runs as root and can access the socket regardless of group
# membership. Needed for VM-based runtimes (Colima, Rancher Desktop) where the
# socket is owned by a group that doesn't map cleanly across the VM boundary.
# The container user has NOPASSWD:ALL sudo, so no password is prompted.
RUN printf '#!/bin/sh\nexec sudo /usr/bin/docker "$@"\n' > /usr/local/bin/docker \
 && chmod 0755 /usr/local/bin/docker \
 && docker --version

`)

	// Shell rc files + welcome banner script. Sourced from templates/
	// in the repo, extracted into the docker build context by
	// PrepareBuildContext. Landing them in /etc/skel/ means the
	// subsequent `useradd -m` (end of Stage 2) copies them into the
	// unprivileged user's home automatically — no per-user RUN needed.
	//
	// All three shells get rc files regardless of which is the user's
	// login shell. Cost is a few KB; benefit is `bash` and `fish`
	// invocations inside a zsh container don't trigger each shell's
	// own first-run wizard.
	b.WriteString(`# --- shell rc files (copied into new-user homes via /etc/skel) ---
COPY shells/bashrc /etc/skel/.bashrc
COPY shells/zshrc /etc/skel/.zshrc
RUN mkdir -p /etc/skel/.config/fish
COPY shells/config.fish /etc/skel/.config/fish/config.fish

# --- shell-agnostic welcome banner (sourced from each rc file) ---
RUN mkdir -p /opt/vibrator
COPY scripts/welcome.sh /opt/vibrator/welcome.sh
RUN chmod 0755 /opt/vibrator/welcome.sh

# --- entrypoint script (wired via ENTRYPOINT in the runtime stage) ---
# Runs once on 'docker run' to merge host Claude config/rules/settings
# into the container (see scripts/entrypoint.sh for the full sequence).
# Lands in /opt/vibrator/ alongside welcome.sh so all vibrator-owned
# scripts share a stable, predictable location.
COPY scripts/entrypoint.sh /opt/vibrator/entrypoint.sh
RUN chmod 0755 /opt/vibrator/entrypoint.sh

# --- claude-exec wrapper (CMD on docker run + Cmd on docker exec) -----
# Re-runs the integration probes on every shell entry so a host-side
# server start/stop is picked up without rebuilding the container.
# Installed at /usr/local/bin/claude-exec for a stable absolute path
# the launch code can reference. See scripts/claude-exec.sh for what
# it does on each invocation.
COPY scripts/claude-exec.sh /usr/local/bin/claude-exec
RUN chmod 0755 /usr/local/bin/claude-exec
`)

	// --- codex config materializer (codex-gated) ---------------------------
	// Reconciles the host config.host.toml sidecar (added by the --mount
	// flip) with vibrator's baked MCP servers at container startup. Only
	// codex images need it; entrypoint.sh gates the call on both
	// VIBRATOR_HARNESS and this file's presence. See
	// scripts/codex-materialize.sh for the reconciliation logic.
	if spec.Harness.ID() == "codex" {
		b.WriteString(`
COPY scripts/codex-materialize.sh /usr/local/bin/codex-materialize
RUN chmod 0755 /usr/local/bin/codex-materialize
`)
	}

	// --- opencode config materializer (opencode-gated) ----------------------
	// Reconciles the host .config/opencode.host sidecar dir (added by the
	// mount flip) with vibrator's baked extension artifacts at container
	// startup. Only opencode images need it; entrypoint.sh gates the call
	// on both VIBRATOR_HARNESS and this file's presence. See
	// scripts/opencode-materialize.sh for the merge logic.
	if spec.Harness.ID() == "opencode" {
		b.WriteString(`
COPY scripts/opencode-materialize.sh /usr/local/bin/opencode-materialize
RUN chmod 0755 /usr/local/bin/opencode-materialize
`)
	}

	b.WriteString(`
# --- integrations manifest (per-harness wiring; data-driven probes) ---
# Generated at build time from the integrations registry filtered by
# the harness being built. claude-exec.sh reads this file on every
# shell entry to refresh MCP transport + env state.
RUN mkdir -p /etc/vibrator
COPY integrations.json /etc/vibrator/integrations.json
`)
	// Build ID written at image-build time. Check with:
	//   cat /etc/vibrator/build             (inside container)
	//   docker inspect <name> | grep BUILD  (outside container)
	fmt.Fprintf(b, "RUN echo %q > /etc/vibrator/build\n", buildID)
	b.WriteString(`

# Mirror rc files into /root/ so root sub-invocations (debugging via
# 'docker exec -u root') get the same prompt + banner, and so zsh
# build-stage commands don't trip zsh-newuser-install.
RUN cp /etc/skel/.bashrc /root/.bashrc \
 && cp /etc/skel/.zshrc /root/.zshrc \
 && mkdir -p /root/.config/fish \
 && cp /etc/skel/.config/fish/config.fish /root/.config/fish/config.fish

`)
	b.WriteString(`ENV LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8 COLORTERM=truecolor

# uv tools install into /usr/local/bin so every user finds them on PATH.
ENV UV_TOOL_BIN_DIR=/usr/local/bin

# uv-managed Python interpreters land in /opt/uv-python (world-readable)
# instead of the default $HOME/.local/share/uv/python — which would be
# /root/.local/share/uv/python during root-stage installs, unreachable
# to the unprivileged user (mode 0700 on /root). See
# https://github.com/astral-sh/uv/issues/13309 for the upstream bug;
# this ENV plus the mkdir in the python feature dodges it cleanly.
ENV UV_PYTHON_INSTALL_DIR=/opt/uv-python

# ripgrep — multi-arch binary release. Always-on.
RUN ARCH=$(dpkg --print-architecture) && \
    RG_VERSION="15.1.0" && \
    if [ "$ARCH" = "amd64" ]; then RG_TRIPLE="x86_64-unknown-linux-musl"; else RG_TRIPLE="aarch64-unknown-linux-gnu"; fi && \
    curl -fsSL "https://github.com/BurntSushi/ripgrep/releases/download/${RG_VERSION}/ripgrep-${RG_VERSION}-${RG_TRIPLE}.tar.gz" \
      -o rg.tar.gz && \
    tar -xzf rg.tar.gz --strip-components=1 -C /usr/local/bin/ "ripgrep-${RG_VERSION}-${RG_TRIPLE}/rg" && \
    rm rg.tar.gz && rg --version

# fd-find via apt (Ubuntu names the binary fdfind; we symlink to fd).
RUN apt-get update && apt-get install -y --no-install-recommends fd-find fzf \
 && ln -sf /usr/bin/fdfind /usr/local/bin/fd \
 && rm -rf /var/lib/apt/lists/*

`)
}

// writeFeaturesStage emits Stage 2: per-feature Dockerfile fragments,
// then creates the unprivileged user and switches to them. Subsequent
// stages (harness, extensions) run as that user so binaries land in
// /home/$USERNAME (readable) instead of /root (mode 0700, unreachable).
//
// Features come in dep order from feature.Resolve (deps first).
func writeFeaturesStage(b *bytes.Buffer, spec Spec) {
	b.WriteString(`# ============================================================================
# Stage 2 — features
# ============================================================================
FROM base AS features

`)
	if len(spec.Features) == 0 {
		b.WriteString("# (no features enabled — minimal profile)\n\n")
	} else {
		for _, f := range spec.Features {
			fmt.Fprintf(b, "# --- feature: %s (%s) ---\n", f.ID, f.Name)
			writeFragment(b, f.Dockerfile)
			b.WriteString("\n")
		}
	}

	// User creation lives at the END of features so that:
	//   - All apt/multi-stage-COPY work runs as root (required)
	//   - Stages 3+ (harness, extensions) run as the unprivileged user, so
	//     `claude.ai/install.sh` lands in /home/$USERNAME/.local/bin,
	//     `claude plugin install …` writes to /home/$USERNAME/.claude/,
	//     and there's no /root/-mode-0700 traversal issue when the user
	//     later uses these.
	writeUserCreation(b, spec)
}

// writeUserCreation emits the user/group setup plus the USER + WORKDIR
// switch. Extracted so the call site is obvious and tests can pin its
// position in the Dockerfile.
func writeUserCreation(b *bytes.Buffer, spec Spec) {
	username := spec.Username
	if username == "" {
		username = "vibrate"
	}

	b.WriteString("# --- user creation + USER switch -------------------------------------------\n")
	b.WriteString("# Build args supply the host's UID/GID so file permissions on bind-mounted\n")
	b.WriteString("# workspace paths match the caller (set by internal/docker at build time).\n")
	fmt.Fprintf(b, "ARG USERNAME=%s\n", username)
	fmt.Fprintf(b, "ARG HOST_UID=%d\n", spec.HostUID)
	fmt.Fprintf(b, "ARG HOST_GID=%d\n", spec.HostGID)

	b.WriteString(`
# Replace any existing user/group at the target UID/GID (Ubuntu ships an
# "ubuntu" user at 1000 which usually clashes with the host's first user).
RUN set -eux; \
    if EXISTING_USER=$(getent passwd ${HOST_UID} | cut -d: -f1); [ -n "$EXISTING_USER" ]; then userdel -r "$EXISTING_USER" 2>/dev/null || true; fi; \
    if EXISTING_GROUP=$(getent group ${HOST_GID} | cut -d: -f1); [ -n "$EXISTING_GROUP" ]; then groupdel "$EXISTING_GROUP" 2>/dev/null || true; fi; \
`)
	fmt.Fprintf(b, "    groupadd -g ${HOST_GID} ${USERNAME} && \\\n")
	fmt.Fprintf(b, "    useradd -m -s /bin/%s -u ${HOST_UID} -g ${HOST_GID} ${USERNAME} && \\\n", spec.Shell)
	b.WriteString(`    echo "${USERNAME} ALL=(root) NOPASSWD:ALL" > /etc/sudoers.d/${USERNAME} && \
    chmod 0440 /etc/sudoers.d/${USERNAME}

USER ${USERNAME}
WORKDIR /home/${USERNAME}

# ============================================================================
# INVARIANT — install-destination ENVs at the privilege boundary
# ============================================================================
# The USER switch above is a privilege boundary. ENVs set in Stages 1-2
# (as root) PERSIST into Stages 3-5 (as the unprivileged user). Any ENV
# that directs a tool to install into a system path (e.g.
# UV_TOOL_BIN_DIR=/usr/local/bin) becomes a guaranteed EACCES once an
# extension in Stage 4 invokes that tool — because the user can't write
# to system paths.
#
# Every ENV that controls "where do binaries land" MUST be overridden
# here to point at a user-writable location. Common offenders:
#   - NPM_CONFIG_PREFIX     (npm install -g)
#   - UV_TOOL_BIN_DIR       (uv tool install)
#   - GOBIN                 (go install)  — not currently set; document
#   - CARGO_HOME            (cargo install) — not currently set; document
#   - GEM_HOME / GEM_PATH   (gem install) — not currently set; document
#   - PIP_TARGET            (pip install --target) — not currently set
#
# If you add a new "install dir" ENV to Stage 1 (base) or Stage 2
# (features), add a matching override here. Failure mode: an extension
# install in Stage 4 hits "Permission denied" on a /usr/local/* path.
#
# ----------------------------------------------------------------------------
# RELATED INVARIANT — install paths whose RESOLUTION requires root traversal
# ----------------------------------------------------------------------------
# Distinct from the above: an install can land in a "world-readable" path
# whose RESOLUTION at runtime walks through /root (mode 0700, unreadable
# to the user). Symlinks LOOK fine via 'ls -la' but exec returns EACCES.
#
# Example: 'uv python install' (without UV_PYTHON_INSTALL_DIR set)
# writes the interpreter to /root/.local/share/uv/python/... in the root
# stage. The symlink at /usr/local/bin/python3 then resolves through
# /root/, killing exec for unprivileged users — see
# https://github.com/astral-sh/uv/issues/13309.
#
# Fix pattern: redirect the install dir to a world-readable path BEFORE
# the install runs. For uv-managed Python this is
# UV_PYTHON_INSTALL_DIR=/opt/uv-python (set in Stage 1). For any future
# tool that defaults to $HOME/... when run as root, set the install dir
# to /opt/<tool>/ and chmod 0755 it.

# npm — global installs to ~/.npm-global (created by npm on first use).
ENV NPM_CONFIG_PREFIX=/home/${USERNAME}/.npm-global

# uv — tool symlinks to ~/.local/bin (overriding the /usr/local/bin set
# in Stage 1 which is correct only for the root-stage uv tool installs
# in features like audit-toolkit).
ENV UV_TOOL_BIN_DIR=/home/${USERNAME}/.local/bin

# Final image-wide PATH. Order:
#   1. user-local npm globals (mcp-server-*)
#   2. user-local uv tool symlinks + claude.ai install ($HOME/.local/bin)
#   3. /usr/local/go/bin (no-op if go feature wasn't selected — a
#      non-existent PATH entry is silently ignored by exec lookups)
#   4. system defaults
# We re-emit the whole PATH rather than $PATH-prepending because
# Docker's ENV var substitution only sees prior ENVs from THIS
# Dockerfile, and the base PATH lives in the ubuntu:24.04 image's
# default — invisible to the substitution.
ENV PATH=/home/${USERNAME}/.npm-global/bin:/home/${USERNAME}/.local/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

`)
}

// writeHarnessStage emits Stage 3: harness install.
func writeHarnessStage(b *bytes.Buffer, spec Spec) {
	b.WriteString(`# ============================================================================
# Stage 3 — harness install
# ============================================================================
FROM features AS harness

`)
	fmt.Fprintf(b, "# --- harness: %s (%s) ---\n", spec.Harness.ID(), spec.Harness.Name())
	writeFragment(b, spec.Harness.Dockerfile())
	b.WriteString("\n")
}

// writeExtensionsStage emits Stage 4: extensions entry installs. Entries are
// processed in alphabetical ID order for deterministic output (the extensions
// loader doesn't impose order; we sort here).
//
// Returns an error when an install snippet collides with the heredoc
// delimiter; Generate propagates it so the build aborts BEFORE invoking
// docker rather than baking a Dockerfile with an embedded `# ERROR:`
// comment that builds a broken image.
func writeExtensionsStage(b *bytes.Buffer, spec Spec) error {
	b.WriteString(`# ============================================================================
# Stage 4 — extensions
# ============================================================================
FROM harness AS extensions

`)
	if len(spec.Extensions) == 0 {
		b.WriteString("# (no extensions selected)\n\n")
		return nil
	}
	// Stable order so the same selection set always produces the same bytes.
	entries := append([]*extensions.Entry(nil), spec.Extensions...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })

	for _, e := range entries {
		fmt.Fprintf(b, "# --- extensions/%s/%s (%s) ---\n", e.Harness, e.ID, e.Kind)
		if e.Source != "" {
			fmt.Fprintf(b, "# Source: %s\n", e.Source)
		}
		if err := writeExtensionInstall(b, e.Install); err != nil {
			return fmt.Errorf("extension %q: %w", e.ID, err)
		}
		b.WriteString("\n")
	}
	return nil
}

// extensionRunDelimiter is the heredoc tag used by writeExtensionInstall.
// Deliberately distinct from the common `EOF` so that install snippets
// containing their own `cat > file <<'EOF' ... EOF` blocks don't
// accidentally terminate the outer RUN heredoc early.
//
// BuildKit's heredoc matches the literal tag on a line by itself; any
// other line (including a bare `EOF`) is treated as content. Pick a tag
// no install script would plausibly use as a sentinel.
const extensionRunDelimiter = "VIBRATE_EXT_INSTALL"

// writeExtensionInstall wraps an extension's shell install snippet in a
// BuildKit heredoc RUN block. The extensions `install:` field is plain
// shell (no Dockerfile RUN prefix) so authors don't have to think about
// line continuations or `&&` chaining. We add `set -e` so a failed
// command aborts the build (matches Docker's exit-on-error semantics
// for single-command RUNs).
//
// Single-quoted `<<'VIBRATE_EXT_INSTALL'` prevents the shell from
// expanding variables at heredoc-feed time — `$HOME`, `$USER`, etc.
// inside the script are resolved by the container's shell when the
// script runs, not by the host's docker build.
//
// Defensive guard: if an install script happens to contain a standalone
// line equal to our delimiter, we reject the build rather than emit
// silently-broken output. Authors hit a clear error message.
func writeExtensionInstall(b *bytes.Buffer, install string) error {
	body := strings.TrimRight(install, "\n")
	if strings.TrimSpace(body) == "" {
		return nil
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == extensionRunDelimiter {
			return fmt.Errorf("extension install contains reserved heredoc delimiter %q on a standalone line — rename or quote", extensionRunDelimiter)
		}
	}
	b.WriteString("RUN <<'" + extensionRunDelimiter + "'\n")
	b.WriteString("set -e\n")
	b.WriteString(body)
	b.WriteString("\n" + extensionRunDelimiter + "\n")
	return nil
}

// writeRuntimeStage emits Stage 5: labels, env, CMD. User creation
// happens earlier (end of Stage 2) so harness + extensions can install
// as that user. This stage just stamps metadata + sets the default
// command.
func writeRuntimeStage(b *bytes.Buffer, spec Spec) {
	b.WriteString(`# ============================================================================
# Stage 5 — runtime: labels, default command
# (User creation + USER switch already happened at end of Stage 2.)
# ============================================================================
FROM extensions AS runtime

`)

	// Auth env-var ARGs — declared so they're listed in the image manifest
	// (`docker inspect`). Actual values are injected by `docker run -e ...`
	// at runtime, not baked into the image.
	if envs := spec.Harness.AuthEnvVars(); len(envs) > 0 {
		b.WriteString("# Auth env vars expected by this harness (forwarded by `docker run -e`):\n")
		for _, e := range envs {
			fmt.Fprintf(b, "ENV %s=\"\"\n", e)
		}
		b.WriteString("\n")
	}

	// Variant metadata as ENV so in-container scripts (welcome banner,
	// future entrypoint) can read them. Same values are emitted as
	// LABELs below for image-management commands (`vibrate variants list`).
	b.WriteString("# Variant metadata — readable from inside the container.\n")
	fmt.Fprintf(b, "ENV VIBRATOR_HARNESS=%q\n", spec.Harness.ID())
	fmt.Fprintf(b, "ENV VIBRATOR_PROFILE=%q\n", spec.Profile)
	fmt.Fprintf(b, "ENV VIBRATOR_FEATURES_LIST=%q\n", featureIDsCSV(spec.Features))
	fmt.Fprintf(b, "ENV VIBRATOR_EXTENSIONS_LIST=%q\n", extensionsIDsCSV(spec.Extensions))
	fmt.Fprintf(b, "ENV VIBRATOR_VERSION=%q\n", versionOrDev(spec.VibratorVersion))
	fmt.Fprintf(b, "ENV VIBRATOR_BUILD_ID=%q\n", buildID)

	// Codex: snapshot the MCP servers the extensions just baked into
	// ~/.codex/config.toml, as replayable JSON. The runtime materializer
	// (codex-materialize.sh) re-adds these on top of the user's host config
	// after seeding it, since the host mount would otherwise shadow them.
	// `|| echo '[]'` keeps a codex image with zero MCP extensions valid.
	if spec.Harness.ID() == "codex" {
		b.WriteString(`RUN codex mcp list --json > "$HOME/.vibrator-codex-baked-mcp.json" 2>/dev/null || echo '[]' > "$HOME/.vibrator-codex-baked-mcp.json"` + "\n")
	}

	// OpenCode: snapshot the whole baked ~/.config/opencode directory
	// (config.json MCPs plus agent/, themes/, tui.json baked by
	// extensions). The runtime materializer (opencode-materialize.sh)
	// restores it as the deterministic merge base after the host sidecar
	// seeds the container config — snapshotting only config.json would
	// lose the other baked artifacts and make restarts non-idempotent.
	// `|| mkdir -p` keeps a zero-extension opencode image valid (empty
	// snapshot, nothing to restore).
	if spec.Harness.ID() == "opencode" {
		b.WriteString(`RUN cp -a "$HOME/.config/opencode" "$HOME/.vibrator-opencode-baked" 2>/dev/null || mkdir -p "$HOME/.vibrator-opencode-baked"` + "\n")
	}

	// Pi: snapshot the whole baked ~/.pi tree (agent/mcp.json MCPs, the
	// agent/settings.json extension registry, providers/, themes/,
	// prompts/ baked by extensions). The runtime materializer
	// (pi-materialize.sh) copies it back as the deterministic merge base
	// after the host sidecar seeds the container tree. `|| mkdir -p`
	// keeps a zero-extension pi image valid (empty snapshot, nothing to
	// restore).
	if spec.Harness.ID() == "pi" {
		b.WriteString(`RUN cp -a "$HOME/.pi" "$HOME/.vibrator-pi-baked" 2>/dev/null || mkdir -p "$HOME/.vibrator-pi-baked"` + "\n")
	}

	// VIBRATOR_LAUNCH_BIN / VIBRATOR_YOLO_ARGS drive the env-driven shell
	// alias in templates/shells/{zshrc,bashrc,config.fish} — the alias reads
	// these two vars instead of a hardcoded "claude --dangerously-skip-
	// permissions", so it can never drift from the harness's own
	// PermissionBypassArgs (the same source resolveLaunchCmd uses for the
	// direct-launch path). These are the build-time DEFAULTS; the launch
	// code overrides VIBRATOR_YOLO_ARGS at `docker run`/`docker exec` time
	// (via yoloEnvVar in internal/app/launch.go) so --no-yolo blanks the
	// alias without requiring a rebuild.
	launchBin := ""
	if lc := spec.Harness.LaunchCommand(); len(lc) > 0 {
		launchBin = lc[0]
	}
	fmt.Fprintf(b, "ENV VIBRATOR_LAUNCH_BIN=%q\n", launchBin)
	fmt.Fprintf(b, "ENV VIBRATOR_YOLO_ARGS=%q\n", strings.Join(spec.Harness.PermissionBypassArgs(), " "))

	// Labels — used by `vibrate variants list` and image upgrade workflows.
	b.WriteString("\n# Labels — used by `vibrate variants list` and upgrade workflows.\n")
	fmt.Fprintf(b, "LABEL vibrator.version=%q\n", versionOrDev(spec.VibratorVersion))
	fmt.Fprintf(b, "LABEL vibrator.build_id=%q\n", buildID)
	fmt.Fprintf(b, "LABEL vibrator.harness=%q\n", spec.Harness.ID())
	fmt.Fprintf(b, "LABEL vibrator.profile=%q\n", spec.Profile)
	fmt.Fprintf(b, "LABEL vibrator.shell=%q\n", spec.Shell)
	fmt.Fprintf(b, "LABEL vibrator.features=%q\n", featureIDsCSV(spec.Features))
	fmt.Fprintf(b, "LABEL vibrator.extensions=%q\n", extensionsIDsCSV(spec.Extensions))

	b.WriteString("\n")
	// ENTRYPOINT runs the host-config merge + rules + settings setup
	// (see scripts/entrypoint.sh). It exec's $@ at the end, so CMD
	// below is what actually becomes PID 1 after entrypoint completes.
	// Without ENTRYPOINT, an empty home means claude inside the
	// container would re-prompt for onboarding on first launch.
	b.WriteString("\nENTRYPOINT [\"/opt/vibrator/entrypoint.sh\"]\n")
	// CMD wraps the user's shell with claude-exec so the Serena probe
	// fires on first start (entrypoint → CMD chain). Subsequent `docker
	// exec` calls also go through claude-exec (set in launch.go), so the
	// probe runs on every shell entry — not just the first.
	fmt.Fprintf(b, "CMD [\"/usr/local/bin/claude-exec\", \"/bin/%s\"]\n", spec.Shell)
}

// writeFragment appends a (possibly multi-line) fragment to the buffer with
// a single trailing newline. Trims excess trailing newlines so successive
// fragments don't accumulate blank lines.
func writeFragment(b *bytes.Buffer, frag string) {
	b.WriteString(strings.TrimRight(frag, "\n"))
	b.WriteByte('\n')
}

// featureIDsCSV formats feature IDs for header comments and labels.
// Returns "(none)" when the slice is empty so the header reads sensibly.
func featureIDsCSV(features []feature.Feature) string {
	if len(features) == 0 {
		return "(none)"
	}
	ids := make([]string, len(features))
	for i, f := range features {
		ids[i] = f.ID
	}
	return strings.Join(ids, ",")
}

// extensionsIDsCSV formats extensions entry IDs for header comments and labels.
func extensionsIDsCSV(entries []*extensions.Entry) string {
	if len(entries) == 0 {
		return "(none)"
	}
	// Sort for stability — same selection should always produce the same
	// CSV regardless of input order.
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func versionOrDev(v string) string {
	if v == "" {
		return "dev"
	}
	return v
}
