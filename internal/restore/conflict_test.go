package restore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/harish/packrat/internal/backup"
)

func TestDetectConflictsMissing(t *testing.T) {
	snap := &backup.Snapshot{
		Files: []backup.FileEntry{
			{Path: "/nonexistent/file.txt", SHA256: "abc123", Status: "added"},
		},
	}

	conflicts, err := DetectConflicts(snap)
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("len = %d, want 1", len(conflicts))
	}
	if conflicts[0].Status != ConflictMissing {
		t.Errorf("status = %q, want missing", conflicts[0].Status)
	}
}

func TestDetectConflictsClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	// SHA-256 of "hello world"
	hash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	snap := &backup.Snapshot{
		Files: []backup.FileEntry{
			{Path: path, SHA256: hash, Status: "unchanged"},
		},
	}

	conflicts, err := DetectConflicts(snap)
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("len = %d, want 1", len(conflicts))
	}
	if conflicts[0].Status != ConflictClean {
		t.Errorf("status = %q, want clean", conflicts[0].Status)
	}
}

func TestDetectConflictsDiverged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	os.WriteFile(path, []byte("local version"), 0o644)

	snap := &backup.Snapshot{
		Files: []backup.FileEntry{
			{Path: path, SHA256: "different-hash", Status: "modified"},
		},
	}

	conflicts, err := DetectConflicts(snap)
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("len = %d, want 1", len(conflicts))
	}
	if conflicts[0].Status != ConflictDiverged {
		t.Errorf("status = %q, want diverged", conflicts[0].Status)
	}
}

func TestBackupLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	os.WriteFile(path, []byte("original"), 0o644)

	if err := BackupLocalFile(path); err != nil {
		t.Fatalf("BackupLocalFile: %v", err)
	}

	// Check backup was created
	entries, _ := os.ReadDir(dir)
	if len(entries) < 2 {
		t.Errorf("expected at least 2 files (original + backup), got %d", len(entries))
	}
}

func TestBackupLocalFileNonexistent(t *testing.T) {
	err := BackupLocalFile("/nonexistent/file.txt")
	if err != nil {
		t.Fatalf("should not error on nonexistent file: %v", err)
	}
}
