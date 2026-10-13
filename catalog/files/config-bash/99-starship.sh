# Interactive shells only, and not on the Linux console, which can't show
# starship's symbols.
case $- in *i*) ;; *) return ;; esac
[ "$TERM" = linux ] || eval "$(starship init bash)"
