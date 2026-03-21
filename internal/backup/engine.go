package backup

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/crypto"
	"github.com/harish/packrat/internal/hooks"
	"github.com/harish/packrat/internal/platform"
	"github.com/harish/packrat/internal/storage"
)

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
	return &Engine{
		cfg:        cfg,
		storage:    store,
		stateDB:    stateDB,
		logger:     slog.Default(),
		maxWorkers: 2,
	}
}

// Run executes a full backup for the specified groups (or all if none specified).
func (e *Engine) Run(ctx context.Context, groups ...string) error {
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
			snap, err := e.RunGroup(ctx, group)
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
func (e *Engine) RunGroup(ctx context.Context, group config.BackupGroup) (*Snapshot, error) {
	e.logger.Info("starting backup", "group", group.Name)

	// Walk and hash files
	files, err := WalkPaths(group.Paths, group.Exclude)
	if err != nil {
		return nil, fmt.Errorf("walking paths for %s: %w", group.Name, err)
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

	var (
		entries      []FileEntry
		uploadSize   int64
		changedFiles int
		addedFiles   int
	)

	for _, fi := range files {
		status := "unchanged"
		prevHash, existed := lastHashes[fi.Path]

		if !existed {
			status = "added"
			addedFiles++
		} else if prevHash != fi.SHA256 {
			status = "modified"
			changedFiles++
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

		// Upload changed/added blobs
		if status == "added" || status == "modified" {
			if err := e.uploadBlob(ctx, fi.Path, fi.SHA256, group.Encrypt); err != nil {
				return nil, fmt.Errorf("uploading blob %s: %w", fi.Path, err)
			}
			uploadSize += fi.Size
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
			}
		}
	}

	// Skip snapshot if nothing changed
	if changedFiles == 0 && addedFiles == 0 && deletedFiles == 0 {
		e.logger.Info("no changes detected", "group", group.Name)
		return nil, nil
	}

	// Create snapshot
	var totalSize int64
	for _, e := range entries {
		totalSize += e.Size
	}

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

	remoteMPath := e.cfg.General.MachineID + "/" + ManifestPath(group.Name, snap.ID)
	if err := e.storage.Upload(ctx, remoteMPath, bytes.NewReader(manifestData)); err != nil {
		return nil, fmt.Errorf("uploading manifest: %w", err)
	}

	// Save to local state
	if err := e.stateDB.SaveSnapshot(snap); err != nil {
		return nil, fmt.Errorf("saving snapshot state: %w", err)
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

	// Check for existing lock
	if data, err := os.ReadFile(lockPath); err == nil {
		pid := strings.TrimSpace(string(data))
		// Check if process is still running
		if _, err := os.FindProcess(mustAtoi(pid)); err == nil {
			// Check /proc to verify process exists (Linux-specific)
			if _, err := os.Stat(fmt.Sprintf("/proc/%s", pid)); err == nil {
				return platform.ErrLockAcquire
			}
		}
		// Stale lock, remove it
		os.Remove(lockPath)
	}

	return os.WriteFile(lockPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
}

func (e *Engine) releaseLock() {
	os.Remove(platform.LockFilePath())
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

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}
