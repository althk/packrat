package backup

import (
	"strings"
	"testing"
)

func TestGenerateSnapshotID(t *testing.T) {
	id := GenerateSnapshotID()
	if !strings.HasPrefix(id, "snap-") {
		t.Errorf("ID should start with snap-, got %q", id)
	}
	// Format: snap-YYYYMMDD-HHMMSS-XXXX
	parts := strings.Split(id, "-")
	if len(parts) != 4 {
		t.Errorf("ID should have 4 parts, got %d: %q", len(parts), id)
	}
}

func TestMarshalUnmarshalSnapshot(t *testing.T) {
	snap := &Snapshot{
		ID:          "snap-20240315-143022-a1b2",
		MachineID:   "test123",
		MachineName: "test-machine",
		Group:       "dotfiles",
		Files: []FileEntry{
			{Path: "/home/user/.bashrc", SHA256: "abc123", Size: 4096, Status: "modified"},
		},
		Stats: SnapshotStats{TotalFiles: 1, ChangedFiles: 1},
	}

	data, err := MarshalSnapshot(snap)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	loaded, err := UnmarshalSnapshot(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if loaded.ID != snap.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, snap.ID)
	}
	if len(loaded.Files) != 1 {
		t.Fatalf("Files len = %d, want 1", len(loaded.Files))
	}
	if loaded.Files[0].SHA256 != "abc123" {
		t.Errorf("SHA256 = %q, want abc123", loaded.Files[0].SHA256)
	}
}

func TestBlobPath(t *testing.T) {
	path := BlobPath("abc123def456")
	if path != "blobs/ab/c123def456" {
		t.Errorf("BlobPath = %q, want blobs/ab/c123def456", path)
	}
}

func TestManifestPath(t *testing.T) {
	path := ManifestPath("dotfiles", "snap-20240315-143022-a1b2")
	if path != "manifests/dotfiles/snap-20240315-143022-a1b2.json" {
		t.Errorf("ManifestPath = %q", path)
	}
}
