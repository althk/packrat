# Security & Bug Audit Report — 2026-03-22

After a thorough review of the entire packrat codebase, here are the findings organized by severity.

**Status: ALL 18 ISSUES FIXED** — All tests pass with race detector enabled.

---

## CRITICAL

### 1. Path Traversal in Restore — `internal/restore/restore.go:105-112` — FIXED

The restore path only strips leading `/` but doesn't sanitize `../` sequences. A malicious snapshot with a path like `../../etc/passwd` would escape the destination directory and overwrite arbitrary files.

```go
rel = strings.TrimPrefix(rel, "/")           // strips "/" but not "../"
destPath = filepath.Join(opts.DestDir, rel)   // resolves outside DestDir
```

**Fix applied:** Added path traversal guard after `filepath.Join` that validates the cleaned destination path stays within `opts.DestDir` using `strings.HasPrefix`.

---

### 2. TOCTOU Race in Lock File — `internal/backup/engine.go` — FIXED

Lock acquisition is non-atomic: read PID → check process → delete → write. Between delete and write, another process can acquire the lock, causing two concurrent backups that corrupt the SQLite database and remote storage.

**Fix applied:** Replaced with atomic `os.OpenFile()` using `O_CREATE|O_EXCL`. On conflict, checks existing lock's PID liveness, removes if stale, and retries once atomically.

---

### 3. Stale Lock Detection Broken on Non-Linux — `internal/backup/engine.go` — FIXED

The `/proc/<pid>` check is Linux-only. On macOS/Windows, `os.FindProcess()` always succeeds on Unix without verifying the process exists, so the stale lock check always considers the lock stale — allowing concurrent backups.

**Fix applied:** Replaced `/proc` check with `syscall.Kill(pid, 0)` which portably checks process existence on all Unix systems.

---

### 4. Missing DB Transactions in SaveSnapshot — `internal/backup/state.go` — FIXED

`SaveSnapshot()` and `RecordBackupRun()` do single INSERTs without transactions. Multiple goroutines from parallel backup groups call these concurrently. With SQLite's single-writer model this can cause `SQLITE_BUSY` errors or partial writes.

**Fix applied:** Added `sync.Mutex` to `StateDB` to serialize all write operations. `SaveSnapshot` now uses `db.Begin()`/`tx.Commit()` transactions. Enabled WAL mode for better concurrent read performance.

---

### 5. Manifest Uploaded Before State Saved — `internal/backup/engine.go` — FIXED

If manifest upload succeeds but `SaveSnapshot` fails, the remote has a manifest referencing this backup but local state doesn't know about it. The next backup re-uploads everything, creating orphaned blobs.

**Fix applied:** Reversed ordering — local state is saved first, then manifest is uploaded. On manifest upload failure, local state is rolled back via `DeleteSnapshot`.

---

## HIGH

### 6. `Exists()` Swallows All Errors — `internal/storage/rclone.go` — FIXED

Returns `(false, nil)` for ALL errors including network failures. Callers think the file doesn't exist when actually the storage is unreachable. This could trigger unnecessary re-uploads or data loss.

**Fix applied:** Now returns the error for all failures except rclone exit codes 3 (directory not found) and 4 (file not found), which genuinely mean "does not exist".

---

### 7. Path Traversal in LocalBackend — `internal/storage/local.go` — FIXED

`fullPath()` does `filepath.Join(basePath, remotePath)` with no traversal validation. A `remotePath` containing `../` escapes the storage directory.

**Fix applied:** `fullPath()` now returns `(string, error)` and validates the cleaned result stays within `basePath` using `strings.HasPrefix`. All callers updated to handle the error.

---

### 8. Symlink Attack in Dotfiles Restore — `internal/shell/dotfiles.go` — FIXED

`SymlinkRestore()` creates symlinks from backup data without validating that source paths don't escape the dotfiles directory or follow existing symlinks to arbitrary locations.

**Fix applied:** Added path traversal validation on the source path and `filepath.EvalSymlinks` check to ensure resolved paths don't escape the dotfiles directory.

---

### 9. Setuid/Setgid Bits Restored Unchecked — `internal/restore/restore.go` — FIXED

File modes from snapshots are applied as-is via `os.Chmod()`. A crafted snapshot could set setuid/setgid bits, enabling privilege escalation.

**Fix applied:** Added `mode &^= os.ModeSetuid | os.ModeSetgid | os.ModeSticky` before applying file permissions.

---

### 10. Silent Data Loss from Corrupted Snapshots — `internal/backup/state.go` — FIXED

`ListSnapshots()` silently `continue`s on unmarshal errors. Corrupted snapshots vanish from the list with no user notification.

**Fix applied:** Added `slog.Warn("skipping corrupted snapshot manifest", "error", err)` to log corrupted entries.

---

### 11. Context Ignored in Retry Loop — `internal/storage/rclone.go` — FIXED

`withRetry()` uses `time.Sleep()` without checking `ctx.Done()`. Cancelled contexts still sleep through all retries (up to 7 seconds with exponential backoff).

**Fix applied:** Changed `withRetry` signature to accept `context.Context`. Replaced `time.Sleep` with `select` on `time.NewTimer` and `ctx.Done()`. All callers updated to pass `ctx`.

---

## MEDIUM

### 12. File Modified Between Hash and Upload — `internal/backup/engine.go` — FIXED

`WalkPaths()` hashes files at time T1, `uploadBlob()` reads at T2. File content could change between these, causing hash/content mismatch in the backup.

**Fix applied:** `uploadBlob` now re-computes SHA-256 after reading the file and compares against the expected hash. Returns an error if the file was modified during backup.

---

### 13. Config Files Saved World-Readable — `internal/config/config.go` — FIXED

`os.Create()` defaults to `0o644`. Config may contain encryption recipients. Should use `0o600`.

**Fix applied:** Replaced `os.Create()` with `os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)`.

---

### 14. PID File World-Readable — `internal/scheduler/daemon.go` — FIXED

Written with `0o644`. Should be `0o600`.

**Fix applied:** Changed `os.WriteFile` permission argument from `0o644` to `0o600`.

---

### 15. Lock Release Error Ignored — `internal/backup/engine.go` — FIXED

`releaseLock()` doesn't check `os.Remove()` error. If removal fails, next backup hangs until stale timeout.

**Fix applied:** `releaseLock` now checks the error from `os.Remove` and logs a warning (ignoring `ErrNotExist`).

---

### 16. `mustAtoi` Returns 0 on Failure — `internal/backup/engine.go` — FIXED

PID parsing failure returns 0, which is the kernel's PID on Linux. `os.FindProcess(0)` then checks if PID 0 exists.

**Fix applied:** Removed `mustAtoi` entirely. Lock acquisition now uses `strconv.Atoi` with proper error handling — corrupt PID files are treated as stale locks and removed.

---

### 17. MockBackend Returns `io.EOF` for Missing Files — `internal/storage/backend.go` — FIXED

Inconsistent with `LocalBackend` which returns `os.ErrNotExist`. Callers checking error types will behave differently per backend.

**Fix applied:** Added `ErrNotFound` sentinel error to storage package. `MockBackend.Download` now returns `fmt.Errorf("file not found: %s: %w", remotePath, ErrNotFound)`.

---

### 18. Key File Permissions Not Verified on Load — `internal/crypto/keyring.go` — FIXED

Keys are saved with `0o600` but existing keys are loaded without checking they haven't been made world-readable.

**Fix applied:** `LoadKeyFromFile` now checks file permissions before reading. Rejects files where group or other bits are set (`mode & 0o077 != 0`) with a descriptive error message.
