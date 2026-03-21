package restore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/storage"
)

func testRestorer(t *testing.T) (*Restorer, *storage.MockBackend, *backup.StateDB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	stateDB, err := backup.OpenStateDB(dbPath)
	if err != nil {
		t.Fatalf("OpenStateDB: %v", err)
	}
	t.Cleanup(func() { stateDB.Close() })

	mock := storage.NewMockBackend()
	cfg := &config.Config{
		General: config.GeneralConfig{
			MachineID:   "test-machine",
			MachineName: "test",
		},
		Encryption: config.EncryptionConfig{
			Enabled: false,
		},
	}

	return NewRestorer(cfg, mock, stateDB), mock, stateDB
}

func TestRestoreFiles(t *testing.T) {
	r, mock, stateDB := testRestorer(t)
	ctx := context.Background()

	destDir := t.TempDir()
	content := []byte("restored content")

	// Upload a blob to mock storage
	hash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	blobPath := "test-machine/" + backup.BlobPath(hash)
	mock.Upload(ctx, blobPath, bytes.NewReader(content))

	// Create a snapshot
	snap := &backup.Snapshot{
		ID:        "snap-test-001",
		Timestamp: time.Now().UTC(),
		MachineID: "test-machine",
		Group:     "test",
		Files: []backup.FileEntry{
			{
				Path:   "/original/path/file.txt",
				SHA256: hash,
				Size:   int64(len(content)),
				Status: "added",
			},
		},
	}
	stateDB.SaveSnapshot(snap)

	// Restore to destDir
	opts := RestoreOptions{DestDir: destDir}
	if err := r.RestoreSnapshot(ctx, snap, opts); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	// Verify file was restored
	restoredPath := filepath.Join(destDir, "original/path/file.txt")
	data, err := os.ReadFile(restoredPath)
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Errorf("restored content = %q, want %q", data, content)
	}
}

func TestRestoreSkipDeleted(t *testing.T) {
	r, mock, _ := testRestorer(t)
	ctx := context.Background()

	snap := &backup.Snapshot{
		ID:        "snap-test-002",
		MachineID: "test-machine",
		Group:     "test",
		Files: []backup.FileEntry{
			{Path: "/deleted/file.txt", SHA256: "abc", Status: "deleted"},
		},
	}

	destDir := t.TempDir()
	opts := RestoreOptions{DestDir: destDir}

	// Should succeed without trying to download anything
	if err := r.RestoreSnapshot(ctx, snap, opts); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	_ = mock // not used but needed for setup
}

func TestListSnapshots(t *testing.T) {
	r, _, stateDB := testRestorer(t)

	snap := &backup.Snapshot{
		ID:        "snap-test-003",
		Timestamp: time.Now().UTC(),
		MachineID: "test-machine",
		Group:     "dotfiles",
	}
	stateDB.SaveSnapshot(snap)

	snaps, err := r.ListSnapshots("dotfiles")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("len = %d, want 1", len(snaps))
	}
}
