bind "set completion-ignore-case on"

alias ls='eza -al --icons --color=always --group-directories-first'
alias la='eza -a  --icons --color=always --group-directories-first'
alias ll='eza -l  --icons --color=always --group-directories-first'
alias lt='eza -aT --icons --color=always --group-directories-first'

alias cat='batcat --style=plain --pager=never'

eval "$(starship init bash)"
