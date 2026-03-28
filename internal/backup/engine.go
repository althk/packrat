package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/crypto"
	"github.com/harish/packrat/internal/hooks"
	"github.com/harish/packrat/internal/platform"
	"github.com/harish/packrat/internal/storage"
)

// ProgressFunc is called to report backup progress for a group.
// stage is one of "scanning", "uploading", "done".
type ProgressFunc func(group, stage string, current, total int, bytes, totalBytes int64)

// BackupOptions controls backup behavior.
type BackupOptions struct {
	// Force creates a snapshot even when no changes are detected.
	Force bool
	// Verbose enables detailed per-file logging during backup.
	Verbose bool
	// OnProgress is called to report progress. May be nil.
	OnProgress ProgressFunc
}

// Engine orchestrates backup operations.
type Engine struct {
	cfg        *config.Config
	storage    storage.StorageBackend
	stateDB    *StateDB
	logger     *slog.Logger
	maxWorkers int
}

// NewEngine creates a new backup engine.
func NewEngine(cfg *config.Config, store storage.StorageBackend, stateDB *StateDB) *Engine {
	if cfg.General.ParallelUploads == 0 {
		cfg.General.ParallelUploads = 3
	}
	return &Engine{
		cfg:        cfg,
		storage:    store,
		stateDB:    stateDB,
		logger:     slog.Default(),
		maxWorkers: 2,
	}
}

// Run executes a full backup for the specified groups (or all if none specified).
func (e *Engine) Run(ctx context.Context, opts BackupOptions, groups ...string) error {
	if err := e.acquireLock(); err != nil {
		return err
	}
	defer e.releaseLock()

	// Run pre-backup hooks
	if err := hooks.RunHooks(ctx, e.cfg.Hooks, "pre-backup"); err != nil {
		return fmt.Errorf("pre-backup hooks: %w", err)
	}

	// Determine which groups to back up
	backupGroups := e.cfg.Backups
	if len(groups) > 0 {
		backupGroups = filterGroups(e.cfg.Backups, groups)
	}

	// Update lock file with active group names
	if err := e.writeLockGroups(backupGroups); err != nil {
		e.logger.Warn("failed to update lock file with group names", "error", err)
	}

	// Run groups with limited concurrency
	var wg sync.WaitGroup
	sem := make(chan struct{}, e.maxWorkers)
	errs := make([]error, len(backupGroups))

	for i, bg := range backupGroups {
		wg.Add(1)
		go func(idx int, group config.BackupGroup) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			snap, err := e.RunGroup(ctx, group, opts)
			duration := time.Since(start)

			if err != nil {
				errs[idx] = err
				e.logger.Error("backup group failed", "group", group.Name, "error", err)
				e.stateDB.RecordBackupRun(group.Name, "", "failure", err.Error(), duration, 0, 0)
			} else if snap != nil {
				e.logger.Info("backup group completed",
					"group", group.Name,
					"snapshot", snap.ID,
					"changed", snap.Stats.ChangedFiles+snap.Stats.AddedFiles,
					"duration", duration,
				)
				e.stateDB.RecordBackupRun(group.Name, snap.ID, "success", "", duration, snap.Stats.ChangedFiles+snap.Stats.AddedFiles, snap.Stats.UploadSize)
			} else {
				// No changes detected — still record a successful run so status tracks last check time
				e.stateDB.RecordBackupRun(group.Name, "", "success", "", duration, 0, 0)
			}
		}(i, bg)
	}
	wg.Wait()

	// Run post-backup hooks
	if hookErr := hooks.RunHooks(ctx, e.cfg.Hooks, "post-backup"); hookErr != nil {
		e.logger.Warn("post-backup hooks failed", "error", hookErr)
	}

	// Collect errors
	var errMsgs []string
	for _, err := range errs {
		if err != nil {
			errMsgs = append(errMsgs, err.Error())
		}
	}
	if len(errMsgs) > 0 {
		return fmt.Errorf("backup errors: %s", strings.Join(errMsgs, "; "))
	}
	return nil
}

