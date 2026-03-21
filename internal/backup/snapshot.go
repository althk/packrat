package backup

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"time"
)

// Snapshot represents a point-in-time backup of a group.
type Snapshot struct {
	ID          string        `json:"id"`
	Timestamp   time.Time     `json:"timestamp"`
	MachineID   string        `json:"machine_id"`
	MachineName string        `json:"machine_name"`
	Group       string        `json:"group"`
	Files       []FileEntry   `json:"files"`
	Stats       SnapshotStats `json:"stats"`
}

// FileEntry represents a file within a snapshot.
type FileEntry struct {
	Path      string      `json:"path"`
	SHA256    string      `json:"sha256"`
	Size      int64       `json:"size"`
	Mode      fs.FileMode `json:"mode"`
	ModTime   time.Time   `json:"mod_time"`
	Encrypted bool        `json:"encrypted"`
	Status    string      `json:"status"` // "added", "modified", "deleted", "unchanged"
}

// SnapshotStats contains summary statistics for a snapshot.
type SnapshotStats struct {
	TotalFiles   int   `json:"total_files"`
	ChangedFiles int   `json:"changed_files"`
	AddedFiles   int   `json:"added_files"`
	DeletedFiles int   `json:"deleted_files"`
	TotalSize    int64 `json:"total_size"`
	UploadSize   int64 `json:"upload_size"`
}

// GenerateSnapshotID creates a new snapshot ID in the format snap-YYYYMMDD-HHMMSS-<4hex>.
func GenerateSnapshotID() string {
	now := time.Now().UTC()
	b := make([]byte, 2)
	rand.Read(b)
	return fmt.Sprintf("snap-%s-%02x%02x", now.Format("20060102-150405"), b[0], b[1])
}

// MarshalSnapshot serializes a snapshot to JSON.
func MarshalSnapshot(s *Snapshot) ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// UnmarshalSnapshot deserializes a snapshot from JSON.
func UnmarshalSnapshot(data []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshaling snapshot: %w", err)
	}
	return &s, nil
}

// BlobPath returns the content-addressable storage path for a file hash.
// Uses 2-level directory: first 2 chars / rest.
func BlobPath(sha256 string) string {
	if len(sha256) < 2 {
		return sha256
	}
	return fmt.Sprintf("blobs/%s/%s", sha256[:2], sha256[2:])
}

// ManifestPath returns the remote path for a snapshot manifest.
func ManifestPath(group, snapshotID string) string {
	return fmt.Sprintf("manifests/%s/%s.json", group, snapshotID)
}
