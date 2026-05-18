# Host-side claude-mem auto-bootstrap.
#
# Flow when the admin dotenv has CLAUDE_MEM_SERVER_DATABASE_URL but the
# workspace .vb.env has no CLAUDE_MEM_SERVER_BETA_API_KEY:
#
#   1. generate a fresh "cmem_<48-hex>" plaintext token + sha256 hash
#   2. run a SERIALIZABLE Postgres transaction via a one-shot
#      postgres:16-alpine container that:
#        - upserts team "vibrators"          (idempotent on name)
#        - upserts project = $(basename PWD) (idempotent on (team,name))
#        - revokes any prior live keys for (team,project,actor)
#        - inserts a new key with scopes = ["*"]  (project-scoped → wildcard
#          is safe; the project_id boundary IS the real security perimeter)
#   3. append "CLAUDE_MEM_SERVER_BETA_*" lines to $WORKSPACE/.vb.env so
#      subsequent vibrate invocations skip the bootstrap entirely
#
# The DB credentials never enter the vibrator container. Only the
# project-scoped Bearer token does. After bootstrap, docker_cmd.sh forwards
# the key + URL + IDs but NEVER the DATABASE_URL.

readonly _CMB_PG_IMAGE="postgres:16-alpine"
readonly _CMB_DEFAULT_TEAM="vibrators"
readonly _CMB_KEY_PREFIX="cmem_"

# Rewrite localhost / 127.0.0.1 in the DB URL so the one-shot postgres
# container can reach the host's Postgres. Admin dotenv typically holds the
# URL with the host that the *vibrator container* would use
# (host.docker.internal:5432). The one-shot container needs the same trick,
# which we enable with --add-host=host.docker.internal:host-gateway.
claude_mem_bootstrap::_rewrite_db_url() {
    local url="$1"
    # Delimiter is '#' (not '|') because the regex contains '|' alternation,
    # and BSD sed (macOS) treats the first '|' after 's' as the pattern/replacement
    # separator — breaks with "parentheses not balanced" on the alternation group.
    printf '%s' "$url" | sed -E 's#//([^/@]*@)?(localhost|127\.0\.0\.1)([:/])#//\1host.docker.internal\3#'
}

# Run an SQL string against the DB URL via a one-shot psql container.
# The SQL is fed on STDIN (via `-f -`), NOT `-c`, because psql's `:'name'`
# variable interpolation is more reliable in script mode than in -c mode.
# Pass `-v key=value` as extra args to set variables.
#
# psql flags:
#   -t  tuples only (no column headers / row counts)
#   -A  unaligned output (no padding)
#   -q  quiet — suppresses BEGIN/UPDATE/INSERT/COMMIT command tags
#   -f - read SQL from stdin
#
# stderr is intentionally NOT captured — psql errors flow to the user's terminal
# unfiltered. Callers wrap the call in `if !` so set -e doesn't abort silently.
claude_mem_bootstrap::_pg_query() {
    local db_url="$1" sql="$2"
    shift 2
    printf '%s\n' "$sql" | docker run --rm -i \
        --add-host=host.docker.internal:host-gateway \
        "$_CMB_PG_IMAGE" \
        psql "$db_url" -tAq -v ON_ERROR_STOP=1 "$@" -f -
}

# Pre-flight: pull the postgres image (so we don't surface a docker pull
# during the bootstrap, which can confuse the timing of error output).
claude_mem_bootstrap::_ensure_image() {
    docker image inspect "$_CMB_PG_IMAGE" >/dev/null 2>&1 && return 0
    log::verbose "claude-mem: pulling $_CMB_PG_IMAGE (one-time)"
    docker pull "$_CMB_PG_IMAGE" >/dev/null 2>&1
}

# fresh "cmem_<48-hex>" plaintext (printed once, never persisted plain
# anywhere except the workspace .vb.env).
claude_mem_bootstrap::_gen_key() {
    printf '%s%s' "$_CMB_KEY_PREFIX" "$(openssl rand -hex 24)"
}

# sha256 hex with NO trailing newline — matches the server's hash-on-write.
claude_mem_bootstrap::_sha256_hex() {
    printf '%s' "$1" | sha256sum | awk '{print $1}'
}

# True if $WORKSPACE/.vb.env already carries a project-scoped API key. We
# look at $WORKSPACE specifically (not the walked-up pin file) because the
# bootstrap writes to the workspace root and that's the canonical home for
# the cached credential.
claude_mem_bootstrap::is_cached() {
    local pin="$WORKSPACE/.vb.env"
    [[ -f "$pin" ]] || return 1
    grep -q '^[[:space:]]*CLAUDE_MEM_SERVER_BETA_API_KEY=' "$pin"
}

