function __debforge_list_available_packages
    set -l pkgs_dir (string trim -- "$DEBFORGE_PKGS_DIR")
    if test -z "$pkgs_dir"
        set pkgs_dir /opt/debforge/src/repo/packages
    end
    set -l all_pkgs (grep -rh '^name:' "$pkgs_dir" 2>/dev/null | awk '{print $2}' | sort -u)

    set -l cmd (commandline -pco)
    set -l after_debforge 0
    for word in $cmd
        if test "$word" = "debforge"
            set after_debforge 1
        else if test $after_debforge -eq 1
            if contains -- "$word" $all_pkgs
                set all_pkgs (string match -v "$word" -- $all_pkgs)
            end
        end
    end
    for pkg in $all_pkgs; echo $pkg; end
end

complete -c debforge -n "not __fish_seen_subcommand_from install remove update setup doctor search" \
    -a "install remove update setup doctor search"

for cmd in install remove update search
    complete -c debforge -n "__fish_seen_subcommand_from $cmd" \
        -a "(__debforge_list_available_packages)"
end
