# debforge

Manage packages outside Debian repositories. Extensible base with self-update and self-remove.

## Install

```bash
curl -sSfL https://raw.githubusercontent.com/hmwassim/debforge/main/install.sh | sudo bash
```

## Usage

```
debforge install <package>...     Install packages from external repos
debforge remove <package>...      Remove packages and repo sources
debforge update <package>...      Update specific packages
debforge update --all             Update system and all installed deb packages
debforge list                     List managed packages
debforge search <query>           Search managed packages
debforge core setup               Set up core packages and configs (idempotent)
debforge core setup -f, --force   Force re-apply all core packages and configs
debforge core list                List core packages and status
debforge --self-update            Install or update debforge
debforge --self-remove            Remove debforge and all data
debforge --version, -V            Show version
debforge --help, -h               Show this help message
```
