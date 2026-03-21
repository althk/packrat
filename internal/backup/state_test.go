package backup

import (
	"path/filepath"
	"testing"
	"time"
)

func testDB(t *testing.T) *StateDB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenStateDB(path)
	if err != nil {
		t.Fatalf("OpenStateDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStateDBSaveAndGetSnapshot(t *testing.T) {
	db := testDB(t)

	snap := &Snapshot{
		ID:        "snap-20240315-143022-a1b2",
		Timestamp: time.Now().UTC(),
		MachineID: "test123",
		Group:     "dotfiles",
		Files: []FileEntry{
			{Path: "/home/user/.bashrc", SHA256: "abc123", Status: "added"},
		},
	}

	if err := db.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	loaded, err := db.GetLastSnapshot("dotfiles")
	if err != nil {
		t.Fatalf("GetLastSnapshot: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if loaded.ID != snap.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, snap.ID)
	}
}

func TestStateDBListSnapshots(t *testing.T) {
	db := testDB(t)

	for i := 0; i < 3; i++ {
		snap := &Snapshot{
			ID:        GenerateSnapshotID(),
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Hour),
			MachineID: "test",
			Group:     "dotfiles",
		}
		db.SaveSnapshot(snap)
	}

	snaps, err := db.ListSnapshots("dotfiles")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 3 {
		t.Errorf("len = %d, want 3", len(snaps))
	}
}

func TestStateDBRecordBackupRun(t *testing.T) {
	db := testDB(t)

	err := db.RecordBackupRun("dotfiles", "snap-123", "success", "", 5*time.Second, 3, 1024)
	if err != nil {
		t.Fatalf("RecordBackupRun: %v", err)
	}

	history, err := db.GetBackupHistory("dotfiles", 10)
	if err != nil {
		t.Fatalf("GetBackupHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0].Status != "success" {
		t.Errorf("status = %q, want success", history[0].Status)
	}
}

func TestStateDBGetLastBackupTime(t *testing.T) {
	db := testDB(t)

	// No backups yet
	ts, err := db.GetLastBackupTime("dotfiles")
	if err != nil {
		t.Fatalf("GetLastBackupTime: %v", err)
	}
	if !ts.IsZero() {
		t.Errorf("expected zero time, got %v", ts)
	}

	// Add a backup
	db.RecordBackupRun("dotfiles", "snap-1", "success", "", time.Second, 1, 100)

	ts, err = db.GetLastBackupTime("dotfiles")
	if err != nil {
		t.Fatalf("GetLastBackupTime: %v", err)
	}
	if ts.IsZero() {
		t.Error("expected non-zero time")
	}
}

func TestStateDBGetNonexistentSnapshot(t *testing.T) {
	db := testDB(t)

	snap, err := db.GetLastSnapshot("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap != nil {
		t.Error("expected nil snapshot")
	}
}
