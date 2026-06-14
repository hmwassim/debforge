# debforge

Manage packages outside Debian repositories. Extensible base with self-update and self-remove.

## Install

```bash
curl -sSfL https://raw.githubusercontent.com/hmwassim/debforge/main/install.sh | sudo bash
```

## Usage

```
debforge --self-update    Install or update debforge
debforge --self-remove    Remove debforge and all data
debforge --version, -V    Show version
debforge --help, -h       Show this help message

debforge core update      Upgrade packages to latest versions
debforge core repair      Install dependencies and apply system configuration
debforge core list        Show installed and missing packages
```
