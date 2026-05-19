#!/bin/sh
# Vibrator welcome banner.
#
# Sourced from each shell's rc file (templates/shells/{bashrc,zshrc,config.fish}).
# Must be POSIX `sh` — runs unchanged under bash, zsh, fish-invoked-sh.
#
# Set VIBRATOR_NO_BANNER=1 in the environment to silence.

[ -n "$VIBRATOR_NO_BANNER" ] && return 0 2>/dev/null

# --- colors (ANSI) ---
# Wrapped in case the terminal doesn't support color — printf %b handles
# the escapes; if the terminal can't render them they appear as literal
# escape codes (acceptable degradation).
C_RESET='\033[0m'
C_BOLD='\033[1m'
C_CYAN='\033[1;36m'
C_GREEN='\033[1;32m'
C_YELLOW='\033[1;33m'
C_RED='\033[1;31m'
C_DIM='\033[0;90m'

_banner_line() {
    printf '%b\n' "${C_CYAN}+--------------------------------------------------------------+${C_RESET}"
}

_banner_line
printf '%b\n' "${C_CYAN}|${C_RESET}  ${C_BOLD}${C_YELLOW}Vibrator${C_RESET} ${C_DIM}— AI coding agent sandbox${C_RESET}                          ${C_CYAN}|${C_RESET}"
_banner_line
echo ""

# --- harness CLI + version ---
# Detect whichever harness binaries are on PATH. Multiple may be
# installed (claude + codex from a "full" profile). Show all that
# respond to --version.
for _cli in claude codex opencode pi; do
    if command -v "$_cli" >/dev/null 2>&1; then
        _v=$("$_cli" --version 2>/dev/null | head -1)
        [ -z "$_v" ] && _v="unknown"
        # Pad CLI name to align the version columns.
        printf '%b%-12s%b  %s\n' "${C_GREEN}" "${_cli}:" "${C_RESET}" "$_v"
    fi
done

# --- auth status ---
# Show the FIRST credential that's set (in preference order). Falls
# back to a "not configured" hint with a tip on how to authenticate.
if [ -n "$CLAUDE_CODE_OAUTH_TOKEN" ]; then
    printf '%b%-12s%b  OAuth token\n' "${C_GREEN}" "auth:" "${C_RESET}"
elif [ -n "$ANTHROPIC_API_KEY" ]; then
    printf '%b%-12s%b  Anthropic API key\n' "${C_GREEN}" "auth:" "${C_RESET}"
elif [ -n "$OPENAI_API_KEY" ]; then
    printf '%b%-12s%b  OpenAI API key\n' "${C_GREEN}" "auth:" "${C_RESET}"
else
    printf '%b%-12s%b  %bnot configured%b\n' \
        "${C_YELLOW}" "auth:" "${C_RESET}" "${C_YELLOW}" "${C_RESET}"
    printf '  %bTip: run `claude setup-token` on host, then restart vibrate.%b\n' \
        "${C_DIM}" "${C_RESET}"
fi

echo ""

# --- profile + tools + extensions (baked at build time) ---
if [ -n "$VIBRATOR_PROFILE" ]; then
    printf '%b%-12s%b  %s\n' "${C_GREEN}" "profile:" "${C_RESET}" "$VIBRATOR_PROFILE"
fi
if [ -n "$VIBRATOR_FEATURES_LIST" ]; then
    # FEATURES_LIST holds the build-time tool/runtime selection
    # (python, go, gh, …) — labeled "tools:" in the banner because
    # that's what end-users perceive them as (CLIs and runtimes
    # available inside the container), not abstract features.
    printf '%b%-12s%b  %s\n' "${C_GREEN}" "tools:" "${C_RESET}" "$VIBRATOR_FEATURES_LIST"
fi
if [ -n "$VIBRATOR_EXTENSIONS_LIST" ]; then
    # EXTENSIONS_LIST is the selected set of plugins, MCP servers,
    # skills, and subagents that extend the agent's capabilities.
    # Read from extensions/<harness>/*.md at build time.
    printf '%b%-12s%b  %s\n' "${C_GREEN}" "extensions:" "${C_RESET}" "$VIBRATOR_EXTENSIONS_LIST"
fi

# --- workspace ---
echo ""
printf '%b%-12s%b  %s\n' "${C_GREEN}" "workspace:" "${C_RESET}" "${WORKSPACE_PATH:-$(pwd)}"
echo ""

# --- footer hint ---
# Suggest the right "first command" per harness. If multiple harnesses
# are installed, prefer claude (the original target).
if command -v claude >/dev/null 2>&1; then
    printf '%b\n' "${C_DIM}Run \`claude --help\` to get started.${C_RESET}"
elif command -v codex >/dev/null 2>&1; then
    printf '%b\n' "${C_DIM}Run \`codex --help\` to get started.${C_RESET}"
fi
echo ""
