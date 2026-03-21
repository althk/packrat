# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build                  # Build binary to bin/packrat (injects git version via ldflags)
make test                   # Run all unit tests (-v -race -count=1)
make test-integration       # Run all tests including integration tests (-tags=integration)
make test-cover             # Generate HTML coverage report
make lint                   # Run golangci-lint
```

Run a single test:
```bash
go test -v -race -count=1 ./internal/backup/ -run TestEngineRunGroup
```

## Architecture

Packrat is a CLI tool + background daemon for backing up files to remote storage via rclone. Module: `github.com/harish/packrat`.

### Package Dependency Graph

```
platform (paths, errors, logger)         ← zero internal deps, everything depends on this
config                                   ← platform
storage (interface + rclone/local/mock)  ← platform
crypto (age encryption + keyring)        ← platform
hooks                                    ← platform, config
shell (history parsing, dotfiles)        ← platform
backup (engine, differ, snapshot, state) ← all of the above
restore                                  ← backup, storage, crypto, config
scheduler (cron + daemon)                ← backup, config
tui (bubbletea restore UI)               ← backup, restore, storage, config
cmd/packrat (CLI)                        ← everything
```

### Key Abstractions

**StorageBackend** (`internal/storage/backend.go`) — the central interface. All storage operations go through `Upload`, `Download`, `List`, `Delete`, `Exists`. Three implementations: `RcloneBackend` (shells out to rclone CLI with retry), `LocalBackend` (filesystem), `MockBackend` (in-memory, used across all tests).

**Backup Engine** (`internal/backup/engine.go`) — orchestrates the full backup flow: acquire lock → run hooks → walk files → compute SHA-256 hashes → diff against last snapshot → upload changed blobs (content-addressable) → upload manifest → record in SQLite → release lock. Groups run in parallel via goroutines with a semaphore.

**StateDB** (`internal/backup/state.go`) — SQLite wrapper for local state. Two tables: `snapshots` (stores full JSON manifests) and `backup_runs` (history/stats). Uses `modernc.org/sqlite` (pure Go, no CGO).

### CLI Command Pattern

Commands live in `cmd/packrat/`, one file per command. Each registers itself in `func init()` via `rootCmd.AddCommand(...)`. Shared setup helpers in `root.go`: `loadConfig()` → `setupLogger()` → `openStateDB()` → `newStorageBackend()`. Global state: `appCfg *config.Config`, `stateDB *backup.StateDB`.

### Test Patterns

- Use `storage.NewMockBackend()` for testing anything that needs storage
- Use `t.TempDir()` for filesystem operations and temp SQLite DBs
- Helper functions like `testEngine(t)` create fully-configured test fixtures
- Integration tests use `//go:build integration` tag and `LocalBackend` against temp dirs

## Constraints

- **No CGO**: all deps must be pure Go. SQLite is `modernc.org/sqlite`, not `mattn/go-sqlite3`.
- **Single binary**: statically linked, cross-compiled for linux/darwin amd64/arm64.
- **rclone is a peer dependency**: invoked via `os/exec`, not embedded.
- **Config**: TOML at `~/.config/packrat/config.toml`, state at `~/.local/share/packrat/`.
- **Encryption**: `filippo.io/age` with keys in OS keyring (`go-keyring`), file, or passphrase. Opt-in per backup group.
- Blob hashing is on **plaintext** (not ciphertext) so dedup works across encrypted/unencrypted.
- Minimum backup interval enforced at 5 minutes.
