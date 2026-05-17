# Configuration defaults and environment variable loading.
# Sets all global state used throughout the script.

config::init() {
    # User identity
    CFG_USERNAME="$(whoami)"

    # Paths
    # Get absolute canonical path (resolves symlinks, normalizes ..)
    WORKSPACE="$(realpath "$(pwd)")"
    CLAUDE_CONFIG="$HOME/.claude"

    # Image naming
    IMAGE_NAME="claude-vb-${CFG_USERNAME}:latest"

    # Runtime/security flags (orthogonal to the feature catalog below)
    INTERACTIVE=true
    REMOVE_AFTER=false
    PRIVILEGED=false
    DANGEROUS=true
    FORWARD_AGENTS=false
    VERBOSE=false
    INSTALL_PLUGINS=true
    DOCKER_IN_DOCKER=false
    AIDER=false              # legacy alias, mirrors FEATURES[aider] for back-compat
    GENERIC=false
    AGENT_TEAMS=false
    SIMPLE_BUILD=false       # legacy alias, set by --simple/--no-tools and --profile minimal

    # Build-time feature catalog. Each unit can be toggled with
    # --with-<name> / --no-<name>, or set in bulk via --profile <preset>.
    # Order here is the canonical order used in help text and features.json.
    #
    # NOTE: macOS ships bash 3.2 (no associative arrays), so we represent
    # feature state as a space-padded string FEATURES_ENABLED and use the
    # config::feature_{enable,disable,enabled} helpers below.
    FEATURE_CATALOG=(
        playwright       # Chromium + Playwright MCP (~500MB)
        audit-toolkit    # trivy/syft/grype/semgrep/gitleaks/etc. (~400MB)
        python           # Python 3.13 via uv (~100MB)
        go               # Go toolchain (~200MB)
        gh               # GitHub CLI
        dev-cli          # jq, yq, fzf, fd, ripgrep, tree, httpie, websocat, csvkit, delta, lazygit, glow
        serena           # Serena MCP runtime wiring
        claude-mem       # claude-mem plugin auto-install + server-beta env forwarding
        codex            # OpenAI Codex CLI (used by /planning:exec)
        aider            # aider AI pair programming (off by default)
    )

    # Space-padded list of currently-enabled feature names. Mutated by
    # config::apply_profile and the --with-*/--no-* parsers in args::parse.
    FEATURES_ENABLED=""

    # Currently active profile name (mirrored into the image manifest).
    PROFILE="default"
    config::apply_profile "default"

    # Variant fingerprint and pin-file source are populated later, after CLI
    # parsing and dep validation, by config::finalize_image_name and
    # config::load_pin_file respectively. Initialized empty so the welcome
    # message can safely check them.
    VARIANT_FINGERPRINT=""
    PIN_SOURCE=""

    # Build/action flags
    FLAG_BUILD_ONLY=false
    FLAG_REBUILD=false
    FLAG_RECREATE=false
    FLAG_PULL=""
    FLAG_EXPLAIN_FEATURES=false
    FLAG_NO_MENU=false
    FLAG_UPGRADE_CLAUDE=false
    FLAG_CLAUDE_MEM_SETUP=false
    FLAG_CLAUDE_MEM_STATUS=false
    FLAG_CLAUDE_MEM_BOOTSTRAP=false
    EXPORT_DOCKERFILE=""

    # True if the user passed any feature-affecting CLI flag (--profile,
    # --with-*, --no-*, --simple, --no-tools, --aider). Drives whether the
    # interactive menu fires on first run in a workspace.
    USER_SPECIFIED_FEATURES=false

    # Resource limits
    MEMORY_LIMIT=""
    CPU_LIMIT=""

    # OS detection (canonical method)
    IS_MACOS=false
    if [[ "$(uname -s)" == "Darwin" ]]; then
        IS_MACOS=true
    fi

    # Selective mount: true on macOS (path incompatibility), false on Linux
    SELECTIVE_MOUNT="$IS_MACOS"
    MOUNT_FULL_CONFIG=false

    # Environment forwarding list.
    # Note: OPENAI_API_KEY is used by both OpenAI SDKs and the Codex CLI
    # (baked into the image for the /planning:exec skill). Codex prefers
    # ~/.codex/auth.json (mounted in docker_cmd.sh) but falls back to
    # this env var when no OAuth login is present.
    FORWARDED_ENV=(
        ANTHROPIC_API_KEY
        OPENAI_API_KEY
        ANTHROPIC_MODEL
        CLAUDE_CODE_OAUTH_TOKEN
        TERM
    )

    # Extra volumes (populated by --aws etc.)
    EXTRA_VOLUMES=()

    # Docker passthrough args (after --)
    PASSTHROUGH_ARGS=()

    # Command to run inside container (remaining positional args)
    REMAINING_ARGS=()
}

