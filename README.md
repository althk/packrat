# 🐀 Packrat

**Because you never know when you'll need that `.bashrc` from 3 weeks ago.**

Packrat is a CLI tool and background daemon that automatically backs up your shell history, dotfiles, config directories, and arbitrary paths to remote storage via [rclone](https://rclone.org). It uses git-style content-addressable versioning, encrypts sensitive data at rest with [age](https://age-encryption.org/), and provides a TUI for easy restore — especially when setting up a new machine.

---

## Features

- **Shell history backup** — auto-detects bash, zsh, and fish; merges and deduplicates across machines
- **Dotfiles management** — tracks `.bashrc`, `.zshrc`, `.gitconfig`, `.ssh/config`, and more out of the box
- **Config directory backup** — back up `~/.config/nvim/`, VS Code settings, AI tool configs, etc.
- **Custom paths** — back up anything you want, organized into groups with independent schedules
- **Git-style versioning** — incremental snapshots with SHA-256 content-addressable deduplication
- **Encryption at rest** — age encryption (AES-256-GCM) with OS keyring, file, or passphrase-based key management
- **Background daemon** — built-in cron scheduler with quiet hours, overdue backup detection, and graceful shutdown
- **TUI restore interface** — browse snapshots, view file changes, and restore interactively
- **Non-interactive restore** — script-friendly CLI flags for automation and new machine bootstrap
- **Pre/post backup hooks** — run arbitrary commands (e.g., dump package lists) before or after backups
- **Integrity verification** — SHA-256 checksums for every file in every snapshot
- **Garbage collection** — retention-based cleanup of old snapshots and orphaned blobs

## Quick Start

### Prerequisites

- **Go 1.22+** (for building from source)
- **[rclone](https://rclone.org/install/)** — configured with at least one remote (Google Drive, S3, Dropbox, etc.)

### Install

```bash
# From source
go install github.com/harish/packrat/cmd/packrat@latest

# Or build locally
git clone https://github.com/harish/packrat.git
cd packrat
make build
sudo cp bin/packrat /usr/local/bin/
```

### Setup

```bash
# Interactive setup wizard
packrat init

# This will:
# 1. Ask for a machine name
# 2. Detect and validate your rclone remote
# 3. Generate encryption keys (stored in OS keyring)
# 4. Auto-configure backup groups based on your shell
# 5. Optionally start the background daemon
```

### Daily Usage

```bash
# Run a backup now
packrat backup

# Check what's changed since last backup
packrat diff

# See backup status
packrat status

# Start the background daemon (backs up on schedule)
packrat daemon start
```

## Commands

| Command | Description |
|---|---|
| `packrat init` | First-time setup wizard |
| `packrat backup` | Run backup now (all groups or `--group <name>`) |
| `packrat backup --dry-run` | Show what would be backed up |
| `packrat restore` | Launch TUI restore interface |
| `packrat restore --list` | List available snapshots |
| `packrat restore --latest --group dotfiles` | Restore latest snapshot for a group |
| `packrat restore --snapshot <id>` | Restore a specific snapshot |
| `packrat restore --file <path>` | Restore a single file |
| `packrat status` | Show daemon status and last backup times |
| `packrat diff` | Diff current files vs last snapshot |
| `packrat diff <snap1> <snap2>` | Diff between two snapshots |
| `packrat verify` | Verify file integrity against last snapshot |
| `packrat daemon start` | Start the background scheduler |
| `packrat daemon stop` | Stop the background scheduler |
| `packrat daemon status` | Check if daemon is running |
| `packrat log` | Show recent log entries |
| `packrat config show` | Print resolved config |
| `packrat config edit` | Open config in `$EDITOR` |
| `packrat config validate` | Check config for errors |
| `packrat config add-path <path>` | Quick-add a path to backup |
| `packrat history` | Show backup run history |
| `packrat gc` | Run garbage collection on old snapshots |
| `packrat rotate-key` | Generate new encryption key and re-encrypt blobs |
| `packrat nuke --local` | Delete all local packrat data |
| `packrat nuke --remote` | Delete all remote packrat data |
| `packrat version` | Print version info |

### Global Flags

```
--config <path>    Override config file location
--verbose / -v     Enable debug logging
--quiet / -q       Suppress all output except errors
--no-color         Disable colored output
```

## Configuration

Config lives at `~/.config/packrat/config.toml`. Here's an overview of the structure:

```toml
[general]
machine_name = "my-workstation"
machine_id = "a1b2c3d4"            # Auto-generated, do not edit
log_level = "info"                  # debug | info | warn | error

[scheduler]
enabled = true
default_interval = "1h"
quiet_hours_start = "23:00"        # Optional
quiet_hours_end = "06:00"

[storage]
backend = "rclone"
rclone_remote = "gdrive"           # Your rclone remote name
remote_base_path = "packrat-backups"
bandwidth_limit = ""               # e.g., "1M" for 1MB/s

[encryption]
enabled = true
key_source = "keyring"             # "keyring" | "file" | "prompt"

[versioning]
retention_count = 50               # Keep last N snapshots per group
retention_days = 30                # Keep snapshots from last N days
# Both rules are OR'd — a snapshot is kept if it matches either

# Backup groups — add as many [[backup]] blocks as you want
[[backup]]
name = "dotfiles"
paths = ["~/.bashrc", "~/.zshrc", "~/.gitconfig"]
encrypt = false
interval = "1h"
exclude = []

[[backup]]
name = "ai-configs"
paths = ["~/.claude/", "~/.config/github-copilot/"]
encrypt = true                     # Encrypted before upload
interval = "2h"
exclude = ["*.log", "*.cache"]

# Hooks — run commands before/after backup
[[hook]]
name = "dump-packages"
when = "pre-backup"                # "pre-backup" | "post-backup"
command = "dpkg --get-selections > ~/.config/packrat/packages.txt"
timeout = "30s"
fail_action = "continue"           # "continue" | "abort"
```

See [configs/default.toml](configs/default.toml) for a complete example.

## New Machine Bootstrap

The killer feature — set up a new machine from your Packrat backups:

```bash
# On the new machine (after installing packrat + rclone):
packrat init --restore
```

This walks you through selecting a remote, choosing which machine's backups to restore from, and pulling down your dotfiles, configs, and history.

## How It Works

### Backup Flow

1. Acquire file lock (prevents concurrent runs)
2. Run pre-backup hooks
3. For each backup group (in parallel):
   - Walk file trees, compute SHA-256 hashes
   - Compare against last snapshot to find changes
   - Upload new/changed files as content-addressable blobs
   - Encrypt blobs if the group has `encrypt = true`
   - Create and upload a snapshot manifest (JSON)
4. Record results in local SQLite database
5. Run post-backup hooks
6. Release lock

### Content-Addressable Storage

Files are stored by their SHA-256 hash in a two-level directory structure:

```
packrat-backups/<machine-id>/
├── manifests/<group>/<snapshot-id>.json
├── blobs/ab/c123def456...        # First 2 chars / rest of hash
└── meta/machine-info.json
```

Identical files (even across groups) are stored only once — deduplication is automatic.

### Encryption

Packrat uses [age](https://age-encryption.org/) for encryption. Keys can be stored in:

- **OS keyring** (default) — most convenient, keys survive reboots
- **File** — you manage the key file
- **Passphrase** — derived via scrypt, prompted each time

Encryption is per-group: sensitive configs can be encrypted while dotfiles stay plain.

## Architecture

```
┌────────────────────────────────────────────────┐
│                   CLI Layer                     │
│  packrat init | backup | restore | daemon | ... │
└──────────────────────┬─────────────────────────┘
                       │
┌──────────────────────▼─────────────────────────┐
│                  Core Engine                    │
│  Scheduler · Differ · Encryptor · Snapshotter  │
└──────────────────────┬─────────────────────────┘
                       │
┌──────────────────────▼─────────────────────────┐
│             Storage Abstraction                 │
│         StorageBackend interface                │
└──────────────────────┬─────────────────────────┘
                       │
┌──────────────────────▼─────────────────────────┐
│               Rclone Adapter                   │
│    Wraps rclone CLI (supports 70+ backends)    │
└────────────────────────────────────────────────┘
```

### Project Structure

```
packrat/
├── cmd/packrat/          # CLI entry point and commands
├── internal/
│   ├── backup/           # Engine, differ, snapshots, SQLite state
│   ├── config/           # TOML config loading, validation, defaults
│   ├── crypto/           # age encryption, keyring integration
│   ├── hooks/            # Pre/post backup hook execution
│   ├── platform/         # Paths, errors, logging
│   ├── restore/          # Restore logic, conflict detection
│   ├── scheduler/        # Cron scheduler, daemon lifecycle
│   ├── shell/            # Shell history parsing, dotfile discovery
│   ├── storage/          # StorageBackend interface + implementations
│   └── tui/              # Bubbletea restore interface
├── configs/              # Example config
├── scripts/              # Install script, shell completions
├── Makefile
└── .goreleaser.yaml
```

## Contributing

### Development Setup

```bash
git clone https://github.com/harish/packrat.git
cd packrat
make build        # Build the binary
make test         # Run unit tests
make test-integration   # Run integration tests (needs rclone)
make lint         # Run linter (needs golangci-lint)
```

### Running Tests

```bash
# Unit tests (no external dependencies)
make test

# Integration tests (uses local filesystem backend)
make test-integration

# Test coverage report
make test-cover
open coverage.html
```

### Code Organization

- All internal packages live under `internal/` and are not importable by external code
- The `StorageBackend` interface in `internal/storage/backend.go` is the main extension point
- `MockBackend` in the same file is used across all test packages
- State is managed via SQLite (`modernc.org/sqlite`, pure Go, no CGO)

### Adding a New CLI Command

1. Create `cmd/packrat/mycommand.go`
2. Define a `cobra.Command` variable
3. Register it in `init()` with `rootCmd.AddCommand(...)`
4. Use `loadConfig()`, `setupLogger()`, `openStateDB()` helpers as needed

### Key Design Decisions

- **No CGO** — all dependencies are pure Go for maximum portability
- **rclone as peer dependency** — leverages rclone's mature auth flows and 70+ backend support
- **Interface-driven storage** — easy to add native backends without touching core logic
- **Snapshot-based, not continuous** — simpler, more predictable, less resource usage
- **Encryption is opt-in per group** — not everything needs encryption
- **SQLite for local state** — single file, no server, excellent Go support

### Dependencies

| Package | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/BurntSushi/toml` | Config parsing |
| `github.com/robfig/cron/v3` | Cron scheduling |
| `filippo.io/age` | Encryption |
| `github.com/zalando/go-keyring` | OS keyring |
| `modernc.org/sqlite` | State database (pure Go) |
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `github.com/charmbracelet/bubbles` | TUI components |
| `github.com/sergi/go-diff` | Text diffing |
| `gopkg.in/lumberjack.v2` | Log rotation |

## Generated by AI

This entire project — every line of Go code, every test, the Makefile, the TUI, the config system, and this README — was generated in a single session by [Claude Code](https://claude.ai/claude-code) (Claude Opus 4.6) from a natural language specification ([PACKRAT_SPEC.md](PACKRAT_SPEC.md)). No code was written by hand. The spec was authored by a human; everything else is Claude's work.

## License

MIT

---

*Built with 🐀 energy — hoard everything, lose nothing.*
