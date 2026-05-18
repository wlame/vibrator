# Host-side claude-mem status / verification helpers.
#
# Runs from the host shell (bash 3.2+ compat — macOS), NOT inside the
# container. Reads the per-workspace cache file ($WORKSPACE/.vb.env)
# first, then falls back to the admin dotenv (~/.config/vibrator/claude-mem.env)
# for users on the legacy explicit-key flow. Optionally talks to Postgres
# via a one-shot postgres:16-alpine container (the same pattern the bootstrap
# uses), so the host doesn't need psql installed.

# Probe URL rewriter: inside the container we point at host.docker.internal,
# but from the host shell we have to swap that back to a real reachable host
# for the curl-from-host /healthz check.
claude_mem::_host_probe_url() {
    local url="$1"
    printf '%s' "${url//host.docker.internal/127.0.0.1}"
}

# Read a single key from a dotenv-style file in a subshell, so the values
# don't leak into the host environment. Empty output = key unset.
claude_mem::_read_dotenv() {
    local cfg_file="$1" key="$2"
    [[ -f "$cfg_file" ]] || return 0
    (
        # shellcheck disable=SC1090
        . "$cfg_file" >/dev/null 2>&1
        printf '%s' "${!key:-}"
    )
}

# SHA-256 hex of a string, no trailing newline (matches the server's
# hash-on-write path — see claude_mem_bootstrap::_sha256_hex).
claude_mem::_sha256_hex() {
    printf '%s' "$1" | sha256sum | awk '{print $1}'
}

# Run a one-shot SELECT against the DB URL using postgres:16-alpine, the
# same pattern as the bootstrap. Returns the result on stdout (or empty
# on error). DB URL gets the localhost → host.docker.internal rewrite so
# the container can reach the host's Postgres.
#
# SQL is fed via stdin (`-f -`) and parameter values are passed via
# `-v key=value` so the query uses `:'name'` interpolation. This matches
# the bootstrap pattern and avoids string-concatenating user-controlled
# values from .vb.env into the SQL (the explicit values get psql's quoted
# string substitution — no escaping bugs, no injection surface).
#
# Extra args after the SQL are forwarded to psql (use them for `-v` bindings).
claude_mem::_pg_query() {
    local db_url="$1" sql="$2"
    [[ -n "$db_url" ]] || return 0
    shift 2
    local rewritten
    rewritten=$(printf '%s' "$db_url" | sed -E 's#//([^/@]*@)?(localhost|127\.0\.0\.1)([:/])#//\1host.docker.internal\3#')
    printf '%s\n' "$sql" | docker run --rm -i \
        --add-host=host.docker.internal:host-gateway \
        postgres:16-alpine \
        psql "$rewritten" -tA -v ON_ERROR_STOP=1 "$@" -f - 2>/dev/null
}

# Pretty-print whether a probe succeeded. Returns 0 on success, 1 on failure.
claude_mem::_probe_health() {
    local probe_url="$1"
    if curl -sf --connect-timeout 1 --max-time 2 "$probe_url/healthz" >/dev/null 2>&1; then
        printf '  Health probe:   \033[32mOK\033[0m  (%s/healthz)\n' "$probe_url"
        return 0
    fi
    printf '  Health probe:   \033[31mFAIL\033[0m  (%s/healthz unreachable)\n' "$probe_url"
    return 1
}

