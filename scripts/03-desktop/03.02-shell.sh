#!/usr/bin/env bash
set -euo pipefail

echo "==> Installing shell utilities..."
sudo apt install -y \
    eza \
    starship \
    papirus-icon-theme \
    fastfetch \
    bat \
    ripgrep

BASHRC_D="$HOME/.config/bashrc.d"
CUSTOM_FILE="$BASHRC_D/00-custom.sh"
BASHRC="$HOME/.bashrc"
LOADER_MARKER="# source ~/.config/bashrc.d"

mkdir -p "$BASHRC_D"

if ! grep -qF "$LOADER_MARKER" "$BASHRC"; then
    cat << 'BASHRC_LOADER' >> "$BASHRC"

# source ~/.config/bashrc.d
if [ -d "$HOME/.config/bashrc.d" ]; then
    for _f in "$HOME/.config/bashrc.d"/*.sh; do
        [ -r "$_f" ] && . "$_f"
    done
    unset _f
fi
BASHRC_LOADER
    echo "Loader added to ~/.bashrc."
fi

cat > "$CUSTOM_FILE" << 'CUSTOM'
bind "set completion-ignore-case on"

alias ls='eza -al --icons --color=always --group-directories-first'
alias la='eza -a  --icons --color=always --group-directories-first'
alias ll='eza -l  --icons --color=always --group-directories-first'
alias lt='eza -aT --icons --color=always --group-directories-first'

alias cat='batcat --style=plain --pager=never'

# Starship prompt
eval "$(starship init bash)"
CUSTOM

