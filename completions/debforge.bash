_debforge() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local commands="install remove update setup doctor search"

    local idx
    for ((idx=0; idx<COMP_CWORD; idx++)); do
        [[ "${COMP_WORDS[idx]}" == "debforge" ]] && break
    done

    local verb="${COMP_WORDS[idx+1]}"

    if [[ $COMP_CWORD -eq $((idx + 1)) ]]; then
        COMPREPLY=($(compgen -W "$commands" -- "$cur"))
        return
    fi

    case "$verb" in
        install|remove|update|search)
            local pkgs_dir="${DEBFORGE_PKGS_DIR:-/opt/debforge/src/repo/packages}"
            local all_pkgs
            all_pkgs=$(grep -rh '^name:' "$pkgs_dir" 2>/dev/null | awk '{print $2}' | sort -u)
            local exclude=""
            for ((i=idx+2; i<COMP_CWORD; i++)); do
                local w="${COMP_WORDS[i]}"
                [[ "$w" != -* ]] && exclude+=" -e $w"
            done
            if [[ -n "$exclude" ]]; then
                COMPREPLY=($(compgen -W "$all_pkgs" -- "$cur" | grep -vF $exclude))
            else
                COMPREPLY=($(compgen -W "$all_pkgs" -- "$cur"))
            fi
            ;;
        setup|doctor)
            COMPREPLY=()
            ;;
    esac
}

complete -F _debforge debforge
