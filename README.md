# DebForge

System optimization configurations for Debian Trixie. This project provides a robust, reliable, and informative way to apply system tweaks for improved performance, especially for gaming and desktop use.

## Features

- **No sudo prefix needed**: Scripts handle privilege escalation internally
- **User-level state**: Manifest, logs, and backups stored in your home directory
- **Two-stage installation**: Setup scripts → Config deployment
- **Idempotent**: Safe to run multiple times
- **Full Rollback Support**: Clean uninstall with backup restoration
- **JSON Manifest**: Track every installed file with checksums and metadata
- **Detailed Logging**: All operations logged to `~/.local/share/debforge/logs/`
- **Dry-Run Mode**: Preview changes before applying
- **Home Config Support**: Tracks and manages ~/.config files (KDE, PipeWire, etc.)

## Quick Start

### Option 1: Bootstrap Installer (Recommended)

```bash
# Download and install DebForge to /opt/debforge
curl -fsSL https://raw.githubusercontent.com/hmwassim/debforge/main/scripts/bootstrap.sh | bash

# Then run the installer
debforge install
```

### Option 2: From Git Repository

```bash
# Clone the repository
git clone https://github.com/hmwassim/debforge.git
cd debforge

# Full installation (setup scripts + configs)
./scripts/install.sh

# Check status (runs as normal user)
./scripts/status.sh

# Uninstall configs (rollback)
./scripts/uninstall.sh
```

## Installation Flow

```
┌─────────────────────────────────────────────────────────────┐
│  debforge install  (or ./scripts/install.sh)                │
├─────────────────────────────────────────────────────────────┤
│  Stage 1: Setup Scripts (01-core → 05-misc)                 │
│  ├─ Install packages (firmware, tools, kernels)             │
│  ├─ Configure system (DNS, repositories, users)             │
│  └─ Interactive choices (kernel selection, etc.)            │
├─────────────────────────────────────────────────────────────┤
│  Stage 2: System Config Deployment                          │
│  ├─ Sysctl, udev, modprobe, tmpfiles                        │
│  ├─ Systemd services and configs                            │
│  └─ Binaries (ksmctl, pci-latency, game-performance)        │
├─────────────────────────────────────────────────────────────┤
│  Stage 3: Home Config Deployment                            │
│  ├─ WirePlumber audio configs                               │
│  ├─ PipeWire configs                                        │
│  ├─ KWin compositor settings                                │
│  └─ Baloo file indexing config                              │
├─────────────────────────────────────────────────────────────┤
│  Stage 4: Apply and Verify                                  │
│  └─ Reload services, apply sysctl, verify installation      │
└─────────────────────────────────────────────────────────────┘
```

## Usage

### Using the `debforge` CLI (Recommended)

After bootstrap installation, use the `debforge` command:

```bash
# Full installation (interactive)
debforge install

# Non-interactive: specify kernel and NVIDIA driver
debforge install --kernel backports --nvidia nvidia-open

# Configs only (skip setup scripts)
debforge install --skip-scripts

# Preview changes (dry-run)
debforge install --dry-run

# Check status
debforge status

# Verify installation
debforge verify

# Uninstall (interactive prompt)
debforge uninstall

# Uninstall modes
debforge uninstall --debforge-only    # Keep all configs
debforge uninstall --system           # Remove system configs (default)
debforge uninstall --full             # Remove everything

# Update to latest version
debforge update
```

### Using Scripts Directly (Git Clone)

If you cloned the repository, use scripts directly:

#### Installation

```bash
# Full installation (interactive prompts for kernel, drivers, etc.)
./scripts/install.sh

# Non-interactive: specify kernel and NVIDIA driver
./scripts/install.sh --kernel backports --nvidia nvidia-open

# Configs only (skip setup scripts)
./scripts/install.sh --skip-scripts

# Scripts only (skip configs, run again later for configs)
./scripts/install.sh --scripts-only

# Preview changes (dry-run)
./scripts/install.sh --dry-run

# Force re-installation of configs
./scripts/install.sh --force

# Verbose output
./scripts/install.sh --verbose
```

#### Kernel & Driver Options

```bash
# Kernel choices
--kernel backports    # Debian backported kernel (stable)
--kernel liquorix     # Liquorix kernel (performance-tuned)

# NVIDIA driver choices
--nvidia nvidia-open  # Open kernel modules (RTX 3060+)
--nvidia cuda-drivers # Proprietary drivers (full CUDA stack)
```

#### Uninstallation

```bash
# Interactive uninstall (prompts for mode)
./scripts/uninstall.sh

# Preview changes (dry-run)
./scripts/uninstall.sh --dry-run

# Keep backup files after uninstall
./scripts/uninstall.sh --keep-backups

# Remove DebForge only (keep all configs)
./scripts/uninstall.sh --debforge-only

# Remove DebForge + system configs
./scripts/uninstall.sh --system

# Full cleanup (remove everything including ~/.config)
./scripts/uninstall.sh --full
```

#### Status & Verification

```bash
# Show installation status
./scripts/status.sh

# Verify installed files (checksums, permissions)
./scripts/status.sh --verify

# JSON output (for scripting)
./scripts/status.sh --json
```

## Script Organization

### Setup Scripts (Run First)

| Directory | Purpose | Example Scripts |
|-----------|---------|-----------------|
| `01-core/` | Base system setup | `01.01-system.sh`, `01.03-kernel.sh` |
| `02-hardware/` | Hardware-specific | `02.01-mesa.sh`, `02.02-nvidia.sh` |
| `03-desktop/` | Desktop environment | `03.01-audio.sh`, `03.03-kde.sh` |
| `04-gaming/` | Gaming tools | `04.01-gaming-stack.sh`, `04.02-tricks.sh` |
| `05-misc/` | Additional tools | GitHub Desktop, etc. |