## --- Feature catalog helpers ----------------------------------------------
##
## Always-on substrate (not in the catalog, not toggleable): bash, sh, jq,
## curl, wget, git, sudo, zsh, vim, tree, gpg, sqlite3, sed/awk/find, plus
## bun + node + uv. These are tiny and load-bearing for entrypoint and many
## features; toggling them off would break the container shell.
##
## State representation: FEATURES_ENABLED is a space-padded string holding
## the names of currently-enabled features (e.g. " python go dev-cli ").
## We deliberately avoid associative arrays so the script runs on macOS's
## bash 3.2 (no `declare -A`, no `declare -g`).

# Cross-feature dependencies. Returns the deps for the given feature name,
# one per line. Empty output means "no implicit requirements".
config::feature_deps() {
    case "$1" in
        audit-toolkit) echo "python" ;;   # bandit, checkov, pip-audit
        serena)        echo "python" ;;   # uvx → python via uv
        aider)         echo "python" ;;   # uv tool install aider-chat
        *)             echo "" ;;
    esac
}

# Is the named feature currently enabled?
config::feature_enabled() {
    case " $FEATURES_ENABLED " in
        *" $1 "*) return 0 ;;
        *)        return 1 ;;
    esac
}

# Mark feature as enabled (idempotent).
config::feature_enable() {
    config::feature_enabled "$1" && return 0
    FEATURES_ENABLED="${FEATURES_ENABLED} $1"
    FEATURES_ENABLED="${FEATURES_ENABLED# }"
}

# Mark feature as disabled (idempotent).
config::feature_disable() {
    local padded=" $FEATURES_ENABLED "
    padded="${padded// $1 / }"
    padded="${padded# }"
    padded="${padded% }"
    FEATURES_ENABLED="$padded"
}

# True if the given name is in the feature catalog. Used by the CLI parser
# to validate --with-X / --no-X before applying.
config::is_known_feature() {
    local needle="$1" f
    for f in "${FEATURE_CATALOG[@]}"; do
        [[ "$f" == "$needle" ]] && return 0
    done
    return 1
}

# Profile presets. Each preset is the canonical starting point; users may
# layer --with-*/--no-* on top to fine-tune. The chosen profile name is
# recorded in /opt/vibrator/features.json for the welcome message.
config::apply_profile() {
    local profile="$1" f
    PROFILE="$profile"
    FEATURES_ENABLED=""

    case "$profile" in
        minimal)
            # Tiny image — just shell + dev-cli. ~150MB.
            config::feature_enable dev-cli
            ;;
        backend)
            # No Playwright, no audit toolkit, no aider. ~600MB.
            config::feature_enable python
            config::feature_enable go
            config::feature_enable gh
            config::feature_enable dev-cli
            config::feature_enable serena
            config::feature_enable claude-mem
            config::feature_enable codex
            ;;
        default)
            # What `vibrate` runs today — everything except aider. ~2GB.
            for f in "${FEATURE_CATALOG[@]}"; do
                config::feature_enable "$f"
            done
            config::feature_disable aider
            ;;
        kitchen-sink)
            # Everything, including aider.
            for f in "${FEATURE_CATALOG[@]}"; do
                config::feature_enable "$f"
            done
            ;;
        *)
            log::die "Unknown profile: '$profile' (valid: minimal, backend, default, kitchen-sink)"
            ;;
    esac
}

# Enforce cross-feature dependencies by auto-enabling missing deps. Logs a
# warning for each auto-enabled dep so the user knows why their --no-X was
# overridden. Called once at the end of args parsing.
config::validate_features() {
    local feat dep deps changed
    # Iterate in catalog order for stable warning output.
    for feat in "${FEATURE_CATALOG[@]}"; do
        config::feature_enabled "$feat" || continue
        deps=$(config::feature_deps "$feat")
        for dep in $deps; do
            if ! config::feature_enabled "$dep"; then
                config::feature_enable "$dep"
                changed=1
                log::warn "Feature '$feat' requires '$dep' — auto-enabling. To avoid, also pass --no-$feat."
            fi
        done
    done
    [[ -z "${changed:-}" ]] || log::verbose "Feature dependencies resolved."
}

