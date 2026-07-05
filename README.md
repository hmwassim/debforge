# debforge

A package manager for Debian (Trixie) that installs `apt`, `.deb`, source-built,
and config-file packages from YAML definitions.

```
curl -sL https://raw.githubusercontent.com/hmwassim/debforge/main/inshall.sh | sudo bash
```

The installer also rewrites `/etc/apt/sources.list` to enable
`contrib`/`non-free`/`non-free-firmware`/`backports` (required for several
packages in this repo) and runs a full system upgrade. See [`inshall.sh`](inshall.sh)
for exactly what runs before you pipe it into `sudo bash`.

## What debforge does with root

debforge is designed to run under `sudo`. It performs system-level operations
that require root:
- Runs `apt-get` and `dpkg` to install, remove, and query system packages.
- Writes configuration files under `/etc/` and user home directories.
- Installs software to `/usr/local/bin/`.
- Manages its own installation at `/opt/debforge/`.
- Reads and writes package state to a JSON file under `/opt/debforge/`.

This is not a sandboxed or rootless tool — it assumes full system trust.

## Supported distributions

debforge targets Debian Trixie (13). The `setup`
command's `sources.list`, the default `trixie-backports` suite, and the
packages in `repo/packages/` all assume Trixie. Other Debian-based releases
aren't a design goal — the code will build with Go 1.21+ elsewhere, but
nothing is tested or maintained against other releases.

## Usage

```
debforge - package manager

Usage: debforge [flags] <command> [<name>...]

Flags:
    -y, --yes               Skip confirmation prompts
    -f, --force             Force operation (reinstall)
    -a, --all               Update all packages (update only)

Commands:
    install <name>...       Install packages
    remove <name>...        Remove packages from system
    update [<name>...]      Reinstall packages (runs apt-get update)
        --all               Update all packages and run apt-get upgrade
    setup                   Provision system (repos, firmware, desktop)
        --force             Skip checks, reapply all steps
    doctor                  Check system health
    list                    List available categories
    list @<category>        List packages in a category
    list --packages         List packages grouped by category
    search [<pattern>]      Search packages by name or description
    diff [<path>...]        Show config diff vs sidecar
    info <name>...          Show detailed package information
        -v, --verbose       Show full config and script contents
    update --self           Update debforge itself
    remove --self           Remove debforge from system
    --help                  Show this help
    --version               Show version
```

Removing a package that other installed packages depend on prints what will
also be removed before the confirmation prompt.

### `setup`

Provisions a fresh Debian install: repositories, i386 multiarch, a full
upgrade, firmware, core devtools, kernel packages, zram, `systemd-resolved`/
`timesyncd` tuning, extrepo, Mesa, multimedia codecs, fonts, and desktop
packages. Each step checks current system state first and only changes what's
missing or out of date — run it again any time to catch up on new defaults.
`--force` skips those checks and reapplies every step unconditionally,
including overwriting `/etc/apt/sources.list` regardless of its current
contents.

Packages that require an NVIDIA GPU (`nvidia`, and anything depending on it)
check for one via `lspci` before installing and fail cleanly if none is
found.

### `doctor`

Read-only system health check. Runs every setup step's check and reports the
status of each — satisfied (green), not configured (blue), drifted or conflict
(yellow), or error (red). Exits 0 when everything is ready, 1 otherwise.

## Config file conflicts

For every config file debforge writes, it records a hash of the content at
write time. On the next install or update:

- If the file on disk still matches what debforge last wrote, and the
  package's content changed, the file is updated.
- If you edited the file and the package's content for it didn't change,
  your edit is left alone.
- If both changed, your file is left untouched and the new version is
   written next to it as `<file>.debforge-new` — review and merge manually.
   Run `debforge diff` to show all pending sidecar diffs, or
   `debforge diff <file>` for a specific path.

## Package schema

Packages are YAML files under `repo/packages/<type>/`. Each file defines one
package. Full real-world examples are in `repo/packages/`.

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

### deb — `.deb` packages downloaded from a URL

```yaml
name: vscodium
description: VS Code without Microsoft branding
type: deb
package: codium    # primary system package name (for installed checks)
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

Repos with no tagged releases can omit version fields entirely — debforge
falls back to tracking the latest commit on the default branch.

### config — static configuration files

```yaml
name: config-system
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
