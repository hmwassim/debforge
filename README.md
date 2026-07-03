# debforge

Package manager for deb, apt, source, and config packages defined via YAML.

```
curl -sL https://raw.githubusercontent.com/hmwassim/debforge/main/inshall.sh | sudo bash
```

## What debforge does with root

debforge is designed to run under `sudo`. It performs system-level operations
that require root:
- Runs `apt-get` and `dpkg` to install, remove, and query system packages.
- Writes configuration files under `/etc/` and user home directories.
- Installs software to `/usr/local/bin/`.
- Manages its own installation at `/opt/debforge/`.
- Reads and writes package state to a JSON file (typically under
  `/opt/debforge/`).

This is not a sandboxed or rootless tool — it assumes full system trust.

## Supported distributions

The apt backend defaults the backports suite to `trixie-backports` (Debian
Trixie). On other Debian/Ubuntu releases you must set `backport_suite` per
package if you use the `backports` feature, or override the default at build
time. The tool itself works on any Debian-based distribution with Go 1.21+,
but backports behavior is release-specific.

## Package schema

Packages are YAML files placed under `repo/packages/<type>/`. Each file
defines one package. The four types are:

### apt — system packages from apt repositories

```yaml
name: firefox
description: Firefox web browser from Mozilla
type: apt
install:
  extrepo:          # optional: enable an extrepo repository
    - mozilla
  packages:         # packages to apt-get install
    - firefox
  conflicts:        # optional: error if these are installed
    - firefox-esr
depends:            # optional: other debforge packages required first
  - some-other-pkg
```

### deb — .deb packages downloaded from a URL

```yaml
name: vscodium
description: VS Code without Microsoft branding
type: deb
package: codium              # primary system package name (for installed checks)
install:
  url: https://github.com/VSCodium/vscodium/releases/download/{version}/codium_{version}_amd64.deb
depends:
  - some-other-pkg
```

### source — build from source (clone + script)

```yaml
name: pfetch
description: Pretty fetch system information tool
type: source
install:
  repo: https://github.com/Gobidev/pfetch-rs.git
  skip_clone: true
  url: "https://github.com/Gobidev/pfetch-rs/releases/download/v{version}/pfetch-linux-gnu-x86_64.tar.gz"
  install: |
    cp pfetch /usr/local/bin/pfetch
    chmod +x /usr/local/bin/pfetch
remove:
  script: |
    rm -f /usr/local/bin/pfetch
```

### config — static configuration files

```yaml
name: system-settings
description: System-wide performance and power tuning
type: config
install:
  configs:
    /etc/systemd/system.conf.d/00-timeout.conf: systemd-00-timeout.conf
    /etc/systemd/journald.conf.d/00-journal-size.conf: journald-00-size.conf
  user_configs:
    ~/.config/my-app/settings.yaml: my-app-settings.yaml
post_install: |
  sysctl --system
```

Full real-world examples are in `repo/packages/`.

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
    setup               Provision system (repos, firmware, desktop)
        --force         Skip checks, reapply all steps
    --self-update       Update debforge itself
    --self-remove       Remove debforge from system
    --help              Show this help
    --version           Show version
```