# Resolve the project-scoped credentials for the current workspace, in
# priority order:
#   1. workspace .vb.env       (post-bootstrap cache, the new normal)
#   2. admin dotenv API_KEY    (legacy explicit-key flow)
# Sets globals consumed by the rest of the status report:
#   CLAUDE_MEM_RESOLVED_API_KEY, _TEAM_ID, _PROJECT_ID, _SOURCE
CLAUDE_MEM_RESOLVED_API_KEY=""
CLAUDE_MEM_RESOLVED_TEAM_ID=""
CLAUDE_MEM_RESOLVED_PROJECT_ID=""
CLAUDE_MEM_RESOLVED_SOURCE=""
claude_mem::_resolve_credentials() {
    local cfg_file="$1"
    local pin="$WORKSPACE/.vb.env"

    # Workspace cache wins.
    if [[ -f "$pin" ]] && grep -q '^[[:space:]]*CLAUDE_MEM_SERVER_BETA_API_KEY=' "$pin"; then
        CLAUDE_MEM_RESOLVED_API_KEY=$(claude_mem::_read_dotenv "$pin" CLAUDE_MEM_SERVER_BETA_API_KEY)
        CLAUDE_MEM_RESOLVED_TEAM_ID=$(claude_mem::_read_dotenv "$pin" CLAUDE_MEM_SERVER_BETA_TEAM_ID)
        CLAUDE_MEM_RESOLVED_PROJECT_ID=$(claude_mem::_read_dotenv "$pin" CLAUDE_MEM_SERVER_BETA_PROJECT_ID)
        CLAUDE_MEM_RESOLVED_SOURCE="workspace .vb.env"
        return 0
    fi

    # Legacy: admin dotenv carries the key directly.
    local admin_key
    admin_key=$(claude_mem::_read_dotenv "$cfg_file" CLAUDE_MEM_SERVER_BETA_API_KEY)
    if [[ -n "$admin_key" ]]; then
        CLAUDE_MEM_RESOLVED_API_KEY="$admin_key"
        CLAUDE_MEM_RESOLVED_TEAM_ID=$(claude_mem::_read_dotenv "$cfg_file" CLAUDE_MEM_SERVER_BETA_TEAM_ID)
        CLAUDE_MEM_RESOLVED_PROJECT_ID=$(claude_mem::_read_dotenv "$cfg_file" CLAUDE_MEM_SERVER_BETA_PROJECT_ID)
        CLAUDE_MEM_RESOLVED_SOURCE="admin dotenv (legacy)"
        return 0
    fi

    return 1
}

