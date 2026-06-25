# debforge

Package manager for deb, apt, source, and config packages defined via YAML.

```
curl -sL https://raw.githubusercontent.com/hmwassim/debforge/main/inshall.sh | sudo bash
```

## Usage

```
debforge - package manager

Usage: debforge [flags] <command> [<name>...]

Flags:
    -y, --yes           Skip confirmation prompts
    -f, --force         Force operation (reinstall)
    -a, --all           Update all packages (update only)

Commands:
    install <name>...    Install packages
    remove <name>...    Remove packages from system
    update [<name>...]   Reinstall packages (runs apt-get update)
        --all           Update all packages and run apt-get upgrade
    --self-update       Update debforge itself
    --self-remove       Remove debforge from system
    --help              Show this help
    --version           Show version
```
