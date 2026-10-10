# debforge

An opinionated provisioner and small package manager for **Debian 13 (trixie), amd64**.

- `debforge setup` turns a fresh Debian install into a ready desktop: repositories
  (contrib, non-free, backports), i386 multiarch, a backports kernel, firmware and Mesa,
  zram, encrypted DNS, time sync, codecs, fonts and Flatpak.
- `debforge install <pkg>` installs software that Debian doesn't package well: apt
  packages from extrepo or vendor repositories, upstream `.deb` releases, AppImages,
  source builds and configuration bundles. Each one is described by a small YAML file.

Every command shows a plan before it changes anything. debforge records exactly what it
installed, so removing a package undoes that and nothing else.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/hmwassim/debforge/main/install.sh | sudo sh
```

`install.sh` downloads the latest release binary and checks it against the release's
`SHA256SUMS`. It installs the binary to `/usr/local/bin/debforge` and adds shell
completions. **It changes nothing else.** Run `sudo debforge setup` afterwards to review and
apply the system setup.

## Usage

```
debforge <command> [flags] [args]

  install <pkg>...     install packages (and their dependencies)
  remove <pkg>...      remove packages, what depends on them, and dependencies
                       nothing else needs
  update <pkg>...      update packages
  update --all         apt update + full-upgrade, then update every debforge package
  update --self        update debforge itself
  setup                provision the system from the setup profile
  doctor               check the system against the setup profile (no root needed)
  list [@category]     list packages (--installed, --names)
  search <term>...     search names and descriptions
  info <pkg>...        show a package (-v shows file contents and hooks)
  diff [<path>...]     show pending .debforge-new config updates
  sync                 find packages removed behind debforge's back
  completion <shell>   print a bash/zsh/fish completion script
  remove --self        uninstall debforge (--all also removes its packages)
```

Packages can be named directly, by glob (`'fonts-nerd-*'`) or by category (`@gaming`).

Common flags:
- `-y`: answer yes to confirmations.
- `-n` / `--dry-run`: print the plan and stop.
- `-f` / `--force`: reinstall even when up to date, and replace modified config files.
  The replaced files are backed up.
- `--variant pkg=name`: choose a package variant, e.g. `--variant nvidia=open`.

A package that has variants needs a choice. debforge asks interactively; with `-y` you must
pass `--variant`, because debforge never picks one silently.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | success, or you declined the plan |
| 1 | error |
| 2 | usage error |
| 130 | interrupted |

## How it behaves

- **Plan, confirm, execute.** Nothing changes before you confirm. Downloads happen before
  any package change, so a failed download leaves the system untouched.
- **One apt transaction.** apt packages, local `.deb` files and the removal of conflicting
  packages go into a single `apt-get install`. Backports are installed in a second
  transaction with `-t trixie-backports`.
- **apt never asks questions.** apt runs non-interactively:
  - `DEBIAN_FRONTEND=noninteractive`;
  - conffiles keep your local version (`--force-confold`);
  - debconf answers such as EULAs are preseeded from the package definition;
  - it waits up to 5 minutes for the dpkg lock.

  Progress comes from apt's machine-readable status channel, not from parsing its text
  output.
- **Backports never get mixed with stable.** When any package from a source package is
  installed from backports, debforge pins that whole source to backports for every
  architecture, in `/etc/apt/preferences.d/debforge-backports.pref`. The file is rewritten
  before every install. Without it, a later stable install can pick a stable 32-bit library
  next to its backports sibling and fail with "held broken packages".
- **Ctrl-C is safe.** A running apt/dpkg transaction is always allowed to finish. debforge
  then records what was installed and stops. A package interrupted after its apt step is
  marked *incomplete*, and `debforge update <pkg>` finishes it.
- **Exact removal.** Each package has a manifest of the apt packages, `.deb`s, extrepos
  and files it added. Removing it reverses that manifest:
  - an apt package that was already installed before debforge, or that another debforge
    package still needs, is kept;
  - a file you edited is kept;
  - a file debforge replaced is restored from its backup.
- **One lock.** All commands that change the system take one lock. A second debforge
  process waits, and says so.
- **Read-only commands need no root.** That covers `list`, `search`, `info`, `diff` and
  `doctor`.

## Config files: 3-way merge

debforge records a hash of every file it writes. On the next install, update or setup it
compares the file on disk, that recorded hash, and the new content:

| File on disk | Action |
|---|---|
| absent | written |
| unchanged since debforge wrote it | updated to the new content |
| edited by you, and the package's content is unchanged | kept |
| edited by you, and the package's content changed too | kept; the new version is written to `<file>.debforge-new` (see `debforge diff`) |
| existed before debforge and differs | moved to `<file>.debforge-orig`, then written; restored when the package is removed |

Writes are atomic (temporary file, fsync, rename). Files in your home directory are
written without following symlinks, and are owned by you. That includes any directories
debforge creates.

## Package definitions

Definitions are built into the binary, so they always match the code. To add or override
packages locally, put YAML files in `/etc/debforge/packages.d/`. Their source files go in
`/etc/debforge/packages.d/files/<package>/`.

Definitions are decoded strictly:
- an unknown or misplaced key is an error;
- everything is validated when the catalog loads.

```yaml
name: lutris                   # [a-z0-9+.-]
description: Lutris open gaming platform
category: gaming               # browsers communication desktop development fonts gaming media system utils
depends: [wine]                # other debforge packages
requires: [python3-gi]         # apt prerequisites (same meaning for every kind)
hardware: {pci_vendor: "10de"} # optional: only install when this PCI vendor is present