# Print server-side verification (optional — only if DATABASE_URL is set
# in the admin dotenv AND docker is reachable). Confirms the cached key
# matches a live, unrevoked api_keys row and that the team/project exist.
claude_mem::_verify_server_side() {
    local db_url="$1"
    [[ -n "$db_url" ]] || {
        printf '  Server verify:  (skipped — no CLAUDE_MEM_SERVER_DATABASE_URL set)\n'
        return 0
    }

    local hash row
    hash=$(claude_mem::_sha256_hex "$CLAUDE_MEM_RESOLVED_API_KEY")
    row=$(claude_mem::_pg_query "$db_url" "
        SELECT
            COALESCE(t.name, '?')       AS team,
            COALESCE(p.name, '?')       AS project,
            CASE WHEN k.revoked_at IS NULL THEN 'live' ELSE 'revoked' END AS state,
            COALESCE(k.actor_id, '?')   AS actor
        FROM api_keys k
        LEFT JOIN teams    t ON t.id = k.team_id
        LEFT JOIN projects p ON p.id = k.project_id
        WHERE k.key_hash = :'hash'
        LIMIT 1;" \
        -v hash="$hash")

    if [[ -z "$row" ]]; then
        printf '  Server verify:  \033[31mFAIL\033[0m  (key hash not found in api_keys)\n'
        printf '                    Either the key was deleted or the DB URL points elsewhere.\n'
        return 1
    fi

    local team project state actor
    team=$(printf '%s' "$row" | cut -d'|' -f1)
    project=$(printf '%s' "$row" | cut -d'|' -f2)
    state=$(printf '%s' "$row" | cut -d'|' -f3)
    actor=$(printf '%s' "$row" | cut -d'|' -f4)

    if [[ "$state" == "live" ]]; then
        printf '  Server verify:  \033[32mOK\033[0m  team="%s" project="%s" actor="%s"\n' "$team" "$project" "$actor"
    else
        printf '  Server verify:  \033[31mREVOKED\033[0m  team="%s" project="%s" actor="%s"\n' "$team" "$project" "$actor"
        printf '                    Delete CLAUDE_MEM_SERVER_BETA_* from .vb.env and re-run vibrate.\n'
        return 1
    fi
}

# Count recent agent_events for this project. Useful smoke test that
# hooks are actually firing.
claude_mem::_count_recent_events() {
    local db_url="$1"
    [[ -n "$db_url" && -n "$CLAUDE_MEM_RESOLVED_PROJECT_ID" ]] || return 0

    local count last_age
    count=$(claude_mem::_pg_query "$db_url" "
        SELECT count(*) FROM agent_events
        WHERE project_id = :'pid'
          AND received_at > now() - interval '1 hour';" \
        -v pid="$CLAUDE_MEM_RESOLVED_PROJECT_ID")
    last_age=$(claude_mem::_pg_query "$db_url" "
        SELECT extract(epoch from now() - max(received_at))::int FROM agent_events
        WHERE project_id = :'pid';" \
        -v pid="$CLAUDE_MEM_RESOLVED_PROJECT_ID")

    if [[ -n "$count" ]]; then
        printf '  Recent events:  %s in the last hour (this project)\n' "$count"
        if [[ -n "$last_age" && "$last_age" != "" ]]; then
            printf '                    most recent: %ss ago\n' "$last_age"
        fi
    fi
}

# Public entry point: print a single-screen status report. Returns 0 if
# everything that COULD be checked passed; 1 if any required step failed.
claude_mem::print_status() {
    local cfg_file="${VIBRATOR_CLAUDE_MEM_ENV:-$HOME/.config/vibrator/claude-mem.env}"

    printf '\nclaude-mem status\n'
    printf '=================\n'
    printf '  Admin config:   %s\n' "$cfg_file"

    if [[ ! -f "$cfg_file" ]]; then
        printf '                    \033[31mMISSING\033[0m — run: vibrate --claude-mem-setup\n\n'
        return 1
    fi

    local runtime url database_url
    runtime=$(claude_mem::_read_dotenv "$cfg_file" CLAUDE_MEM_RUNTIME)
    url=$(claude_mem::_read_dotenv      "$cfg_file" CLAUDE_MEM_SERVER_BETA_URL)
    database_url=$(claude_mem::_read_dotenv "$cfg_file" CLAUDE_MEM_SERVER_DATABASE_URL)

    printf '    Runtime:        %s\n' "${runtime:-(unset)}"
    printf '    Server URL:     %s\n' "${url:-(unset)}"
    printf '    Database URL:   %s\n' "$([[ -n "$database_url" ]] && echo "(set — will auto-bootstrap missing keys)" || echo "(unset — bootstrap disabled)")"
    printf '\n'

    local probe_url healthy=0
    probe_url=$(claude_mem::_host_probe_url "$url")
    if [[ -n "$probe_url" ]]; then
        claude_mem::_probe_health "$probe_url" || healthy=1
    fi

    printf '\n'
    printf '  Workspace:      %s\n' "$WORKSPACE"
    printf '  Project name:   %s\n' "$(basename "$WORKSPACE")"

    if ! claude_mem::_resolve_credentials "$cfg_file"; then
        printf '  Cached key:     \033[33mNONE\033[0m\n'
        if [[ -n "$database_url" ]]; then
            printf '                    next `vibrate` will auto-bootstrap a project-scoped key\n'
            printf '                    and persist it to %s/.vb.env\n' "$WORKSPACE"
        else
            printf '                    no DATABASE_URL set — bootstrap disabled. Either add it\n'
            printf '                    to %s or mint a key manually and set\n' "$cfg_file"
            printf '                    CLAUDE_MEM_SERVER_BETA_API_KEY there.\n'
        fi
        printf '\n'
        return "$healthy"
    fi

    local key_short hash hash_short
    key_short="${CLAUDE_MEM_RESOLVED_API_KEY:0:12}..."
    hash=$(claude_mem::_sha256_hex "$CLAUDE_MEM_RESOLVED_API_KEY")
    hash_short="${hash:0:12}"
    printf '  Key source:     %s\n' "$CLAUDE_MEM_RESOLVED_SOURCE"
    printf '    API key:        %s  (sha256 prefix: %s)\n' "$key_short" "$hash_short"
    [[ -n "$CLAUDE_MEM_RESOLVED_TEAM_ID" ]]    && printf '    Team id:        %s\n' "$CLAUDE_MEM_RESOLVED_TEAM_ID"
    [[ -n "$CLAUDE_MEM_RESOLVED_PROJECT_ID" ]] && printf '    Project id:     %s\n' "$CLAUDE_MEM_RESOLVED_PROJECT_ID"
    printf '\n'

    claude_mem::_verify_server_side "$database_url" || healthy=1
    claude_mem::_count_recent_events "$database_url"

    printf '\n'
    return "$healthy"
}
