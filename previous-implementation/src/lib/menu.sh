# Interactive workspace picker.
#
# Shown on the first `vibrate` invocation in a workspace when:
#   - stdin is a TTY
#   - the user didn't already specify a profile / feature flags
#   - no .vb.env exists in $PWD or any git-tracked ancestor
#   - no container for this workspace exists yet
#   - --no-menu / VIBRATOR_NO_MENU isn't set
#   - we're not in a non-interactive mode (--build, --explain, --pull, ...)
#
# Two-stage flow:
#   1. Pick an existing variant OR a profile to build new from
#   2. (if "build new") toggle individual features on top of the profile
# Then optionally pin the choice into ./.vb.env.

# Public: should the menu be shown at all? Returns 0 to show, 1 to skip.
# When skipping, emits a verbose-mode log line naming the reason — invaluable
# when a user wonders "why didn't the menu fire?".
menu::should_show() {
    if [[ "$INTERACTIVE" != true ]]; then
        log::verbose "menu: skipped (non-interactive mode)"; return 1
    fi
    if [[ ! -t 0 ]]; then
        log::verbose "menu: skipped (stdin is not a TTY)"; return 1
    fi
    if [[ -n "${VIBRATOR_NO_MENU:-}" ]]; then
        log::verbose "menu: skipped (VIBRATOR_NO_MENU is set)"; return 1
    fi
    if [[ "${FLAG_NO_MENU:-false}" != false ]]; then
        log::verbose "menu: skipped (--no-menu flag)"; return 1
    fi
    if [[ -n "${PIN_SOURCE:-}" ]]; then
        log::verbose "menu: skipped (pin file: $PIN_SOURCE)"; return 1
    fi
    if [[ "${USER_SPECIFIED_FEATURES:-false}" != false ]]; then
        log::verbose "menu: skipped (CLI feature flags provided)"; return 1
    fi
    if [[ "${FLAG_BUILD_ONLY:-false}" != false ]]; then
        log::verbose "menu: skipped (--build)"; return 1
    fi
    if [[ "${FLAG_EXPLAIN_FEATURES:-false}" != false ]]; then
        log::verbose "menu: skipped (--explain)"; return 1
    fi
    if [[ -n "${EXPORT_DOCKERFILE:-}" ]]; then
        log::verbose "menu: skipped (--export-dockerfile)"; return 1
    fi
    if [[ -n "${FLAG_PULL:-}" ]]; then
        log::verbose "menu: skipped (--pull)"; return 1
    fi

    # If a container already exists for this workspace (any variant), the
    # reuse path in main.sh will handle it. Don't pester the user.
    local found
    found=$(docker ps -a --filter "label=vibrator.workspace=$WORKSPACE" \
            --format '{{.Names}}' 2>/dev/null | grep '^claude-vb-' || true)
    if [[ -n "$found" ]]; then
        log::verbose "menu: skipped (containers exist for this workspace: $(echo "$found" | tr '\n' ',' | sed 's/,$//'))"
        return 1
    fi

    return 0
}