// RunGroup backs up a single backup group.
func (e *Engine) RunGroup(ctx context.Context, group config.BackupGroup, opts BackupOptions) (*Snapshot, error) {
	e.logger.Info("starting backup", "group", group.Name)

	progress := opts.OnProgress
	if progress == nil {
		progress = func(string, string, int, int, int64, int64) {}
	}

	// Walk and hash files
	progress(group.Name, "scanning", 0, 0, 0, 0)
	files, err := WalkPaths(group.Paths, group.Exclude)
	if err != nil {
		return nil, fmt.Errorf("walking paths for %s: %w", group.Name, err)
	}

	var totalSize int64
	for _, fi := range files {
		totalSize += fi.Size
	}

	if opts.Verbose {
		e.logger.Info("scan complete", "group", group.Name, "files", len(files), "total_size", totalSize)
	}

	// Get last snapshot for diffing
	lastSnap, err := e.stateDB.GetLastSnapshot(group.Name)
	if err != nil {
		return nil, fmt.Errorf("getting last snapshot for %s: %w", group.Name, err)
	}

	// Build file entries and determine changes
	lastHashes := make(map[string]string)
	if lastSnap != nil {
		for _, f := range lastSnap.Files {
			lastHashes[f.Path] = f.SHA256
		}
	}

	type pendingUpload struct {
		path   string
		hash   string
		size   int64
		idx    int
	}

	var (
		entries      []FileEntry
		uploads      []pendingUpload
		uploadSize   int64
		changedFiles int
		addedFiles   int
	)

	for i, fi := range files {
		status := "unchanged"
		prevHash, existed := lastHashes[fi.Path]

		if !existed {
			status = "added"
			addedFiles++
		} else if prevHash != fi.SHA256 {
			status = "modified"
			changedFiles++
		}

		if opts.Verbose && status != "unchanged" {
			e.logger.Info("file change", "group", group.Name, "status", status, "path", fi.Path, "size", fi.Size)
		}

		mode, _ := strconv.ParseUint(fmt.Sprintf("%o", fi.Mode), 8, 32)

		entry := FileEntry{
			Path:      fi.Path,
			SHA256:    fi.SHA256,
			Size:      fi.Size,
			Mode:      fs.FileMode(mode),
			Encrypted: group.Encrypt,
			Status:    status,
		}
		modTime, _ := time.Parse("2006-01-02T15:04:05Z", fi.ModTime)
		entry.ModTime = modTime
		entries = append(entries, entry)

		if status == "added" || status == "modified" {
			uploads = append(uploads, pendingUpload{
				path: fi.Path,
				hash: fi.SHA256,
				size: fi.Size,
				idx:  i,
			})
			uploadSize += fi.Size
		}
	}

	// Upload changed/added blobs in parallel
	if len(uploads) > 0 {
		uploadSem := make(chan struct{}, e.cfg.General.ParallelUploads)
		uploadErrs := make([]error, len(uploads))
		var uploadWg sync.WaitGroup
		var uploaded atomic.Int64

		for i, u := range uploads {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("backup cancelled: %w", err)
			}
			uploadWg.Add(1)
			go func(idx int, up pendingUpload) {
				defer uploadWg.Done()
				uploadSem <- struct{}{}
				defer func() { <-uploadSem }()

				if err := ctx.Err(); err != nil {
					uploadErrs[idx] = err
					return
				}
				if err := e.uploadBlob(ctx, up.path, up.hash, group.Encrypt); err != nil {
					uploadErrs[idx] = fmt.Errorf("uploading blob %s: %w", up.path, err)
					return
				}
				done := uploaded.Add(up.size)
				progress(group.Name, "uploading", int(uploaded.Load()/1024), int(uploadSize/1024), done, uploadSize)
			}(i, u)
		}
		uploadWg.Wait()

		for _, err := range uploadErrs {
			if err != nil {
				return nil, err
			}
		}
	}

	// Detect deleted files
	deletedFiles := 0
	currentPaths := make(map[string]bool)
	for _, fi := range files {
		currentPaths[fi.Path] = true
	}
	if lastSnap != nil {
		for _, f := range lastSnap.Files {
			if !currentPaths[f.Path] && f.Status != "deleted" {
				entries = append(entries, FileEntry{
					Path:   f.Path,
					SHA256: f.SHA256,
					Status: "deleted",
				})
				deletedFiles++
				if opts.Verbose {
					e.logger.Info("file change", "group", group.Name, "status", "deleted", "path", f.Path)
				}
			}
		}
	}

	// Skip snapshot if nothing changed (unless force is set)
	if changedFiles == 0 && addedFiles == 0 && deletedFiles == 0 {
		if opts.Force {
			e.logger.Info("no changes detected, forcing snapshot", "group", group.Name)
		} else {
			e.logger.Info("no changes detected", "group", group.Name)
			progress(group.Name, "done", len(files), len(files), 0, totalSize)
			return nil, nil
		}
	}

	// Create snapshot
	progress(group.Name, "done", len(files), len(files), uploadSize, totalSize)

	snap := &Snapshot{
		ID:          GenerateSnapshotID(),
		Timestamp:   time.Now().UTC(),
		MachineID:   e.cfg.General.MachineID,
		MachineName: e.cfg.General.MachineName,
		Group:       group.Name,
		Files:       entries,
		Stats: SnapshotStats{
			TotalFiles:   len(files),
			ChangedFiles: changedFiles,
			AddedFiles:   addedFiles,
			DeletedFiles: deletedFiles,
			TotalSize:    totalSize,
			UploadSize:   uploadSize,
		},
	}

	// Upload manifest
	manifestData, err := MarshalSnapshot(snap)
	if err != nil {
		return nil, fmt.Errorf("marshaling snapshot: %w", err)
	}

	// Save to local state first (can be rolled back if manifest upload fails)
	if err := e.stateDB.SaveSnapshot(snap); err != nil {
		return nil, fmt.Errorf("saving snapshot state: %w", err)
	}

	remoteMPath := e.cfg.General.MachineID + "/" + ManifestPath(group.Name, snap.ID)
	if err := e.storage.Upload(ctx, remoteMPath, bytes.NewReader(manifestData)); err != nil {
		// Rollback local state on manifest upload failure
		if delErr := e.stateDB.DeleteSnapshot(snap.ID); delErr != nil {
			e.logger.Error("failed to rollback snapshot after manifest upload failure", "error", delErr)
		}
		return nil, fmt.Errorf("uploading manifest: %w", err)
	}

	return snap, nil
}