source:                        # at most one; none = a configuration bundle
  apt:
    packages: [firefox]
    extrepo: [mozilla]                      # enable via extrepo
    backports: [gamescope]                  # install with -t trixie-backports
    conflicts: [firefox-esr]                # removed in the same transaction
    variants: {open: [nvidia-open], proprietary: [cuda-drivers]}
    repo:                                   # a vendor repo not in extrepo
      uri: https://downloads.cursor.com/aptrepo
      suite: stable
      components: [main]
      key: https://downloads.cursor.com/keys/anysphere.asc
  deb:
    urls: [{url: "https://.../lutris_{version}_all.deb", sha256: ...}]
  archive: {url: "https://.../x-{version}.tar.gz", sha256: ..., strip: 1}
  git: {repo: "https://github.com/...", ref: "v{version}"}
  appimage:
    url: "https://.../App-{version}.AppImage"
    bin: app                                # /usr/local/bin/app
    desktop: {name: App, categories: "Utility;", icon: "https://.../icon.png"}

version:                       # required when a URL or ref uses {version}
  from: git-tags               # git-tags | cmd | pin
  repo: https://github.com/lutris/lutris
  tag: "v{version}"            # explicit pattern, e.g. "release-{version}" or "{version}"
  prerelease: false            # rc/beta/alpha tags are skipped unless true
  # cmd: "<shell command printing the version>"   (from: cmd)
  # pin: "1.2.3"                                  (from: pin)
trust: upstream-tls            # allows auto-updating downloads without sha256 (HTTPS only)

files:                         # tracked, merged, and removed with the package
  - {dest: /etc/modules-load.d/ntsync.conf, content: "ntsync\n"}
  - {dest: "~/.config/app/app.conf", src: app.conf, mode: "0600"}

hooks:                         # run with sh -eu; env: VERSION, TARGET_USER, TARGET_HOME, PKGNAME
  build: make
  install: make install DESTDIR="$DESTDIR"   # archive/git: install into $DESTDIR only
  post_install: systemctl enable --now app.service
  pre_remove: ...
  post_remove: ...

notes:                         # shown after install / removal
  install: log out and back in to apply.
  remove: reverts after a reboot.
debconf: ["pkg question type value"]   # preseeded before the apt transaction
reload: [udev, sysctl, systemd, tmpfiles, fontconfig, desktop, modules]
legacy_cleanup: [/etc/old-name.conf]   # deleted on install
```

### Rules

- **File locations.** Files may only go under `/etc/`, `/usr/local/`, `/opt/` or `~/`.
  debforge never writes into dpkg-owned trees such as `/usr/lib`.
- **Source builds** (`archive`, `git`) must install into `$DESTDIR`. debforge copies the
  staged files into place and records each one, so removing the package is as exact as
  removing a `.deb`.
- **Downloads** must be HTTPS. Redirects to plain HTTP are refused.
  - A download needs a `sha256`.
  - The exception is a package that auto-updates (`version.from` is `git-tags` or `cmd`)
    and explicitly declares `trust: upstream-tls`.
  - A `sha256` cannot be combined with an auto-updating `{version}` URL: use
    `version.from: pin` instead.
- **Versions** are checked before they are substituted into URLs or passed to hooks.
- **Matrices.** One file can define several similar packages with a `matrix:` list. Each
  entry's keys replace `{{key}}` placeholders; see `catalog/packages/fonts-nerd.yaml`.

## Setup profile

The steps are data, in `catalog/profiles/default.yaml`. Each step can:
- install packages (optionally from backports);
- write tracked files;
- manage services;
- run check/apply commands;
- verify its result.

A step can be limited to matching machines with `when:`:
- `cpu_vendor`: microcode for Intel or AMD;
- `network_manager`: whether NetworkManager is installed;
- `desktop`: KDE or GNOME, detected from installed packages because sudo strips
  `XDG_CURRENT_DESKTOP`.

`debforge setup` checks every step, shows what is missing, and asks before applying it.
`debforge doctor` runs the same checks without changing anything. Checks verify everything
a step does. For example, the backports kernel and firmware steps are only satisfied when
backports has nothing newer.

Notable defaults:
- **apt sources:** in `/etc/apt/sources.list.d/debian.sources` (deb822). A legacy
  `sources.list` that still has active entries is backed up and replaced with a comment.
- **DNS:** systemd-resolved with Cloudflare's malware-blocking resolver. It uses
  `DNSOverTLS=opportunistic` and `DNSSEC=allow-downgrade`, so captive portals, networks
  that block port 853, and VPN split DNS keep working.
- **Backports:** the kernel, firmware and Mesa come from backports. PipeWire stays on
  stable because backports has no matching i386 build.
- **Microcode:** chosen by CPU vendor.

## Paths

| Path | Purpose |
|---|---|
| `/usr/local/bin/debforge` | binary |
| `/var/lib/debforge/state.json` | installed packages and their manifests (schema 2) |
| `/var/log/debforge/` | daily logs (30 kept), including every command run |
| `/var/cache/debforge/work/` | downloads and builds (on disk, not tmpfs) |
| `/etc/debforge/packages.d/` | your own definitions |
| `/etc/apt/preferences.d/debforge-backports.pref` | backports source pins (generated) |

A corrupt state file is moved aside (`state.json.corrupt-<time>`) and debforge starts
empty. It warns you, and keeps working.

## Development

```sh
make build      # bin/debforge
make test       # unit tests (no test can reach the real apt: unknown commands fail)
make race       # with the race detector
make live       # resolve every upstream version and check every apt name (network)
make itest      # end-to-end scenarios in a disposable debian:trixie container (docker)
make release    # dist/: debforge-linux-amd64, VERSION, SHA256SUMS
```

`ITEST_SLOW=1 make itest` also installs `ttf-mscorefonts-installer` to prove that debconf
prompts never hang.