# Public entry. Mutates IMAGE_NAME / PROFILE / FEATURES_ENABLED /
# VARIANT_FINGERPRINT based on user choice. Optionally writes .vb.env.
menu::main() {
    echo ""
    log::info "$WORKSPACE is a fresh workspace and no .vb.env was found."
    echo ""

    local -a existing_lines=()
    local line
    while IFS= read -r line; do
        [[ -n "$line" ]] && existing_lines+=("$line")
    done < <(menu::_list_existing_variants)

    local existing_count=${#existing_lines[@]}
    local i base

    if [[ "$existing_count" -gt 0 ]]; then
        echo "Use an existing variant on this machine:"
        i=1
        for line in "${existing_lines[@]}"; do
            local profile fp size created _features _image
            IFS='|' read -r profile fp size created _features _image <<< "$line"
            printf "  %2d) %-15s %-10s %-9s built %s\n" "$i" "$profile" "$fp" "$size" "$created"
            i=$((i + 1))
        done
        echo ""
    else
        echo "No prebuilt variants found on this machine."
        echo ""
    fi

    echo "Build a new variant:"
    base=$existing_count
    printf "  %2d) minimal       (~150 MB)  shell + dev-cli only (jq, yq, ripgrep, fzf, glow, ...)\n" "$((base + 1))"
    printf "  %2d) backend       (~600 MB)  + python, go, gh, serena, claude-mem, codex  (no Playwright, no audit toolkit)\n" "$((base + 2))"
    printf "  %2d) default       (~2 GB)    everything except aider  [default if you press Enter]\n" "$((base + 3))"
    printf "  %2d) kitchen-sink  (~2.1 GB)  everything including aider AI pair programmer\n" "$((base + 4))"
    echo ""
    echo "Other:"
    echo "   q) Quit (re-run with explicit --profile / --with-* / --no-* flags)"
    echo ""

    local default_choice=$((base + 3))
    local choice
    read -r -p "Choice [$default_choice]: " choice
    choice="${choice:-$default_choice}"

    case "$choice" in
        q|Q) log::info "Aborted by user."; exit 0 ;;
    esac
    if ! [[ "$choice" =~ ^[0-9]+$ ]]; then
        log::die "Invalid choice: '$choice'"
    fi

    if [[ "$existing_count" -gt 0 && "$choice" -ge 1 && "$choice" -le "$existing_count" ]]; then
        menu::_apply_existing_variant "${existing_lines[$((choice - 1))]}"
        menu::_offer_pin
        menu::_claude_mem_post_pick
        return
    fi

    local profile_idx=$((choice - existing_count))
    local profile
    case "$profile_idx" in
        1) profile="minimal" ;;
        2) profile="backend" ;;
        3) profile="default" ;;
        4) profile="kitchen-sink" ;;
        *) log::die "Invalid choice: $choice (out of range)" ;;
    esac

    config::apply_profile "$profile"
    menu::_toggle_features
    menu::_offer_pin
    menu::_claude_mem_post_pick
}

# If the user's final selection includes claude-mem AND the host hasn't been
# configured yet, print the full setup story once. Quiet otherwise.
menu::_claude_mem_post_pick() {
    config::feature_enabled "claude-mem" || return 0
    config::claude_mem_configured && return 0
    config::print_claude_mem_setup
}

# List existing vibrator images for this user as pipe-separated rows:
#   profile|fingerprint|size|created|features_csv|full_image_tag
menu::_list_existing_variants() {
    local repo tag size created suffix profile fp features
    # docker images CLI is fastest for the basic listing; LABEL inspect per
    # image fills in the feature list.
    while IFS='|' read -r repo tag size created; do
        # Match only images whose name starts with claude-vb-<user>-
        case "$repo" in
            "claude-vb-${CFG_USERNAME}-"*) ;;
            *) continue ;;
        esac
        # repo = claude-vb-<user>-<profile>-<fp> (profile may contain hyphens
        # if we add one later, so split conservatively: last segment = fp,
        # everything between user and that = profile).
        suffix="${repo#claude-vb-${CFG_USERNAME}-}"
        fp="${suffix##*-}"
        profile="${suffix%-*}"
        features=$(docker inspect "$repo:$tag" \
            --format '{{index .Config.Labels "vibrator.features"}}' 2>/dev/null \
            | tr -d '\r')
        # Fall back: if label is missing (older image), show the profile name as a hint
        [[ -z "$features" ]] && features="(no label; rebuild for details)"
        printf '%s|%s|%s|%s|%s|%s:%s\n' \
            "$profile" "$fp" "$size" "$created" "$features" "$repo" "$tag"
    done < <(docker images --format '{{.Repository}}|{{.Tag}}|{{.Size}}|{{.CreatedSince}}' 2>/dev/null)
}

# Adopt an existing variant: lock the image tag and reconstruct features
# from its label so subsequent code (welcome message, env forwarding) sees
# the right set.
menu::_apply_existing_variant() {
    local line="$1"
    local profile fp size created features image
    IFS='|' read -r profile fp size created features image <<< "$line"

    IMAGE_NAME="$image"
    PROFILE="$profile"
    VARIANT_FINGERPRINT="$fp"
    IMAGE_NAME_LOCKED=true

    # Repopulate FEATURES_ENABLED from label so anything downstream that
    # checks config::feature_enabled (e.g., feature-gated docker_cmd blocks)
    # gets accurate answers.
    FEATURES_ENABLED=""
    local f
    for f in $features; do
        config::is_known_feature "$f" || continue
        config::feature_enable "$f"
    done

    log::info "Using existing variant: $IMAGE_NAME"
}

