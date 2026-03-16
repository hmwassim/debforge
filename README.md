# DebForge

System optimization configurations for Debian Trixie. Provides system tweaks for improved performance, especially for gaming and desktop use.

## Features

- **No sudo prefix needed**: Scripts handle privilege escalation internally
- **User-level state**: Manifest, logs, and backups stored in `~/.local/share/debforge/`
- **Two-stage installation**: Setup scripts → Config deployment
- **Idempotent**: Safe to run multiple times
- **Full Rollback Support**: Clean uninstall with backup restoration
- **Dry-Run Mode**: Preview changes before applying

## Quick Start

### Option 1: Bootstrap Installer (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/hmwassim/debforge/main/scripts/bootstrap.sh | bash
debforge install
```

### Option 2: From Git Repository

```bash
git clone https://github.com/hmwassim/debforge.git
cd debforge
./scripts/install.sh
```

## Usage

### CLI Commands

```bash
# Installation
debforge install                           # Full installation
debforge install --skip-scripts            # Configs only
debforge install --kernel backports        # With backports kernel
debforge install --dry-run                 # Preview changes

# Status
debforge status                            # Show status
debforge status --verify                   # Verify installation

# Uninstall
debforge uninstall                         # Interactive uninstall
debforge uninstall --full                  # Remove everything

# Update
debforge update                            # Update to latest version
```

### Script Options

```bash
# Kernel choices
--kernel backports    # Debian backported kernel (stable)
--kernel liquorix     # Liquorix kernel (performance-tuned)

# NVIDIA driver choices
--nvidia nvidia-open  # Open kernel modules (RTX 3060+)
--nvidia cuda-drivers # Proprietary drivers (full CUDA stack)
```

## What Gets Installed

### Setup Scripts (Stage 1)

| Directory | Purpose |
|-----------|---------|
| `scripts/01-core/` | Base system, fonts, kernel |
| `scripts/02-hardware/` | Mesa, NVIDIA drivers |
| `scripts/03-desktop/` | Audio, shell, KDE |
| `scripts/04-gaming/` | Gaming tools, launchers |
| `scripts/05-misc/` | GitHub Desktop, VSCodium |

### Config Files (Stage 2)

| Category | Location |
|----------|----------|
| Sysctl | `/etc/sysctl.d/` |
| Udev rules | `/usr/lib/udev/rules.d/` |
| Modprobe | `/usr/lib/modprobe.d/` |
| Systemd | `/usr/lib/systemd/` |
| Home configs | `~/.config/` (WirePlumber, PipeWire, KWin, Baloo) |

### Binaries

| Binary | Purpose |
|--------|---------|
| `ksmctl` | Kernel Same-page Merging control |
| `pci-latency` | PCI latency timer adjustment |
| `game-performance` | Game performance wrapper |

## Project Structure

```
debforge/
├── bin/                    # Executables (ksmctl, pci-latency, game-performance)
├── configs/                # Config files deployed to system
│   ├── home/              # User configs (wireplumber, pipewire, kwinrc)
│   ├── sysctl.d/          # Kernel parameters
│   ├── udev/              # Device rules
│   ├── systemd/           # Systemd configs
│   └── ...
├── scripts/
│   ├── install.sh         # Main installer
│   ├── uninstall.sh       # Uninstaller
│   ├── status.sh          # Status viewer
│   ├── bootstrap.sh       # GitHub downloader
│   ├── debforge           # CLI wrapper
│   ├── 01-core/           # Setup scripts
│   ├── 02-hardware/
│   ├── 03-desktop/
│   ├── 04-gaming/
│   ├── 05-misc/
│   └── lib/               # Shared libraries
└── README.md
```

## State Directory

```
~/.local/share/debforge/
├── manifest.json          # Tracks installed files
├── backups/               # Backups of replaced configs
└── logs/                  # Install/uninstall logs
```

## Common Workflows

### Fresh Installation

```bash
debforge install
debforge status --verify
sudo reboot
```

### Configs Only (Re-apply)

```bash
debforge install --skip-scripts
```

### Development Iteration

```bash
# Edit config
vim configs/sysctl.d/99-debforge.conf

# Preview and apply
debforge install --skip-scripts --dry-run
debforge install --skip-scripts
```

## Safety Features

- **Backups**: Existing files backed up before modification
- **Pre-flight checks**: Verifies Debian Trixie, required tools, disk space
- **Rollback**: Failed installations restore previous state

## Troubleshooting

```bash
# View logs
cat ~/.local/share/debforge/logs/install-*.log | tail -100

# Force reinstall configs
debforge install --force --skip-scripts

# Manual cleanup
rm -rf ~/.local/share/debforge/
debforge install --force --skip-scripts
```

## Requirements

- Debian 13 (Trixie)
- sudo privileges

## License

MIT

## Contributing

1. Make changes to `configs/` or scripts
2. Test with `--dry-run`
3. Verify with `status.sh --verify`
4. Test uninstall for clean rollback