// DryRun simulates a backup and returns what would be changed.
func (e *Engine) DryRun(ctx context.Context, groups ...string) (map[string][]FileChange, error) {
	backupGroups := e.cfg.Backups
	if len(groups) > 0 {
		backupGroups = filterGroups(e.cfg.Backups, groups)
	}

	result := make(map[string][]FileChange)

	for _, bg := range backupGroups {
		files, err := WalkPaths(bg.Paths, bg.Exclude)
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", bg.Name, err)
		}

		lastSnap, _ := e.stateDB.GetLastSnapshot(bg.Name)

		// Build a temporary snapshot for diffing
		var newEntries []FileEntry
		for _, fi := range files {
			newEntries = append(newEntries, FileEntry{
				Path:   fi.Path,
				SHA256: fi.SHA256,
				Size:   fi.Size,
			})
		}
		newSnap := &Snapshot{Files: newEntries}

		changes := DiffSnapshots(lastSnap, newSnap)
		if len(changes) > 0 {
			result[bg.Name] = changes
		}
	}

	return result, nil
}

func (e *Engine) uploadBlob(ctx context.Context, filePath, hash string, encrypt bool) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	// Verify file wasn't modified between initial hash and read
	h := sha256.Sum256(data)
	currentHash := hex.EncodeToString(h[:])
	if currentHash != hash {
		return fmt.Errorf("file modified during backup: %s (expected %s, got %s)", filePath, hash, currentHash)
	}

	var reader *bytes.Reader
	blobName := BlobPath(hash)

	if encrypt && e.cfg.Encryption.Enabled && e.cfg.Encryption.Recipient != "" {
		encrypted, err := crypto.Encrypt(bytes.NewReader(data), e.cfg.Encryption.Recipient)
		if err != nil {
			return fmt.Errorf("encrypting: %w", err)
		}
		encData, err := readAll(encrypted)
		if err != nil {
			return fmt.Errorf("reading encrypted data: %w", err)
		}
		reader = bytes.NewReader(encData)
		blobName += ".age"
	} else {
		reader = bytes.NewReader(data)
	}

	remotePath := e.cfg.General.MachineID + "/" + blobName
	return e.storage.Upload(ctx, remotePath, reader)
}