# Interactive checkbox loop on top of a profile's defaults.
menu::_toggle_features() {
    local input target f
    while true; do
        local enabled_count=0
        echo ""
        echo "Customize features for profile '$PROFILE':"
        echo "(type a number to toggle, or:  d) done   r) reset   q) quit)"
        echo ""
        local i=1
        for f in "${FEATURE_CATALOG[@]}"; do
            local mark=" "
            if config::feature_enabled "$f"; then
                mark="x"
                enabled_count=$((enabled_count + 1))
            fi
            local deps
            deps=$(config::feature_deps "$f")
            local note=""
            [[ -n "$deps" ]] && note=" \033[1;90m(needs: $deps)\033[0m"
            printf "  [%s] %2d) %-15s${note}\n" "$mark" "$i" "$f"
            i=$((i + 1))
        done
        echo ""
        printf "  %d of %d enabled\n\n" "$enabled_count" "${#FEATURE_CATALOG[@]}"

        read -r -p "Toggle [d=done]: " input
        input="${input:-d}"

        case "$input" in
            d|D) break ;;
            q|Q) log::info "Aborted by user."; exit 0 ;;
            r|R) config::apply_profile "$PROFILE"; continue ;;
            *)
                if [[ "$input" =~ ^[0-9]+$ ]] && \
                   [[ "$input" -ge 1 ]] && \
                   [[ "$input" -le "${#FEATURE_CATALOG[@]}" ]]; then
                    target="${FEATURE_CATALOG[$((input - 1))]}"
                    if config::feature_enabled "$target"; then
                        config::feature_disable "$target"
                    else
                        config::feature_enable "$target"
                    fi
                else
                    log::warn "Invalid input: '$input'"
                fi
                ;;
        esac
    done

    # Re-run dependency validator after user fiddling (auto-enable missing deps
    # with a warning, same as the CLI flow).
    config::validate_features
}

# Ask whether to write the resolved choice into ./.vb.env for next time.
menu::_offer_pin() {
    local ans
    echo ""
    read -r -p "Pin this choice to $PWD/.vb.env? [y/N]: " ans
    case "$ans" in
        y|Y|yes|YES) menu::_write_pin_file ;;
        *) log::info "Not pinning. You can pin later by writing $PWD/.vb.env manually." ;;
    esac
}

# Record current state as deltas against the chosen profile. Avoids
# capturing the full feature list, so the pin file stays small and is
# legible at a glance.
menu::_write_pin_file() {
    local pin_file="$PWD/.vb.env"
    local profile_features f with="" no=""

    profile_features=$(menu::_features_for_profile "$PROFILE")

    for f in "${FEATURE_CATALOG[@]}"; do
        local in_profile=0 in_current=0
        case " $profile_features " in *" $f "*) in_profile=1 ;; esac
        config::feature_enabled "$f" && in_current=1

        if [[ "$in_current" == 1 && "$in_profile" == 0 ]]; then
            with="$with $f"
        elif [[ "$in_current" == 0 && "$in_profile" == 1 ]]; then
            no="$no $f"
        fi
    done
    with="${with# }"
    no="${no# }"

    {
        echo "# vibrator project pin — generated by interactive menu"
        echo "# Edit by hand or re-run \`vibrate\` and pick a new variant."
        echo "PROFILE=$PROFILE"
        [[ -n "$with" ]] && echo "WITH=\"$with\""
        [[ -n "$no" ]]   && echo "NO=\"$no\""
    } > "$pin_file"

    log::success "Pinned to $pin_file (PROFILE=$PROFILE)"
}

# Snapshot what enabling a profile WOULD produce, without disturbing the
# current FEATURES_ENABLED state.
menu::_features_for_profile() {
    local profile="$1"
    local saved="$FEATURES_ENABLED"
    local saved_profile="$PROFILE"
    config::apply_profile "$profile" >/dev/null 2>&1
    local result="$FEATURES_ENABLED"
    FEATURES_ENABLED="$saved"
    PROFILE="$saved_profile"
    echo "$result"
}
