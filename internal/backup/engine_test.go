package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/storage"
)

func testEngine(t *testing.T) (*Engine, *storage.MockBackend) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	stateDB, err := OpenStateDB(dbPath)
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
		Versioning: config.VersioningConfig{
			RetentionCount: 10,
			RetentionDays:  30,
		},
	}

	engine := NewEngine(cfg, mock, stateDB)
	return engine, mock
}

func TestEngineRunGroup(t *testing.T) {
	engine, mock := testEngine(t)
	ctx := context.Background()

	// Create test files
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("world"), 0o644)

	group := config.BackupGroup{
		Name:    "test",
		Paths:   []string{dir},
		Exclude: []string{},
	}

	snap, err := engine.RunGroup(ctx, group, BackupOptions{})
	if err != nil {
		t.Fatalf("RunGroup: %v", err)
	}
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.Stats.AddedFiles != 2 {
		t.Errorf("AddedFiles = %d, want 2", snap.Stats.AddedFiles)
	}

	// Verify blobs were uploaded
	entries, _ := mock.List(ctx, "")
	if len(entries) < 2 {
		t.Errorf("expected at least 2 uploads (blobs + manifest), got %d", len(entries))
	}

	// Run again without changes — should return nil (no changes)
	snap2, err := engine.RunGroup(ctx, group, BackupOptions{})
	if err != nil {
		t.Fatalf("RunGroup second time: %v", err)
	}
	if snap2 != nil {
		t.Error("expected nil snapshot (no changes)")
	}

	// Modify a file and re-run
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("hello modified"), 0o644)
	snap3, err := engine.RunGroup(ctx, group, BackupOptions{})
	if err != nil {
		t.Fatalf("RunGroup third time: %v", err)
	}
	if snap3 == nil {
		t.Fatal("expected snapshot after modification")
	}
	if snap3.Stats.ChangedFiles != 1 {
		t.Errorf("ChangedFiles = %d, want 1", snap3.Stats.ChangedFiles)
	}
}

func TestEngineParallelUploads(t *testing.T) {
	engine, mock := testEngine(t)
	engine.cfg.General.ParallelUploads = 3
	ctx := context.Background()

	// Create enough files to exercise parallel upload paths
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d.txt", i)), []byte(fmt.Sprintf("content-%d", i)), 0o644)
	}

	group := config.BackupGroup{
		Name:  "parallel-test",
		Paths: []string{dir},
	}

	snap, err := engine.RunGroup(ctx, group, BackupOptions{})
	if err != nil {
		t.Fatalf("RunGroup: %v", err)
	}
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.Stats.AddedFiles != 10 {
		t.Errorf("AddedFiles = %d, want 10", snap.Stats.AddedFiles)
	}

	// Verify all 10 blobs + 1 manifest were uploaded
	entries, _ := mock.List(ctx, "")
	if len(entries) != 11 {
		t.Errorf("expected 11 uploads (10 blobs + 1 manifest), got %d", len(entries))
	}
}

func TestEngineDryRun(t *testing.T) {
	engine, _ := testEngine(t)
	ctx := context.Background()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("test"), 0o644)

	engine.cfg.Backups = []config.BackupGroup{
		{Name: "test", Paths: []string{dir}},
	}

	changes, err := engine.DryRun(ctx)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if len(changes["test"]) != 1 {
		t.Errorf("expected 1 change, got %d", len(changes["test"]))
	}
}

func TestEngineGarbageCollect(t *testing.T) {
	engine, _ := testEngine(t)
	ctx := context.Background()

	engine.cfg.Backups = []config.BackupGroup{
		{Name: "test", Paths: []string{t.TempDir()}},
	}
	engine.cfg.Versioning.RetentionCount = 1

	// GC should not error on empty state
	if err := engine.GarbageCollect(ctx); err != nil {
		t.Fatalf("GarbageCollect: %v", err)
	}
}

func TestReadLockStatus(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("PACKRAT_DATA_DIR", dataDir)

	// No lock file → not running
	running, groups := ReadLockStatus()
	if running {
		t.Fatal("expected not running when no lock file exists")
	}
	if len(groups) != 0 {
		t.Fatal("expected no groups")
	}

	// Lock file with our own PID and group names → running
	lockPath := filepath.Join(dataDir, "packrat.lock")
	content := fmt.Sprintf("%d\nshell-history\ndotfiles\n", os.Getpid())
	if err := os.WriteFile(lockPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing lock file: %v", err)
	}

	running, groups = ReadLockStatus()
	if !running {
		t.Fatal("expected running when lock file has live PID")
	}
	if len(groups) != 2 || groups[0] != "shell-history" || groups[1] != "dotfiles" {
		t.Fatalf("unexpected groups: %v", groups)
	}

	// Lock file with dead PID → not running
	if err := os.WriteFile(lockPath, []byte("999999999\nshell-history\n"), 0o600); err != nil {
		t.Fatalf("writing lock file: %v", err)
	}

	running, _ = ReadLockStatus()
	if running {
		t.Fatal("expected not running for dead PID")
	}
}
