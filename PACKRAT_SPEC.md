# Packrat — App Specification

> *"Because you never know when you'll need that .bashrc from 3 weeks ago."*

Packrat is a CLI tool + background daemon that automatically backs up shell history, dotfiles, config directories, and arbitrary paths to remote storage via rclone. It uses git-style versioning, encrypts sensitive data at rest, and provides a TUI for easy restore — especially when setting up a new machine.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [Project Structure](#3-project-structure)
4. [Core Features (v1)](#4-core-features-v1)
5. [Configuration](#5-configuration)
6. [CLI Interface](#6-cli-interface)
7. [Backup Engine](#7-backup-engine)
8. [Versioning System](#8-versioning-system)
9. [Encryption](#9-encryption)
10. [Scheduler / Daemon](#10-scheduler--daemon)
11. [Restore System & TUI](#11-restore-system--tui)
12. [Storage Backend (rclone)](#12-storage-backend-rclone)
13. [First-Run / Init Experience](#13-first-run--init-experience)
14. [New Machine Bootstrap](#14-new-machine-bootstrap)
15. [Logging & Observability](#15-logging--observability)
16. [Error Handling Strategy](#16-error-handling-strategy)
17. [Testing Strategy](#17-testing-strategy)
18. [Future Features (v2+)](#18-future-features-v2)
19. [Dependencies](#19-dependencies)
20. [Build & Release](#20-build--release)
21. [Technical Constraints](#21-technical-constraints)

---

## 1. Overview

| Property       | Value                                            |
|----------------|--------------------------------------------------|
| Language        | Go (1.22+)                                      |
| Config format   | TOML                                            |
| Storage backend | rclone (supports Google Drive, S3, Dropbox, etc.)|
| Scheduler       | Built-in (no systemd dependency)                |
| Encryption      | AES-256-GCM (age library)                       |
| Versioning      | Git-style snapshots with deduplication          |
| Platform        | Linux first (v1), macOS/Windows (v2)            |
| Install target  | Single binary + `rclone` as peer dependency     |

---

## 2. Architecture

```
┌──────────────────────────────────────────────────────────┐
│                        CLI Layer                         │
│  packrat init | backup | restore | status | daemon | ... │
└─────────────────────────┬────────────────────────────────┘
                          │
┌─────────────────────────▼────────────────────────────────┐
│                      Core Engine                         │
│  ┌───────────┐ ┌───────────┐ ┌──────────┐ ┌───────────┐ │
│  │ Scheduler │ │  Differ   │ │ Encryptor│ │  Snapshotter│ │
│  │ (cron)    │ │ (delta)   │ │ (age)    │ │  (versions)│ │
│  └───────────┘ └───────────┘ └──────────┘ └───────────┘ │
└─────────────────────────┬────────────────────────────────┘
                          │
┌─────────────────────────▼────────────────────────────────┐
│                   Storage Abstraction                    │
│            interface StorageBackend {                     │
│              Upload(ctx, path, reader) error              │
│              Download(ctx, path, writer) error            │
│              List(ctx, prefix) ([]Entry, error)           │
│              Delete(ctx, path) error                      │
│            }                                             │
└─────────────────────────┬────────────────────────────────┘
                          │
┌─────────────────────────▼────────────────────────────────┐
│                  Rclone Adapter                          │
│        Wraps rclone CLI commands behind the interface     │
└──────────────────────────────────────────────────────────┘
```

### Key Design Decisions

- **Interface-driven storage**: `StorageBackend` interface so we can add native Google Drive, S3, local-disk backends later without touching core logic.
- **Snapshot-based, not continuous**: Backups run on schedule (or manual trigger), not on file-watch. Simpler, more predictable, less resource usage.
- **Encryption is opt-in per path group**: Some configs have secrets, some don't. User decides what gets encrypted.
- **rclone is a peer dependency, not embedded**: Users install rclone separately. Packrat invokes it via CLI. This keeps our binary small and leverages rclone's mature auth flows (especially OAuth for Google Drive).

---

## 3. Project Structure

```
packrat/
├── cmd/
│   └── packrat/
│       └── main.go                  # Entry point, CLI root
├── internal/
│   ├── backup/
│   │   ├── engine.go                # Orchestrates a backup run
│   │   ├── engine_test.go
│   │   ├── differ.go                # Computes file diffs/changes
│   │   ├── differ_test.go
│   │   ├── snapshot.go              # Snapshot creation & metadata
│   │   └── snapshot_test.go
│   ├── config/
│   │   ├── config.go                # TOML config loading & validation
│   │   ├── config_test.go
│   │   ├── defaults.go              # Default paths per OS
│   │   └── migrate.go               # Config version migrations
│   ├── crypto/
│   │   ├── encrypt.go               # AES-256-GCM encryption (age)
│   │   ├── encrypt_test.go
│   │   ├── keyring.go               # Key management
│   │   └── keyring_test.go
│   ├── restore/
│   │   ├── restore.go               # Restore logic
│   │   ├── restore_test.go
│   │   ├── conflict.go              # Conflict detection & resolution
│   │   └── conflict_test.go
│   ├── scheduler/
│   │   ├── scheduler.go             # Cron-based built-in scheduler
│   │   └── scheduler_test.go
│   ├── shell/
│   │   ├── history.go               # Shell history detection & collection
│   │   ├── history_test.go
│   │   ├── dotfiles.go              # Dotfile discovery & management
│   │   └── dotfiles_test.go
│   ├── storage/
│   │   ├── backend.go               # StorageBackend interface
│   │   ├── rclone.go                # Rclone adapter implementation
│   │   ├── rclone_test.go
│   │   └── local.go                 # Local filesystem backend (for testing/local backups)
│   ├── tui/
│   │   ├── app.go                   # TUI application (bubbletea)
│   │   ├── views/
│   │   │   ├── snapshot_list.go     # Browse snapshots
│   │   │   ├── file_browser.go      # Browse files in a snapshot
│   │   │   ├── diff_view.go         # View changes between snapshots
│   │   │   ├── restore_confirm.go   # Confirm restore actions
│   │   │   └── progress.go          # Upload/download progress
│   │   └── styles.go                # Shared TUI styles (lipgloss)
│   ├── platform/
│   │   ├── logger.go                # Structured logging setup (slog)
│   │   ├── errors.go                # Sentinel errors
│   │   ├── paths.go                 # OS-specific path resolution
│   │   └── paths_test.go
│   └── hooks/
│       ├── hooks.go                 # Pre/post backup hook execution
│       └── hooks_test.go
├── configs/
│   └── default.toml                 # Example/default config
├── scripts/
│   ├── install.sh                   # One-line install script
│   └── completions/
│       ├── packrat.bash
│       ├── packrat.zsh
│       └── packrat.fish
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── LICENSE
└── .goreleaser.yaml                 # Cross-platform release config
```

---

## 4. Core Features (v1)

### 4.1 Shell History Backup
- Auto-detect shell type: bash (`.bash_history`), zsh (`.zsh_history`), fish (`~/.local/share/fish/fish_history`)
- Merge history across machines (append-only, deduplicate by timestamp+command)
- Handle shell-specific formats (zsh extended history with timestamps, fish's YAML-like format)

### 4.2 Dotfiles Management
- Track common dotfiles by default: `.bashrc`, `.zshrc`, `.vimrc`, `.gitconfig`, `.tmux.conf`, `.profile`, `.bash_profile`, `.aliases`, `.ssh/config` (NOT keys unless user explicitly opts in)
- User can add/remove from tracked list via config
- Symlink restoration option: restore files to original locations OR create a `~/.dotfiles/` repo with symlinks

### 4.3 Config Directory Backup
- Default tracked dirs: `~/.claude/`, `~/.config/`, `~/.gnupg/` (pubkeys only by default)
- Support glob patterns: `~/.config/nvim/**`, `~/.config/Code/User/settings.json`
- Exclude patterns: `~/.config/chromium/`, `*.cache`, `node_modules/`
- Size warnings: alert if a tracked path exceeds configurable threshold (default 100MB)

### 4.4 Custom Paths
- User specifies arbitrary files/dirs in config
- Each path group can have its own encryption setting and backup schedule

### 4.5 Git-style Versioning
- Each backup creates a snapshot with metadata (timestamp, hostname, changed files, checksums)
- Incremental: only changed files are stored (content-addressable, SHA-256)
- Configurable retention: keep last N snapshots, or time-based (keep all from last 30 days)
- Diff between any two snapshots

### 4.6 Encryption at Rest
- Encrypt sensitive path groups before upload using `age` (https://age-encryption.org/)
- Passphrase-based encryption (user provides passphrase during init, stored in OS keyring)
- Per-path-group encryption toggle in config
- Encrypted files get `.age` extension in remote storage

### 4.7 TUI Restore Interface
- Built with Bubble Tea + Lip Gloss
- Screens:
  - **Snapshot browser**: list all snapshots, sorted by date, filterable by hostname
  - **File browser**: tree view of files in a snapshot, with change indicators (added/modified/deleted)
  - **Diff viewer**: side-by-side or unified diff of a file between snapshots
  - **Restore wizard**: select what to restore (full snapshot, specific files, specific path groups), choose destination, confirm
  - **Progress view**: real-time upload/download progress with ETA

### 4.8 Pre/Post Backup Hooks
- Run arbitrary shell commands before/after backup
- Example uses: dump `brew list` to a file before backup, stop a service, notify via webhook
- Configurable per path group
- Timeout + failure handling (abort backup on pre-hook failure, or continue)

### 4.9 Machine Profiles
- Each machine gets a unique ID (generated at init, stored in config)
- Backups are namespaced by machine: `packrat-data/<machine-id>/snapshots/...`
- Restore can pull from any machine's backups (cross-machine restore)

### 4.10 Integrity Verification
- SHA-256 checksums for every file in every snapshot
- `packrat verify` command compares local state against last snapshot checksums
- Periodic integrity check of remote storage (configurable)

---

## 5. Configuration

### 5.1 Config Location
- Default: `~/.config/packrat/config.toml`
- Override via `PACKRAT_CONFIG` env var or `--config` flag

### 5.2 Config Schema

```toml
# Packrat Configuration
# Generated by `packrat init`

[general]
machine_name = "harish-workstation"        # Human-readable, auto-generated
machine_id = "a1b2c3d4"                    # Unique ID, auto-generated at init
log_level = "info"                         # debug | info | warn | error
log_file = "~/.local/share/packrat/packrat.log"

[scheduler]
enabled = true
default_interval = "1h"                    # Default backup frequency
quiet_hours_start = "23:00"                # Optional: no backups between these hours
quiet_hours_end = "06:00"

[storage]
backend = "rclone"
rclone_remote = "gdrive"                   # Name of rclone remote (configured via `rclone config`)
remote_base_path = "packrat-backups"       # Folder on remote storage
bandwidth_limit = ""                       # Optional: "1M" for 1MB/s, empty for unlimited

[encryption]
enabled = true
key_source = "keyring"                     # "keyring" (OS keyring) | "file" (age key file) | "prompt"
key_file = ""                              # Only used if key_source = "file"

[versioning]
strategy = "snapshot"                      # "snapshot" (incremental content-addressed)
retention_count = 50                       # Keep last N snapshots (0 = unlimited)
retention_days = 30                        # Also keep anything from last N days
# Both rules are OR'd: a snapshot is kept if it matches EITHER rule

[notifications]
enabled = false
on_failure = true                          # Notify on backup failure
on_success = false                         # Notify on backup success
webhook_url = ""                           # POST JSON payload to this URL
# desktop_notify = true                    # Future: libnotify / osascript

# ─────────────────────────────────────────────
# Backup Groups
# Each [[backup]] block defines a path group.
# ─────────────────────────────────────────────

[[backup]]
name = "shell-history"
paths = [
    "~/.bash_history",
    "~/.zsh_history",
    "~/.local/share/fish/fish_history",
]
encrypt = false
interval = "30m"                           # Override default interval for this group
exclude = []

[[backup]]
name = "dotfiles"
paths = [
    "~/.bashrc",
    "~/.zshrc",
    "~/.bash_profile",
    "~/.profile",
    "~/.aliases",
    "~/.vimrc",
    "~/.tmux.conf",
    "~/.gitconfig",
    "~/.ssh/config",
]
encrypt = false
interval = "1h"
exclude = []

[[backup]]
name = "ai-configs"
paths = [
    "~/.claude/",
    "~/.gemini/",
    "~/.config/github-copilot/",
]
encrypt = true                             # These may contain tokens/secrets
interval = "2h"
exclude = ["*.log", "*.cache"]

[[backup]]
name = "editor-configs"
paths = [
    "~/.config/nvim/",
    "~/.config/Code/User/settings.json",
    "~/.config/Code/User/keybindings.json",
    "~/.config/Code/User/snippets/",
]
encrypt = false
interval = "1h"
exclude = ["*.cache", "workspaceStorage/"]

[[backup]]
name = "gnupg"
paths = ["~/.gnupg/"]
encrypt = true
interval = "6h"
exclude = ["*.lock", "S.gpg-agent*", "random_seed"]

# ─────────────────────────────────────────────
# Hooks
# ─────────────────────────────────────────────

[[hook]]
name = "dump-package-lists"
when = "pre-backup"                        # "pre-backup" | "post-backup"
command = """
dpkg --get-selections > ~/.config/packrat/installed-packages.txt 2>/dev/null || true
pip list --format=freeze > ~/.config/packrat/pip-packages.txt 2>/dev/null || true
"""
timeout = "30s"
fail_action = "continue"                   # "continue" | "abort"

[[hook]]
name = "post-backup-notify"
when = "post-backup"
command = 'echo "Packrat backup completed at $(date)" >> /tmp/packrat-notify.log'
timeout = "10s"
fail_action = "continue"
```

### 5.3 Config Validation Rules
- `machine_id` is auto-generated (UUIDv4 short hash), never edited by user
- All paths are expanded (`~` → home dir, env vars resolved)
- Duplicate paths across backup groups are warned (not errored)
- `rclone_remote` must exist in rclone config (validate at startup)
- Intervals must be valid Go duration strings (`30m`, `1h`, `6h`, etc.)
- Minimum interval: 5 minutes (prevent accidental rapid-fire)

---

## 6. CLI Interface

All commands follow this pattern: `packrat <command> [subcommand] [flags]`

### Commands

```
packrat init                        # First-time setup wizard
packrat backup                      # Run backup now (all groups)
packrat backup --group dotfiles     # Run backup for specific group
packrat backup --dry-run            # Show what would be backed up
packrat restore                     # Launch TUI restore interface
packrat restore --snapshot <id>     # Restore specific snapshot (non-interactive)
packrat restore --list              # List available snapshots (non-interactive)
packrat restore --file <path>       # Restore a specific file
packrat status                      # Show daemon status, last backup time, next scheduled
packrat diff                        # Diff current state vs last snapshot
packrat diff <snap1> <snap2>        # Diff between two snapshots
packrat verify                      # Verify integrity of local files vs last snapshot
packrat daemon start                # Start background scheduler
packrat daemon stop                 # Stop background scheduler
packrat daemon status               # Check if daemon is running
packrat log                         # Tail the log file
packrat log --lines 50              # Show last 50 log lines
packrat config show                 # Print resolved config
packrat config edit                 # Open config in $EDITOR
packrat config validate             # Check config for errors
packrat config add-path <path>      # Quick-add a path to backup (interactive group selection)
packrat history                     # Show backup history (snapshot list with stats)
packrat history --group dotfiles    # Filter by group
packrat nuke --remote               # Delete all remote data (with confirmation)
packrat nuke --local                # Delete all local packrat data
packrat version                     # Print version info
```

### Global Flags
```
--config <path>        # Override config file location
--verbose / -v         # Enable debug logging for this invocation
--quiet / -q           # Suppress all output except errors
--no-color             # Disable colored output
```

### CLI Framework
- Use `cobra` for CLI structure and `viper` for config (though viper should only supplement TOML loading)
- Shell completions generated via cobra's built-in completion support (bash, zsh, fish)

---

## 7. Backup Engine

### 7.1 Backup Flow

```
1. Load config
2. Acquire file lock (~/.local/share/packrat/packrat.lock) — prevent concurrent runs
3. Run pre-backup hooks
4. For each backup group (in parallel, configurable concurrency):
   a. Expand paths, apply exclude patterns
   b. Walk file tree, compute SHA-256 for each file
   c. Compare against last snapshot's manifest
   d. Identify changed/added/deleted files
   e. For changed/added files:
      - Read file content
      - If group has encrypt=true, encrypt content with age
      - Store content in content-addressable blob store (keyed by SHA-256)
   f. Create snapshot manifest (JSON):
      {
        "id": "snap-20240315-143022-a1b2",
        "timestamp": "2024-03-15T14:30:22Z",
        "machine_id": "a1b2c3d4",
        "machine_name": "harish-workstation",
        "group": "dotfiles",
        "files": [
          {
            "path": "~/.bashrc",
            "sha256": "abc123...",
            "size": 4096,
            "mode": "0644",
            "mod_time": "2024-03-15T10:00:00Z",
            "encrypted": false,
            "status": "modified"   // "added" | "modified" | "deleted" | "unchanged"
          }
        ],
        "stats": {
          "total_files": 12,
          "changed_files": 2,
          "added_files": 0,
          "deleted_files": 0,
          "total_size": 45056,
          "upload_size": 8192
        }
      }
5. Upload new blobs + snapshot manifest to remote via rclone
6. Update local state DB (last snapshot ID per group)
7. Run post-backup hooks
8. Release file lock
```

### 7.2 Remote Directory Structure

```
packrat-backups/
└── <machine-id>/
    ├── manifests/
    │   ├── dotfiles/
    │   │   ├── snap-20240315-143022-a1b2.json
    │   │   └── snap-20240316-090000-c3d4.json
    │   └── shell-history/
    │       └── ...
    ├── blobs/
    │   ├── ab/
    │   │   └── abc123def456...          # Content-addressed by SHA-256
    │   └── cd/
    │       └── cdef789...
    └── meta/
        └── machine-info.json            # Machine name, OS, creation date
```

### 7.3 Content-Addressable Storage
- Files are stored by SHA-256 hash in a 2-level directory (first 2 chars / rest)
- Deduplication is automatic: if two files (even across groups) have the same content, only one blob exists
- Encrypted files are stored as `<sha256>.age` — the hash is of the plaintext, encryption happens before upload
- This means: if encryption key changes, blobs must be re-uploaded (detected by missing `.age` blobs)

### 7.4 Local State
- Stored in `~/.local/share/packrat/`
- Contains:
  - `state.db` — SQLite database with: last snapshot ID per group, blob inventory, backup history
  - `packrat.lock` — PID file lock for preventing concurrent runs
  - `packrat.log` — Log output (rotated by size, keep last 5)
- Why SQLite: single-file, no server, Go has excellent support via `modernc.org/sqlite` (pure Go, no CGO)

---

## 8. Versioning System

### 8.1 Snapshot Model
- Each snapshot is a JSON manifest (see Section 7.1)
- Snapshots are immutable once created
- Snapshot ID format: `snap-YYYYMMDD-HHMMSS-<4char-random>`

### 8.2 Diffing
- Diff between any two snapshots by comparing their file manifests
- Output: list of added/modified/deleted files with optional content diff
- Content diff: use Go's `github.com/sergi/go-diff` for unified diff output
- For binary files: just report "changed" with old/new SHA and size

### 8.3 Retention / Garbage Collection
- Retention rules (configurable):
  - Keep last N snapshots per group
  - Keep all snapshots from last N days
  - Rules are OR'd (a snapshot survives if it matches either)
- GC runs after every successful backup:
  - Delete expired snapshot manifests
  - Scan all remaining manifests → collect referenced blob hashes
  - Delete orphaned blobs (not referenced by any snapshot)
- GC can also be triggered manually: `packrat gc` (add this to CLI)

---

## 9. Encryption

### 9.1 Library
- Use `filippo.io/age` — the Go implementation of the age encryption format
- age is modern, audited, simple, and has no config knobs to get wrong

### 9.2 Key Management
- Three key source modes (configured in `[encryption]`):
  1. **keyring** (default): Store age identity in OS keyring (go-keyring library). Key is generated at `packrat init`.
  2. **file**: Store age identity in a file (user-specified path). User is responsible for securing it.
  3. **prompt**: Ask for passphrase every time. Derive age identity from passphrase using scrypt.
- The age **recipient** (public key) is stored in config for encryption. The **identity** (private key) is stored securely per the key_source mode and needed only for decryption/restore.

### 9.3 Encryption Flow
```
File content → age.Encrypt(recipient) → Upload encrypted blob
Download encrypted blob → age.Decrypt(identity) → Restored file content
```

### 9.4 Key Rotation
- `packrat rotate-key` command (add to CLI)
- Generates new key pair, re-downloads and re-encrypts all encrypted blobs, re-uploads
- Old key is kept in a `previous_keys` list for decrypting old snapshots during transition

---

## 10. Scheduler / Daemon

### 10.1 Built-in Scheduler
- Use `github.com/robfig/cron/v3` for cron-like scheduling within the Go process
- The daemon runs as a background process (not systemd-dependent)
- Started via `packrat daemon start`, which forks a background process and writes a PID file
- PID file: `~/.local/share/packrat/daemon.pid`

### 10.2 Daemon Lifecycle
```
packrat daemon start
  → Check if already running (PID file + process check)
  → Fork to background (or use `go run` with nohup-like behavior)
  → Write PID file
  → Start scheduler with configured intervals per group
  → Log to packrat.log

packrat daemon stop
  → Read PID file
  → Send SIGTERM
  → Daemon catches signal, finishes current backup (if any), exits cleanly
  → Remove PID file

packrat daemon status
  → Check PID file + verify process is alive
  → Show: running/stopped, uptime, next scheduled backup per group
```

### 10.3 Missed Backups
- On daemon start, check last backup time per group
- If a backup is overdue (last backup + interval < now), run it immediately
- This handles laptop sleep/hibernate gracefully

### 10.4 Concurrency
- Backup groups can run in parallel (configurable max concurrency, default 2)
- File lock prevents `packrat backup` (manual) from colliding with daemon backups

---

## 11. Restore System & TUI

### 11.1 TUI Framework
- **Bubble Tea** (`github.com/charmbracelet/bubbletea`) for the terminal UI framework
- **Lip Gloss** (`github.com/charmbracelet/lipgloss`) for styling
- **Bubbles** (`github.com/charmbracelet/bubbles`) for common components (list, viewport, textinput, spinner, progress)

### 11.2 TUI Screens

**Screen 1: Snapshot Browser**
```
┌─ Packrat Restore ─────────────────────────────────────┐
│                                                        │
│  Machine: harish-workstation (a1b2c3d4)               │
│                                                        │
│  Filter: [All groups ▾]  [All machines ▾]             │
│                                                        │
│  ┌────────────────────────────────────────────────┐   │
│  │ ● snap-20240316-090000  dotfiles    2 changed  │   │
│  │   snap-20240315-143022  dotfiles    5 changed  │   │
│  │   snap-20240315-140000  shell-hist  1 changed  │   │
│  │   snap-20240315-120000  ai-configs  3 changed  │   │
│  │   ...                                          │   │
│  └────────────────────────────────────────────────┘   │
│                                                        │
│  [Enter] Browse files  [d] Diff  [r] Restore  [q] Quit│
└────────────────────────────────────────────────────────┘
```

**Screen 2: File Browser** (within a snapshot)
```
┌─ Snapshot: snap-20240316-090000 (dotfiles) ───────────┐
│                                                        │
│  ~/.bashrc                    [modified]  4.0 KB      │
│  ~/.zshrc                     [unchanged] 2.1 KB      │
│  ~/.gitconfig                 [modified]  512 B       │
│  ~/.vimrc                     [unchanged] 8.3 KB      │
│  ~/.tmux.conf                 [added]     1.2 KB      │
│                                                        │
│  [Enter] View diff  [r] Restore file  [a] Restore all │
│  [Space] Select multiple  [Esc] Back                  │
└────────────────────────────────────────────────────────┘
```

**Screen 3: Restore Confirmation**
```
┌─ Confirm Restore ─────────────────────────────────────┐
│                                                        │
│  Restoring 3 files from snap-20240316-090000:         │
│                                                        │
│    ~/.bashrc        → overwrite (local modified)      │
│    ~/.gitconfig     → overwrite (local unchanged)     │
│    ~/.tmux.conf     → create new                      │
│                                                        │
│  Conflicts detected:                                   │
│    ~/.bashrc has local changes not in any snapshot.    │
│    [b] Backup local first  [o] Overwrite  [s] Skip    │
│                                                        │
│  [Enter] Proceed  [Esc] Cancel                        │
└────────────────────────────────────────────────────────┘
```

**Screen 4: Progress View**
```
┌─ Restoring... ────────────────────────────────────────┐
│                                                        │
│  Downloading:  ████████████░░░░░░░░  60%  (3/5 files) │
│  Current:      ~/.config/nvim/init.lua                │
│  Speed:        2.4 MB/s                               │
│  ETA:          12s                                    │
│                                                        │
└────────────────────────────────────────────────────────┘
```

### 11.3 Conflict Resolution
- Before restoring, compare snapshot file checksums with current local files
- Three states:
  - **Clean**: local file matches a known snapshot → safe to overwrite
  - **Diverged**: local file doesn't match any snapshot → warn user, offer to backup first
  - **Missing**: file doesn't exist locally → safe to create
- Conflict resolution options per file: backup local first, overwrite, skip

### 11.4 Non-Interactive Restore
- `packrat restore --snapshot <id> --yes` — restore everything, no prompts
- `packrat restore --snapshot <id> --file ~/.bashrc` — restore single file
- `packrat restore --snapshot <id> --group dotfiles --dest ~/restore-test/` — restore to alternate directory
- `packrat restore --latest --group dotfiles` — restore from most recent snapshot of group

---

## 12. Storage Backend (rclone)

### 12.1 Interface

```go
// StorageBackend abstracts remote storage operations.
type StorageBackend interface {
    // Upload copies local file content to remote path.
    Upload(ctx context.Context, remotePath string, reader io.Reader) error
    
    // Download retrieves remote file content.
    Download(ctx context.Context, remotePath string, writer io.Writer) error
    
    // List returns entries under the given remote prefix.
    List(ctx context.Context, prefix string) ([]RemoteEntry, error)
    
    // Delete removes a remote file.
    Delete(ctx context.Context, remotePath string) error
    
    // Exists checks if a remote path exists.
    Exists(ctx context.Context, remotePath string) (bool, error)
}

type RemoteEntry struct {
    Path    string
    Size    int64
    ModTime time.Time
    IsDir   bool
}
```

### 12.2 Rclone Adapter Implementation
- Wraps rclone CLI commands: `rclone copyto`, `rclone cat`, `rclone lsjson`, `rclone deletefile`
- Checks for rclone binary at startup (`rclone version`)
- Validates configured remote exists (`rclone listremotes`)
- Supports bandwidth limiting via `--bwlimit` flag (from config)
- All rclone commands use `--config` to point at rclone's own config (default `~/.config/rclone/rclone.conf`)
- Timeout per operation: configurable, default 5 minutes
- Retry logic: 3 retries with exponential backoff on transient failures

### 12.3 Setup Flow
- During `packrat init`, check if rclone is installed
- If not: print install instructions for user's OS
- If yes: check if a remote is configured. If not, prompt user to run `rclone config` and provide a guided hint for Google Drive setup
- Validate remote access by uploading a small test file and deleting it

---

## 13. First-Run / Init Experience

`packrat init` should be the only command a new user needs to get started.

### Flow:
```
$ packrat init

  🐀 Welcome to Packrat!
  Let's get your backups configured.

  Step 1/5: Machine Name
  > What should we call this machine? [harish-workstation]

  Step 2/5: Storage Backend
  > Checking for rclone... ✓ Found (v1.65.0)
  > Available remotes: gdrive, dropbox-personal
  > Which remote should Packrat use? [gdrive]
  > Testing connection... ✓ Connected

  Step 3/5: Encryption
  > Enable encryption for sensitive configs? [Y/n] y
  > Generating encryption key... ✓
  > Key stored in OS keyring.
  > ⚠️  Save this recovery key somewhere safe:
  >    AGE-SECRET-KEY-1QQQQ...XXXXX
  >    (You'll need this if you lose access to this machine's keyring)

  Step 4/5: What to Back Up
  > Detected shell: zsh
  > Auto-configured backup groups:
  >   ✓ shell-history  (~/.zsh_history)
  >   ✓ dotfiles       (~/.zshrc, ~/.gitconfig, +4 more)
  >   ✓ ai-configs     (~/.claude/, ~/.gemini/) [encrypted]
  >   ✓ editor-configs  (~/.config/nvim/)
  > Add custom paths? [y/N]

  Step 5/5: Schedule
  > Default backup interval: [1h]
  > Start daemon now? [Y/n] y

  ✓ Config written to ~/.config/packrat/config.toml
  ✓ Daemon started (PID 12345)
  ✓ First backup running in background...

  Run `packrat status` to check progress.
  Run `packrat config edit` to customize.
```

---

## 14. New Machine Bootstrap

The killer feature: set up a new machine from a Packrat backup.

### Flow:
```
$ # On new machine, after installing packrat + rclone:
$ packrat init --restore

  🐀 Packrat — New Machine Setup

  > Checking rclone... ✓
  > Which remote has your backups? [gdrive]
  > Scanning for Packrat data... ✓

  Found backups from:
    1. harish-workstation (last backup: 2h ago, 6 groups)
    2. harish-laptop (last backup: 3d ago, 4 groups)
  > Restore from which machine? [1]

  > Enter decryption passphrase (for encrypted groups): ****

  Available groups:
    [x] shell-history   (45 snapshots)
    [x] dotfiles        (120 snapshots)
    [x] ai-configs      (30 snapshots) [encrypted]
    [x] editor-configs  (90 snapshots)
    [ ] gnupg           (10 snapshots) [encrypted]
  > Toggle with space, Enter to continue

  Restoring latest snapshots...
    ✓ shell-history   — 1 file restored
    ✓ dotfiles        — 8 files restored
    ✓ ai-configs      — 12 files restored (decrypted)
    ✓ editor-configs  — 24 files restored

  ✓ Config written for this machine (new machine ID: e5f6g7h8)
  ✓ Daemon started — future backups will sync to the same remote.

  Welcome to your new machine! 🐀
```

---

## 15. Logging & Observability

### 15.1 Structured Logging
- Use `log/slog` (Go stdlib) with JSON handler for daemon mode, text handler for interactive CLI
- Log levels: DEBUG, INFO, WARN, ERROR
- Key fields on every log line:
  - `component`: which subsystem (backup, scheduler, storage, crypto, restore)
  - `group`: backup group name (when applicable)
  - `snapshot_id`: when operating on a specific snapshot
  - `machine_id`: always present

### 15.2 Log File
- Default: `~/.local/share/packrat/packrat.log`
- Rotation: by size (10MB default), keep last 5 rotated files
- Use `lumberjack` (`gopkg.in/natefineart/lumberjack.v2`) for rotation

### 15.3 Backup History / Stats
- After each backup, record in SQLite:
  - Timestamp, duration, group, files changed, bytes uploaded, success/failure, error message (if any)
- `packrat history` reads from this DB and displays a summary

---

## 16. Error Handling Strategy

### 16.1 Sentinel Errors

```go
var (
    ErrConfigNotFound    = errors.New("config file not found; run 'packrat init'")
    ErrConfigInvalid     = errors.New("invalid configuration")
    ErrDaemonRunning     = errors.New("daemon is already running")
    ErrDaemonNotRunning  = errors.New("daemon is not running")
    ErrRcloneNotFound    = errors.New("rclone binary not found; install from https://rclone.org")
    ErrRemoteNotFound    = errors.New("configured rclone remote not found")
    ErrRemoteUnreachable = errors.New("cannot reach remote storage")
    ErrLockAcquire       = errors.New("another backup is already running")
    ErrDecryptionFailed  = errors.New("decryption failed; wrong key or corrupted data")
    ErrSnapshotNotFound  = errors.New("snapshot not found")
    ErrIntegrityMismatch = errors.New("file integrity check failed; content has been tampered with or corrupted")
)
```

### 16.2 Error Wrapping Convention
- Always wrap with context: `fmt.Errorf("backing up group %s: %w", group.Name, err)`
- User-facing errors (CLI output) should be clear and actionable
- Internal errors get full context for logging

### 16.3 Failure Modes & Recovery
| Failure | Behavior |
|---|---|
| rclone not installed | Error at startup with install instructions |
| Remote unreachable | Retry 3x, then skip group, log error, continue other groups |
| Partial upload failure | Snapshot is not committed; next run will retry |
| Encryption key missing | Error with instructions to recover from keyring/file/prompt |
| Corrupted blob on remote | Detected by integrity check, reported to user, re-uploadable from local if available |
| Disk full | Detected pre-backup, warn user, skip backup |
| Config file missing | Clear error message, suggest `packrat init` |

---

## 17. Testing Strategy

### 17.1 Unit Tests
- Every package gets `_test.go` files
- Table-driven tests (Go convention)
- Mock the `StorageBackend` interface for backup/restore tests
- Mock the filesystem for differ/snapshot tests (use `testing/fstest` or `afero`)
- Target: 80%+ code coverage on core packages (`backup`, `crypto`, `config`, `restore`)

### 17.2 Integration Tests
- Test rclone adapter against a local rclone remote (rclone supports `:local:` backend)
- Test full backup → restore cycle with temp directories
- Test encryption round-trip (encrypt → upload → download → decrypt → verify)
- Test config loading with various TOML permutations

### 17.3 Test Tags
- Use build tags to separate unit and integration tests:
  - `//go:build !integration` for unit tests (default)
  - `//go:build integration` for integration tests
- Makefile targets: `make test` (unit only), `make test-integration` (all)

---

## 18. Future Features (v2+)

These are explicitly out of scope for v1 but should influence v1 architecture (don't paint ourselves into a corner):

- **macOS support**: launchd daemon, macOS keychain for key storage, Homebrew formula
- **Windows support**: Windows Task Scheduler, Windows Credential Manager, Scoop/Chocolatey package
- **Native Google Drive / S3 backends**: alongside rclone, for users who don't want the dependency
- **Selective file-watch mode**: fsnotify-based watching for critical files (in addition to scheduled)
- **Web UI**: lightweight local web interface as alternative to TUI
- **Team/shared configs**: shared backup groups across a team (e.g., standard dotfiles)
- **Compression**: gzip/zstd blobs before upload (content-addressable still works, just hash pre-compression)
- **Desktop notifications**: libnotify on Linux, osascript on macOS, toast on Windows
- **Plugin system**: user-defined backup "providers" (e.g., backup Docker volumes, database dumps)
- **Cloud-native storage backends**: direct S3, Azure Blob, GCS without rclone
- **2FA / multi-key encryption**: require multiple keys to decrypt (shamir's secret sharing)
- **Conflict-free merge for shell history**: CRDT-like merge across machines

---

## 19. Dependencies

### Direct Dependencies (all well-maintained, widely used)

| Dependency | Purpose | Justification |
|---|---|---|
| `github.com/spf13/cobra` | CLI framework | Industry standard for Go CLIs |
| `github.com/BurntSushi/toml` | TOML config parsing | Standard Go TOML library |
| `github.com/robfig/cron/v3` | Cron scheduler | Battle-tested, simple API |
| `filippo.io/age` | age encryption | Modern, audited, simple |
| `github.com/zalando/go-keyring` | OS keyring access | Cross-platform keyring |
| `modernc.org/sqlite` | SQLite (pure Go) | No CGO required, portable |
| `github.com/charmbracelet/bubbletea` | TUI framework | Best Go TUI library |
| `github.com/charmbracelet/lipgloss` | TUI styling | Pairs with bubbletea |
| `github.com/charmbracelet/bubbles` | TUI components | List, viewport, progress, spinner |
| `github.com/sergi/go-diff` | Text diffing | Unified diff output |
| `gopkg.in/natefineart/lumberjack.v2` | Log rotation | Simple, reliable |

### Standard Library Usage (prefer over deps where possible)
- `log/slog` — structured logging
- `crypto/sha256` — file hashing
- `os/exec` — rclone invocation
- `encoding/json` — snapshot manifests
- `path/filepath` — cross-platform paths
- `context` — cancellation/timeouts
- `testing` — test framework

---

## 20. Build & Release

### Makefile

```makefile
.PHONY: build test test-integration lint run install clean release

VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/packrat ./cmd/packrat

test:
	go test -v -race -count=1 ./...

test-integration:
	go test -v -race -count=1 -tags=integration ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run

run:
	go run ./cmd/packrat

install: build
	cp bin/packrat /usr/local/bin/

clean:
	rm -rf bin/ coverage.out coverage.html

release:
	goreleaser release --clean
```

### GoReleaser
- Cross-compile for: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
- Windows builds deferred to v2
- Generate checksums, create GitHub release
- Build shell completions into release archive

### Install Methods (v1)
```bash
# Option 1: Go install
go install github.com/<user>/packrat/cmd/packrat@latest

# Option 2: One-line script
curl -sSL https://raw.githubusercontent.com/<user>/packrat/main/scripts/install.sh | bash

# Option 3: From release binary
wget https://github.com/<user>/packrat/releases/latest/download/packrat-linux-amd64.tar.gz
tar xzf packrat-linux-amd64.tar.gz
sudo mv packrat /usr/local/bin/
```

---

## 21. Technical Constraints

- **Go 1.22+** required (for `log/slog` improvements, range-over-func if needed)
- **rclone** must be installed and configured separately (peer dependency)
- **No CGO**: all deps must be pure Go (this is why we use `modernc.org/sqlite` not `mattn/go-sqlite3`)
- **Single binary**: the final build must be a single statically-linked binary
- **No root required**: everything runs in user space (`~/.config/`, `~/.local/share/`)
- **Reasonable resource usage**: daemon should idle at <20MB RAM, <1% CPU when not actively backing up
- **Graceful degradation**: if rclone is temporarily unreachable, queue the backup and retry on next cycle

---

*Built with 🐀 energy — hoard everything, lose nothing.*