# Lookup or insert a team by name. Echoes the team_id on stdout. Returns 1 on failure.
claude_mem_bootstrap::_get_or_create_team() {
    local db_url="$1" team_name="$2"
    local id
    id=$(claude_mem_bootstrap::_pg_query "$db_url" \
        "SELECT id FROM teams WHERE name = :'name' LIMIT 1" \
        -v name="$team_name") || return 1
    if [[ -n "$id" ]]; then
        printf '%s' "$id"
        return 0
    fi
    claude_mem_bootstrap::_pg_query "$db_url" \
        "INSERT INTO teams (id, name) VALUES (gen_random_uuid()::text, :'name') RETURNING id" \
        -v name="$team_name"
}

# Lookup or insert a project by (team_id, name). Echoes the project_id on stdout.
claude_mem_bootstrap::_get_or_create_project() {
    local db_url="$1" team_id="$2" project_name="$3"
    local id
    id=$(claude_mem_bootstrap::_pg_query "$db_url" \
        "SELECT id FROM projects WHERE team_id = :'tid' AND name = :'name' LIMIT 1" \
        -v tid="$team_id" -v name="$project_name") || return 1
    if [[ -n "$id" ]]; then
        printf '%s' "$id"
        return 0
    fi
    claude_mem_bootstrap::_pg_query "$db_url" \
        "INSERT INTO projects (id, team_id, name) VALUES (gen_random_uuid()::text, :'tid', :'name') RETURNING id" \
        -v tid="$team_id" -v name="$project_name"
}

# Revoke any live key for (team, project, actor) and mint a new one, both in
# a single transaction. scopes=["*"] is the wildcard accepted by claude-mem's
# scope-check function K9 — safe because project_id locks the blast radius.
# psql runs multiple `;`-separated statements in -c as one transaction by default.
claude_mem_bootstrap::_rotate_key() {
    local db_url="$1" team_id="$2" project_id="$3" actor_id="$4" key_hash="$5"
    claude_mem_bootstrap::_pg_query "$db_url" "
        UPDATE api_keys SET revoked_at = now()
         WHERE team_id    = :'tid'
           AND project_id = :'pid'
           AND actor_id   = :'actor'
           AND revoked_at IS NULL;
        INSERT INTO api_keys (id, key_hash, team_id, project_id, actor_id, scopes)
        VALUES (gen_random_uuid()::text, :'hash', :'tid', :'pid', :'actor', '[\"*\"]'::jsonb);
    " \
        -v tid="$team_id" \
        -v pid="$project_id" \
        -v actor="$actor_id" \
        -v hash="$key_hash"
}