# Print the resolved feature set. Used by --explain-features.
config::print_features() {
    printf 'Profile: %s\n' "$PROFILE"
    printf 'Features:\n'
    local f state
    for f in "${FEATURE_CATALOG[@]}"; do
        if config::feature_enabled "$f"; then state=true; else state=false; fi
        printf '  %-15s %s\n' "$f" "$state"
    done
}

## --- End feature catalog helpers ------------------------------------------


## --- Image / container variant helpers ------------------------------------
##
## Two distinct feature sets produce different fingerprints, which feed both
## the image tag and the container name. That way:
##   - workspaces with different profiles get distinct images (parallel builds
##     don't race on a shared tag)
##   - the same workspace built with two different profiles gets two distinct
##     containers (no surprising image mismatch on docker exec re-entry)

# Stable 8-char hex hash of the currently-enabled feature set. Two identical
# feature sets always produce the same fingerprint, regardless of toggle order.
config::variant_fingerprint() {
    local features sorted
    features="${FEATURES_ENABLED# }"
    features="${features% }"
    if [[ -z "$features" ]]; then
        echo "00000000"
        return
    fi
    # Word-split intentionally to sort feature names line-by-line.
    sorted=$(printf '%s\n' $features | sort | tr '\n' ' ')
    printf '%s' "$sorted" | sha256sum | cut -c1-8
}

# Set IMAGE_NAME and VARIANT_FINGERPRINT based on the resolved feature set.
# Skips IMAGE_NAME rewriting when the user passed --image, when we're in
# generic mode, or when the interactive menu locked an existing variant
# (different naming convention or pre-resolved value).
config::finalize_image_name() {
    # Preserve a pre-set fingerprint (e.g., the menu picked an existing
    # variant whose label is authoritative even if the catalog has moved on).
    [[ -z "$VARIANT_FINGERPRINT" ]] && VARIANT_FINGERPRINT=$(config::variant_fingerprint)

    local default_pattern="claude-vb-${CFG_USERNAME}"
    if [[ "$IMAGE_NAME" == "${default_pattern}:latest" ]]; then
        IMAGE_NAME="${default_pattern}-${PROFILE}-${VARIANT_FINGERPRINT}:latest"
    fi
}

# Walk up from $PWD looking for .vb.env, stopping at the git root (if cwd
# is inside a repo) or the filesystem root. Prints the path on stdout, or
# nothing if no file found. ALWAYS returns 0 — under `set -e` (bash 5.x+),
# `var=$(failing_cmd)` propagates the non-zero exit and aborts the caller,
# so we signal "not found" by empty output instead of a failure code.
config::_find_pin_file() {
    local dir="$PWD"
    local git_root=""
    if command -v git >/dev/null 2>&1; then
        git_root=$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null || true)
    fi
    while [[ -n "$dir" ]]; do
        if [[ -f "$dir/.vb.env" ]]; then
            echo "$dir/.vb.env"
            return 0
        fi
        # Stop at git root, or at filesystem root
        [[ -n "$git_root" && "$dir" == "$git_root" ]] && return 0
        [[ "$dir" == "/" ]] && return 0
        dir=$(dirname "$dir")
    done
    return 0
}

# Read .vb.env from $PWD or any ancestor up to the git root and apply its
# PROFILE/WITH/NO settings. Parsed manually (not `source`d) so a malicious
# project file can't execute arbitrary code in the host shell.
config::load_pin_file() {
    local pin_file
    pin_file=$(config::_find_pin_file)
    [[ -n "$pin_file" && -f "$pin_file" ]] || return 0

    log::verbose "Loading pin file: $pin_file"

    local line key value f
    local pin_profile="" pin_with="" pin_no=""

    while IFS= read -r line || [[ -n "$line" ]]; do
        [[ "$line" =~ ^[[:space:]]*# ]] && continue
        [[ "$line" =~ ^[[:space:]]*$ ]] && continue
        if [[ "$line" =~ ^[[:space:]]*([A-Z_]+)=(.*)$ ]]; then
            key="${BASH_REMATCH[1]}"
            value="${BASH_REMATCH[2]}"
            # Strip surrounding quotes
            value="${value#\"}"; value="${value%\"}"
            value="${value#\'}"; value="${value%\'}"
            case "$key" in
                PROFILE) pin_profile="$value" ;;
                WITH)    pin_with="$value" ;;
                NO)      pin_no="$value" ;;
                # Per-workspace claude-mem cache (written by
                # claude_mem_bootstrap::run). Whitelisted explicitly so
                # docker_cmd.sh can pick them up from the host env, while
                # arbitrary keys still raise the "unknown key" log line.
                # No eval / source — value is a plain string assignment.
                CLAUDE_MEM_SERVER_BETA_API_KEY|\
                CLAUDE_MEM_SERVER_BETA_TEAM_ID|\
                CLAUDE_MEM_SERVER_BETA_PROJECT_ID)
                    export "$key=$value" ;;
                *)       log::verbose "Pin file: ignoring unknown key '$key'" ;;
            esac
        fi
    done < "$pin_file"

    [[ -n "$pin_profile" ]] && config::apply_profile "$pin_profile"

    for f in $pin_with; do
        config::is_known_feature "$f" \
            || log::die "Pin file $pin_file: unknown feature '$f' in WITH (valid: ${FEATURE_CATALOG[*]})"
        config::feature_enable "$f"
    done
    for f in $pin_no; do
        config::is_known_feature "$f" \
            || log::die "Pin file $pin_file: unknown feature '$f' in NO (valid: ${FEATURE_CATALOG[*]})"
        config::feature_disable "$f"
    done

    PIN_SOURCE="$pin_file"
}

