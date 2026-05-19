# Vibrator config.fish.
# Lives in /etc/skel/.config/fish/config.fish at image-build time;
# useradd -m copies it into the unprivileged user's home. Edit
# templates/shells/config.fish in the source repo to change behavior.

# fish has built-in history; we just bump the cap.
set -U fish_history_max 50000 2>/dev/null

# --- aliases ----------------------------------------------------------
alias ll 'ls -la'

# See zshrc for the rationale on `--dangerously-skip-permissions`.
alias claude 'claude --dangerously-skip-permissions'
alias claude-safe 'command claude'

if type -q nvim
    alias vim 'nvim'
    alias vi 'nvim'
end

# --- environment ------------------------------------------------------
set -gx GIT_SSH_COMMAND 'ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'

# Ensure ~/.local/bin is on PATH (fish_user_paths is the idiomatic way).
if not contains $HOME/.local/bin $fish_user_paths
    set -U fish_user_paths $HOME/.local/bin $fish_user_paths 2>/dev/null
end

# --- prompt -----------------------------------------------------------
# user@host in vibrator-yellow, workspace in green. Hostname is set
# to `vibrate-<workspace>` by docker run --hostname; the yellow
# user@vibrate-XYZ section signals "inside a vibrator container".
function fish_prompt
    set_color yellow
    echo -n (whoami)'@'(hostname)
    set_color normal
    echo -n ' '
    set_color green
    echo -n (prompt_pwd)
    set_color normal
    echo -n ' $ '
end

# --- welcome banner ---------------------------------------------------
if status is-interactive
    if test -z "$VIBRATOR_NO_BANNER"; and test -r /opt/vibrator/welcome.sh
        sh /opt/vibrator/welcome.sh
    end
end