# Run the full bootstrap. Reads SERVER_URL + DATABASE_URL from arguments,
# generates a key, runs the SQL, appends to .vb.env, exports the resolved
# env vars for the rest of this vibrate invocation.
#
# Args:  $1 = CLAUDE_MEM_SERVER_BETA_URL  (forwarded to container later)
#        $2 = CLAUDE_MEM_SERVER_DATABASE_URL (host-only, never forwarded)
#
# Side effects on success:
#   - appends a "# claude-mem (auto-bootstrapped ...)" block to
#     $WORKSPACE/.vb.env (created if missing)
#   - exports CLAUDE_MEM_SERVER_BETA_{API_KEY,TEAM_ID,PROJECT_ID}
#
# Returns 0 on success, 1 on failure. Caller logs and continues without
# claude-mem wiring (graceful degradation — don't block the container).
claude_mem_bootstrap::run() {
    local server_url="$1"
    local database_url="$2"

    local team_name="${VIBRATOR_CLAUDE_MEM_TEAM_NAME:-$_CMB_DEFAULT_TEAM}"
    local project_name actor_id raw_key key_hash db_url
    project_name=$(basename "$WORKSPACE")
    actor_id="vibrator:$(hostname):$WORKSPACE"
    raw_key=$(claude_mem_bootstrap::_gen_key)
    key_hash=$(claude_mem_bootstrap::_sha256_hex "$raw_key")
    db_url=$(claude_mem_bootstrap::_rewrite_db_url "$database_url")

    if ! claude_mem_bootstrap::_ensure_image; then
        log::warn "claude-mem: cannot pull $_CMB_PG_IMAGE — bootstrap skipped"
        return 1
    fi

    log::info "claude-mem: bootstrapping team='$team_name' project='$project_name'"

    # Three discrete psql invocations — each easier to debug than one big
    # script with conditionals + \gset. Run under `if !` so a psql error
    # surfaces to the terminal before set -e aborts the function.
    local team_id project_id
    if ! team_id=$(claude_mem_bootstrap::_get_or_create_team "$db_url" "$team_name"); then
        log::error "claude-mem: team upsert failed (see psql output above)"
        return 1
    fi
    team_id=$(printf '%s' "$team_id" | tr -d '[:space:]')
    if [[ -z "$team_id" ]]; then
        log::error "claude-mem: team upsert produced empty id"
        return 1
    fi
    log::verbose "claude-mem: team_id=$team_id"

    if ! project_id=$(claude_mem_bootstrap::_get_or_create_project "$db_url" "$team_id" "$project_name"); then
        log::error "claude-mem: project upsert failed (see psql output above)"
        return 1
    fi
    project_id=$(printf '%s' "$project_id" | tr -d '[:space:]')
    if [[ -z "$project_id" ]]; then
        log::error "claude-mem: project upsert produced empty id"
        return 1
    fi
    log::verbose "claude-mem: project_id=$project_id"

    if ! claude_mem_bootstrap::_rotate_key "$db_url" "$team_id" "$project_id" "$actor_id" "$key_hash"; then
        log::error "claude-mem: API key rotation failed (see psql output above)"
        return 1
    fi

    claude_mem_bootstrap::_persist_to_vb_env "$raw_key" "$team_id" "$project_id"
    claude_mem_bootstrap::_ensure_gitignored

    export CLAUDE_MEM_SERVER_BETA_API_KEY="$raw_key"
    export CLAUDE_MEM_SERVER_BETA_TEAM_ID="$team_id"
    export CLAUDE_MEM_SERVER_BETA_PROJECT_ID="$project_id"

    log::success "claude-mem: bootstrapped '$project_name' (team_id=${team_id:0:8}… project_id=${project_id:0:8}…)"
    log::info    "claude-mem: project-scoped key cached in $WORKSPACE/.vb.env"
    return 0
}

# If the workspace has a .gitignore, append `.vb.env` to it (with a
# header comment) so the freshly-minted plaintext API key never ends up
# in version control. Only touches the file when:
#   - .gitignore exists (we don't create one — that would be intrusive for
#     workspaces that deliberately track everything)
#   - .vb.env isn't already listed (exact line match, idempotent)
# Does NOT `git add` / stage / commit anything — purely a filesystem write.
claude_mem_bootstrap::_ensure_gitignored() {
    local gi="$WORKSPACE/.gitignore"
    [[ -f "$gi" ]] || return 0
    # Exact-line match anchored to start/end. Tolerates trailing CR/whitespace
    # if someone hand-edited on Windows / pasted weirdly.
    if grep -qE '^[[:space:]]*\.vb\.env[[:space:]]*$' "$gi"; then
        log::verbose "claude-mem: .gitignore already covers .vb.env"
        return 0
    fi
    {
        # Add a leading newline only if the file doesn't end with one already,
        # to avoid the "appended to last line" gotcha.
        [[ -s "$gi" && -z "$(tail -c1 "$gi")" ]] || printf '\n'
        printf '# vibrator: workspace pin, contains a plaintext claude-mem API key\n'
        printf '.vb.env\n'
    } >> "$gi"
    log::info "claude-mem: added .vb.env to $gi"
}

# Append the auto-bootstrap block to $WORKSPACE/.vb.env. Preserves any
# existing content (PROFILE/WITH/NO lines from the menu writer or the user).
claude_mem_bootstrap::_persist_to_vb_env() {
    local raw_key="$1" team_id="$2" project_id="$3"
    local pin="$WORKSPACE/.vb.env"
    local now
    now=$(date -u +%Y-%m-%dT%H:%M:%SZ)

    {
        if [[ ! -f "$pin" ]]; then
            printf '# vibrator workspace pin — auto-created by claude-mem bootstrap\n'
        else
            printf '\n'
        fi
        printf '# claude-mem (auto-bootstrapped %s by vibrator)\n' "$now"
        printf 'CLAUDE_MEM_SERVER_BETA_API_KEY=%s\n'    "$raw_key"
        printf 'CLAUDE_MEM_SERVER_BETA_TEAM_ID=%s\n'    "$team_id"
        printf 'CLAUDE_MEM_SERVER_BETA_PROJECT_ID=%s\n' "$project_id"
    } >> "$pin"

    chmod 600 "$pin" 2>/dev/null || true
}