### Config Files (Deployed Second)

| Category | Files | Location |
|----------|-------|----------|
| Sysctl | `99-debforge.conf` | `/etc/sysctl.d/` |
| Udev Rules | `*.rules` (8 files) | `/usr/lib/udev/rules.d/` |
| Modprobe | `*.conf` | `/usr/lib/modprobe.d/` |
| Tmpfiles | `*.conf` | `/usr/lib/tmpfiles.d/` |
| Systemd | `*.conf` | `/usr/lib/systemd/` |
| Security Limits | `20-audio.conf` | `/etc/security/limits.d/` |
| **Home Configs** | `wireplumber/`, `pipewire/`, `kwinrc` | `~/.config/` |

### Home Configs (User-Level)

| Config | Purpose | Location |
|--------|---------|----------|
| WirePlumber | Audio device suspend, HDMI deprioritization | `~/.config/wireplumber/` |
| PipeWire | Clock and Pulse compatibility tuning | `~/.config/pipewire/` |
| KWin | KDE compositor settings, animations | `~/.config/kwinrc` |
| Baloo | KDE file indexing disabled | `~/.config/baloofilerc` |

### Binaries

| Binary | Purpose |
|--------|---------|
| `ksmctl` | Kernel Same-page Merging control |
| `pci-latency` | PCI latency timer adjustment |
| `game-performance` | Game performance wrapper |

## Project Structure

```
debforge/
├── bin/                        # Executable binaries
│   ├── ksmctl
│   ├── pci-latency
│   └── game-performance
├── configs/                    # Configuration files (deployed by install.sh)
│   ├── sysctl.d/              # Sysctl tuning
│   ├── udev/                  # Udev rules
│   ├── modprobe.d/            # Modprobe configs
│   ├── systemd/               # Systemd configs
│   ├── tmpfiles.d/            # Tmpfiles configs
│   ├── security/              # Security limits
│   ├── NetworkManager/        # NetworkManager configs
│   ├── earlyoom               # EarlyOOM config
│   └── home/                  # Home directory configs
│       ├── wireplumber/       # WirePlumber audio configs
│       ├── pipewire/          # PipeWire configs
│       ├── kwinrc             # KWin compositor settings
│       └── baloofilerc        # Baloo file indexing
├── scripts/                    # Management scripts
│   ├── install.sh             # Main installer
│   ├── uninstall.sh           # Uninstaller (3-tier)
│   ├── status.sh              # Status viewer
│   ├── bootstrap.sh           # GitHub downloader
│   ├── debforge               # CLI wrapper
│   ├── 01-core/               # Setup scripts
│   ├── 02-hardware/
│   ├── 03-desktop/
│   ├── 04-gaming/
│   ├── 05-misc/
│   └── lib/                   # Shared libraries
│       ├── logger.sh
│       ├── manifest.sh
│       ├── backup.sh
│       └── verify.sh
└── README.md
```

## State Directory

All runtime state is stored in your home directory (no root access needed for state):

```
~/.local/share/debforge/
├── manifest.json          # Tracks all installed config files
├── installed              # Marker file indicating installation
├── backups/               # Backups of replaced config files
│   └── _etc_sysctl.d_99-debforge.conf.bak
└── logs/
    ├── install-20260315-103000.log
    └── uninstall-20260315-104500.log
```

## Common Workflows

### Fresh Installation

```bash
# 1. Run full installation (scripts + configs)
./scripts/install.sh

# 2. Check status
./scripts/status.sh --verify

# 3. Reboot to apply all changes
sudo reboot
```

### Configs Only (After System Setup)

```bash
# Skip setup scripts, only deploy configs
./scripts/install.sh --skip-scripts

# Useful for: re-applying configs, testing config changes
```

### Iterative Development

```bash
# 1. Modify a config file
vim configs/sysctl.d/99-debforge.conf

# 2. Preview changes
./scripts/install.sh --skip-scripts --dry-run

# 3. Apply configs only
./scripts/install.sh --skip-scripts

# 4. Verify
./scripts/status.sh --verify
```

## Safety Features

### Backups
Before modifying any existing config file, a backup is created in `~/.local/share/debforge/backups/`. During uninstall, these backups are restored.

### Pre-flight Checks
Before installation, the script verifies:
- NOT running as root (scripts use sudo internally)
- Debian Trixie OS
- Required tools available (jq, systemctl, udevadm, sudo)
- Sufficient disk space (100MB minimum)

### Rollback on Failure
If config installation fails at any phase, changes are automatically rolled back:
- Files with backups are restored
- Files without backups are removed
- Services are disabled

## Troubleshooting

### View Logs
```bash
# Latest install log
cat ~/.local/share/debforge/logs/install-*.log | tail -100

# Latest uninstall log
cat ~/.local/share/debforge/logs/uninstall-*.log | tail -100
```

### Check Status
```bash
# Detailed status with verification
./scripts/status.sh --verify --verbose
```

### Force Reinstall Configs
```bash
# Re-apply all configurations
./scripts/install.sh --force --skip-scripts
```

### Manual Cleanup
If the manifest is lost:
```bash
# Remove state files
rm -rf ~/.local/share/debforge/

# Reinstall configs
./scripts/install.sh --force --skip-scripts
```

## Requirements

- Debian 13 (Trixie)
- sudo privileges (used internally by scripts)
- Required packages: `jq`, `systemd`, `udev` (installed by setup scripts)

## License

Configuration files sourced from CachyOS (Vasiliy Stelmachenok), GPLv2+

## Contributing

1. Make changes to configuration files in `configs/` or scripts in `01-*` to `05-*`
2. Test with `--dry-run` first
3. Verify with `status.sh --verify`
4. Test uninstall to ensure clean rollback
