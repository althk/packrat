package restore

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/harish/packrat/internal/backup"
)

// ConflictStatus represents the state of a local file relative to a snapshot.
type ConflictStatus string

const (
	ConflictClean    ConflictStatus = "clean"    // Local matches a known snapshot
	ConflictDiverged ConflictStatus = "diverged" // Local has changes not in any snapshot
	ConflictMissing  ConflictStatus = "missing"  // File doesn't exist locally
)

// ConflictAction represents how to resolve a conflict.
type ConflictAction string

const (
	ActionBackup    ConflictAction = "backup"    // Backup local first, then overwrite
	ActionOverwrite ConflictAction = "overwrite" // Overwrite local without backup
	ActionSkip      ConflictAction = "skip"      // Skip this file
)

// Conflict represents a conflict between a snapshot file and a local file.
type Conflict struct {
	Path         string
	LocalHash    string
	SnapshotHash string
	Status       ConflictStatus
	Action       ConflictAction
}

// DetectConflicts checks each file in the snapshot against local state.
func DetectConflicts(snap *backup.Snapshot) ([]Conflict, error) {
	var conflicts []Conflict

	for _, entry := range snap.Files {
		if entry.Status == "deleted" {
			continue
		}

		c := Conflict{
			Path:         entry.Path,
			SnapshotHash: entry.SHA256,
		}

		_, err := os.Stat(entry.Path)
		if os.IsNotExist(err) {
			c.Status = ConflictMissing
			c.Action = ActionOverwrite // Safe to create
			conflicts = append(conflicts, c)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", entry.Path, err)
		}

		// File exists locally — compute hash
		localHash, err := backup.ComputeFileHash(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("hashing %s: %w", entry.Path, err)
		}
		c.LocalHash = localHash

		if localHash == entry.SHA256 {
			c.Status = ConflictClean
			c.Action = ActionOverwrite // Same content, safe
		} else {
			c.Status = ConflictDiverged
			c.Action = ActionSkip // Default to safe action; user can override
		}

		conflicts = append(conflicts, c)
	}

	return conflicts, nil
}

// BackupLocalFile creates a backup of a local file before overwriting.
func BackupLocalFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	backupPath := path + ".packrat-backup-" + time.Now().Format("20060102-150405")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(backupPath, data, 0o644)
}
