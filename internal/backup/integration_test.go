//go:build integration

package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/storage"
)

func TestFullBackupRestoreCycle(t *testing.T) {
	ctx := context.Background()

	// Setup local backend (simulates rclone :local:)
	remoteDir := t.TempDir()
	store := storage.NewLocalBackend(remoteDir)

	// Setup state DB
	dbPath := filepath.Join(t.TempDir(), "state.db")
	stateDB, err := OpenStateDB(dbPath)
	if err != nil {
		t.Fatalf("OpenStateDB: %v", err)
	}
	defer stateDB.Close()

	// Create test files to back up
	sourceDir := t.TempDir()
	os.WriteFile(filepath.Join(sourceDir, "file1.txt"), []byte("hello world"), 0o644)
	os.WriteFile(filepath.Join(sourceDir, "file2.txt"), []byte("packrat test"), 0o644)
	os.MkdirAll(filepath.Join(sourceDir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(sourceDir, "subdir", "nested.txt"), []byte("nested content"), 0o644)

	cfg := &config.Config{
		General: config.GeneralConfig{
			MachineID:   "integration-test",
			MachineName: "test-machine",
		},
		Versioning: config.VersioningConfig{
			RetentionCount: 10,
			RetentionDays:  30,
		},
		Backups: []config.BackupGroup{
			{
				Name:  "test-files",
				Paths: []string{sourceDir},
			},
		},
	}

	engine := NewEngine(cfg, store, stateDB)

	// Run first backup
	err = engine.Run(ctx, "test-files")
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}

	// Verify snapshot was created
	snap, err := stateDB.GetLastSnapshot("test-files")
	if err != nil || snap == nil {
		t.Fatal("expected snapshot after backup")
	}
	if snap.Stats.AddedFiles != 3 {
		t.Errorf("AddedFiles = %d, want 3", snap.Stats.AddedFiles)
	}

	// Verify blobs exist in remote
	for _, entry := range snap.Files {
		if entry.Status == "deleted" {
			continue
		}
		blobPath := cfg.General.MachineID + "/" + BlobPath(entry.SHA256)
		exists, err := store.Exists(ctx, blobPath)
		if err != nil || !exists {
			t.Errorf("blob missing for %s", entry.Path)
		}
	}

	// Verify manifest exists
	manifestPath := cfg.General.MachineID + "/" + ManifestPath("test-files", snap.ID)
	exists, _ := store.Exists(ctx, manifestPath)
	if !exists {
		t.Error("manifest not found in remote")
	}

	// Verify manifest content is valid
	var manifestBuf bytes.Buffer
	store.Download(ctx, manifestPath, &manifestBuf)
	remotSnap, err := UnmarshalSnapshot(manifestBuf.Bytes())
	if err != nil {
		t.Fatalf("UnmarshalSnapshot: %v", err)
	}
	if remotSnap.ID != snap.ID {
		t.Errorf("remote snapshot ID = %q, want %q", remotSnap.ID, snap.ID)
	}

	// Modify a file and run again
	os.WriteFile(filepath.Join(sourceDir, "file1.txt"), []byte("hello world modified"), 0o644)
	err = engine.Run(ctx, "test-files")
	if err != nil {
		t.Fatalf("second backup: %v", err)
	}

	// List all snapshots and find the newest one (not snap.ID)
	allSnapsBefore, err := stateDB.ListSnapshots("test-files")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(allSnapsBefore) < 2 {
		t.Fatalf("expected at least 2 snapshots, got %d", len(allSnapsBefore))
	}
	// Find the snapshot that isn't the first one
	var snap2 *Snapshot
	for _, s := range allSnapsBefore {
		if s.ID != snap.ID {
			snap2 = s
			break
		}
	}
	if snap2 == nil {
		t.Fatal("could not find second snapshot")
	}
	// The second backup should have either modified or added files
	totalChanged := snap2.Stats.ChangedFiles + snap2.Stats.AddedFiles
	if totalChanged < 1 {
		t.Errorf("ChangedFiles+AddedFiles = %d, want >= 1", totalChanged)
	}

	// Verify history
	history, err := stateDB.GetBackupHistory("test-files", 10)
	if err != nil {
		t.Fatalf("GetBackupHistory: %v", err)
	}
	if len(history) < 2 {
		t.Errorf("history len = %d, want >= 2", len(history))
	}

	// Run GC (should keep both snapshots with high retention)
	err = engine.GarbageCollect(ctx)
	if err != nil {
		t.Fatalf("GarbageCollect: %v", err)
	}

	// All snapshots should still exist
	allSnaps, err := stateDB.ListSnapshots("test-files")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(allSnaps) < 2 {
		t.Errorf("expected at least 2 snapshots, got %d", len(allSnaps))
	}
}