## --- End image / container variant helpers --------------------------------


## --- claude-mem onboarding helpers ----------------------------------------

# True if the user has the claude-mem feature on AND the host-side config
# is plausible. Three valid states:
#   a) workspace .vb.env has a cached project-scoped key (post-bootstrap)
#   b) admin dotenv has CLAUDE_MEM_SERVER_DATABASE_URL (auto-bootstrap will fire)
#   c) admin dotenv has CLAUDE_MEM_SERVER_BETA_API_KEY (legacy explicit key)
# Used to decide whether to show the setup instructions on launch.
config::claude_mem_configured() {
    config::feature_enabled "claude-mem" || return 1

    # (a) Per-workspace cache from a previous bootstrap.
    if [[ -f "$WORKSPACE/.vb.env" ]] && \
        grep -q '^[[:space:]]*CLAUDE_MEM_SERVER_BETA_API_KEY=' "$WORKSPACE/.vb.env" 2>/dev/null; then
        return 0
    fi

    local cfg="${VIBRATOR_CLAUDE_MEM_ENV:-$HOME/.config/vibrator/claude-mem.env}"
    [[ -f "$cfg" ]] || return 1

    # (c) Legacy: pre-minted key in admin dotenv.
    grep -q '^CLAUDE_MEM_SERVER_BETA_API_KEY=' "$cfg" 2>/dev/null && return 0
    # (b) Bootstrap-enabled: just the DB URL is enough — the per-workspace
    # key will be minted on first vibrate in this dir.
    grep -q '^CLAUDE_MEM_SERVER_DATABASE_URL=' "$cfg" 2>/dev/null && return 0

    return 1
}