func (e *Engine) acquireLock() error {
	lockPath := platform.LockFilePath()
	if err := platform.EnsureDir(platform.DataDir()); err != nil {
		return err
	}

	// Attempt atomic lock creation with O_CREATE|O_EXCL
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		// Lock created successfully, write our PID
		_, writeErr := fmt.Fprintf(f, "%d", os.Getpid())
		f.Close()
		if writeErr != nil {
			os.Remove(lockPath)
			return fmt.Errorf("writing lock file: %w", writeErr)
		}
		return nil
	}

	if !os.IsExist(err) {
		return fmt.Errorf("creating lock file: %w", err)
	}

	// Lock file exists — check if the owning process is still alive
	data, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		return platform.ErrLockAcquire
	}

	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if parseErr != nil || pid <= 0 {
		// Corrupt lock file, remove and retry once
		os.Remove(lockPath)
		return e.acquireLockOnce()
	}

	// Use kill(pid, 0) to portably check if process exists (works on all Unix)
	if err := syscall.Kill(pid, 0); err == nil {
		// Process is still running
		return platform.ErrLockAcquire
	}

	// Stale lock — process no longer exists. Remove and retry.
	e.logger.Info("removing stale lock file", "pid", pid)
	os.Remove(lockPath)
	return e.acquireLockOnce()
}

// writeLockGroups updates the lock file to include the names of active backup groups.
func (e *Engine) writeLockGroups(groups []config.BackupGroup) error {
	lockPath := platform.LockFilePath()
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d\n", os.Getpid())
	for _, g := range groups {
		fmt.Fprintf(&buf, "%s\n", g.Name)
	}
	return os.WriteFile(lockPath, buf.Bytes(), 0o600)
}

// ReadLockStatus checks whether a backup is currently running and returns the
// active group names. Returns running=false if no lock exists or the lock is stale.
func ReadLockStatus() (running bool, groups []string) {
	data, err := os.ReadFile(platform.LockFilePath())
	if err != nil {
		return false, nil
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return false, nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || pid <= 0 {
		return false, nil
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false, nil
	}
	if len(lines) > 1 {
		groups = lines[1:]
	}
	return true, groups
}

// acquireLockOnce makes a single atomic attempt to create the lock file.
func (e *Engine) acquireLockOnce() error {
	lockPath := platform.LockFilePath()
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return platform.ErrLockAcquire
	}
	_, writeErr := fmt.Fprintf(f, "%d", os.Getpid())
	f.Close()
	if writeErr != nil {
		os.Remove(lockPath)
		return fmt.Errorf("writing lock file: %w", writeErr)
	}
	return nil
}

func (e *Engine) releaseLock() {
	if err := os.Remove(platform.LockFilePath()); err != nil && !os.IsNotExist(err) {
		e.logger.Warn("failed to remove lock file", "error", err)
	}
}

// GarbageCollect removes expired snapshots and orphaned blobs.
func (e *Engine) GarbageCollect(ctx context.Context) error {
	for _, bg := range e.cfg.Backups {
		snapshots, err := e.stateDB.ListSnapshots(bg.Name)
		if err != nil {
			return fmt.Errorf("listing snapshots for %s: %w", bg.Name, err)
		}

		cutoffTime := time.Now().AddDate(0, 0, -e.cfg.Versioning.RetentionDays)
		maxCount := e.cfg.Versioning.RetentionCount

		for i, snap := range snapshots {
			keep := false

			// Keep if within retention count
			if maxCount <= 0 || i < maxCount {
				keep = true
			}

			// Keep if within retention days
			if snap.Timestamp.After(cutoffTime) {
				keep = true
			}

			if !keep {
				e.logger.Info("deleting expired snapshot", "id", snap.ID, "group", bg.Name)
				// Delete remote manifest
				remotePath := e.cfg.General.MachineID + "/" + ManifestPath(bg.Name, snap.ID)
				e.storage.Delete(ctx, remotePath)
				// Delete from state DB
				e.stateDB.DeleteSnapshot(snap.ID)
			}
		}
	}

	// TODO: orphan blob cleanup (scan all manifests, collect hashes, delete unlisted blobs)
	return nil
}

func filterGroups(all []config.BackupGroup, names []string) []config.BackupGroup {
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	var filtered []config.BackupGroup
	for _, bg := range all {
		if nameSet[bg.Name] {
			filtered = append(filtered, bg)
		}
	}
	return filtered
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}