# Print the full claude-mem setup story. Used by:
#   - the interactive menu after the user enables claude-mem
#   - main.sh, just before launch, when the feature is on but unconfigured
#   - the standalone `vibrate --claude-mem-setup` flag
config::print_claude_mem_setup() {
    local cfg="${VIBRATOR_CLAUDE_MEM_ENV:-$HOME/.config/vibrator/claude-mem.env}"
    cat <<EOF

═══════════════════════════════════════════════════════════════════════
  claude-mem — persistent memory across sessions
═══════════════════════════════════════════════════════════════════════

Vibrator integrates with claude-mem's server-beta runtime. You bring up
the host-side compose stack once; vibrator auto-mints a project-scoped
API key per workspace on first \`vibrate\` and caches it locally.

NO API key minting, NO team management, NO project creation by hand.
Just give vibrator the Postgres DSN and let it do the rest.

One-time host setup (~3 minutes):

  # 1. Clone the upstream repo and bring the compose stack up.
  git clone https://github.com/thedotmack/claude-mem.git ~/dev/claude-mem-stack
  cd ~/dev/claude-mem-stack
  cat > .env <<ENV
  POSTGRES_USER=claudemem
  POSTGRES_PASSWORD=\$(openssl rand -hex 24)
  POSTGRES_DB=claudemem
  ANTHROPIC_API_KEY=\$ANTHROPIC_API_KEY
  ENV
  chmod 600 .env
  docker compose up -d --build
  curl -fsS http://127.0.0.1:37877/healthz   # → 200 OK

  # 2. Drop the DSN where vibrator picks it up. THREE keys, that's it.
  mkdir -p "\$(dirname "$cfg")"
  cat > $cfg <<KEYS
  CLAUDE_MEM_RUNTIME=server-beta
  CLAUDE_MEM_SERVER_BETA_URL=http://host.docker.internal:37877
  CLAUDE_MEM_SERVER_DATABASE_URL=postgres://claudemem:<PG_PASS>@host.docker.internal:5432/claudemem
  KEYS
  chmod 600 $cfg

  # 3. (Optional) Verify the wiring without launching a container:
  vibrate --claude-mem-status

Per-workspace bootstrap (automatic, you do not run this):

  On first \`vibrate\` in a workspace, vibrator runs a one-shot
  postgres:16-alpine container against your DSN to:

    - upsert team "vibrators"
    - upsert project = basename(\$PWD)
    - mint a project-scoped API key with scopes=["*"]
    - revoke any prior live key for (team, project, actor)
    - persist the key + ids to <workspace>/.vb.env (chmod 600)

  The DSN never enters the vibrator container. Only the project-scoped
  Bearer token does. Subsequent vibrates in the same workspace read the
  cached key from .vb.env — bootstrap is skipped.

  Force a fresh key (e.g., after a leak): delete the
  "CLAUDE_MEM_SERVER_BETA_*" lines from \$WORKSPACE/.vb.env and re-run
  vibrate. Or: \`vibrate --claude-mem-bootstrap\` (no container).

Security model:

  - Workspace key is locked to ONE project (claude-mem ensureProjectAllowed
    enforces this). A leaked key compromises one project's events, nothing
    else. Revocation = single UPDATE.
  - DSN with full DB access stays in $cfg on the host. Never forwarded.
  - <workspace>/.vb.env contains the plaintext token; chmod 600, gitignore.

Already have your own Postgres? See the "Advanced: external Postgres"
section of docs/integrations/claude-mem.md for the compose override.

═══════════════════════════════════════════════════════════════════════

EOF
}

## --- End claude-mem onboarding helpers ------------------------------------

config::load_oauth_token() {
    local token_file="$HOME/.claude-docker-token"
    if [[ -z "${CLAUDE_CODE_OAUTH_TOKEN:-}" && -f "$token_file" ]]; then
        CLAUDE_CODE_OAUTH_TOKEN=$(tr -d '[:space:]' < "$token_file")
        [[ -n "$CLAUDE_CODE_OAUTH_TOKEN" ]] && export CLAUDE_CODE_OAUTH_TOKEN
    fi
}

config::derive_container_name() {
    local two_parts
    two_parts=$(echo "$WORKSPACE" | awk -F'/' '{if(NF>=2) print $(NF-1)"/"$NF; else print $NF}')
    local sanitized
    sanitized="${two_parts//[^a-zA-Z0-9_-]/-}"
    local hash
    hash=$(echo "$WORKSPACE" | sha256sum | cut -c1-8)
    # Append the variant fingerprint so different feature sets for the same
    # workspace map to distinct containers. Falls back to no suffix in legacy
    # call sites that run before config::finalize_image_name.
    local suffix=""
    [[ -n "${VARIANT_FINGERPRINT:-}" ]] && suffix="-${VARIANT_FINGERPRINT}"
    CONTAINER_NAME="claude-vb-${sanitized}-${hash}${suffix}"
}

config::apply_env_overrides() {
    [[ "${VIBRATOR_VERBOSE:-}" == "1" ]] && VERBOSE=true
    [[ -n "${VIBRATOR_IMAGE:-}" ]] && IMAGE_NAME="${VIBRATOR_IMAGE}:latest"

    # Override selective mount if user explicitly requested full mount
    [[ "$MOUNT_FULL_CONFIG" == true ]] && SELECTIVE_MOUNT=false

    # Extra env vars from VIBRATOR_EXTRA_ENV
    if [[ -n "${VIBRATOR_EXTRA_ENV:-}" ]]; then
        local -a extra
        IFS=' ' read -ra extra <<< "$VIBRATOR_EXTRA_ENV"
        for var in "${extra[@]}"; do
            if [[ "$var" == !* ]]; then
                config::remove_forwarded_env "${var#!}"
            else
                config::add_forwarded_env "$var"
            fi
        done
    fi
}

config::add_forwarded_env() {
    FORWARDED_ENV+=("$1")
}

config::remove_forwarded_env() {
    local remove="$1"
    local -a new=()
    for v in ${FORWARDED_ENV[@]+"${FORWARDED_ENV[@]}"}; do
        [[ "$v" != "$remove" ]] && new+=("$v")
    done
    FORWARDED_ENV=(${new[@]+"${new[@]}"})
}
