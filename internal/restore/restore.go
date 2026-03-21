package restore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/crypto"
	"github.com/harish/packrat/internal/storage"
)

// RestoreOptions configures a restore operation.
type RestoreOptions struct {
	DestDir          string // Alternate restore directory (empty = original locations)
	Force            bool   // Skip conflict checks
	BackupLocalFirst bool   // Backup local files before overwriting
	Yes              bool   // Skip confirmation prompts
}

// Restorer handles restore operations.
type Restorer struct {
	cfg     *config.Config
	storage storage.StorageBackend
	stateDB *backup.StateDB
	logger  *slog.Logger
}

// NewRestorer creates a new restore handler.
func NewRestorer(cfg *config.Config, store storage.StorageBackend, stateDB *backup.StateDB) *Restorer {
	return &Restorer{
		cfg:     cfg,
		storage: store,
		stateDB: stateDB,
		logger:  slog.Default(),
	}
}

// ListSnapshots returns snapshots from the local state DB, optionally filtered by group.
func (r *Restorer) ListSnapshots(group string) ([]*backup.Snapshot, error) {
	return r.stateDB.ListSnapshots(group)
}

// ListRemoteSnapshots fetches snapshot manifests from remote storage.
func (r *Restorer) ListRemoteSnapshots(ctx context.Context, machineID, group string) ([]*backup.Snapshot, error) {
	prefix := machineID + "/manifests/"
	if group != "" {
		prefix += group + "/"
	}

	entries, err := r.storage.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("listing remote snapshots: %w", err)
	}

	var snapshots []*backup.Snapshot
	for _, entry := range entries {
		if entry.IsDir || !strings.HasSuffix(entry.Path, ".json") {
			continue
		}
		var buf bytes.Buffer
		if err := r.storage.Download(ctx, entry.Path, &buf); err != nil {
			r.logger.Warn("failed to download manifest", "path", entry.Path, "error", err)
			continue
		}
		snap, err := backup.UnmarshalSnapshot(buf.Bytes())
		if err != nil {
			r.logger.Warn("failed to parse manifest", "path", entry.Path, "error", err)
			continue
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// RestoreSnapshot restores all files from a snapshot.
func (r *Restorer) RestoreSnapshot(ctx context.Context, snap *backup.Snapshot, opts RestoreOptions) error {
	var files []string
	for _, f := range snap.Files {
		if f.Status != "deleted" {
			files = append(files, f.Path)
		}
	}
	return r.RestoreFiles(ctx, snap, files, opts)
}

// RestoreFiles restores specific files from a snapshot.
func (r *Restorer) RestoreFiles(ctx context.Context, snap *backup.Snapshot, files []string, opts RestoreOptions) error {
	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}

	for _, entry := range snap.Files {
		if !fileSet[entry.Path] || entry.Status == "deleted" {
			continue
		}

		destPath := entry.Path
		if opts.DestDir != "" {
			// Strip leading / for relative path under dest dir
			rel := entry.Path
			if filepath.IsAbs(rel) {
				rel = strings.TrimPrefix(rel, "/")
			}
			destPath = filepath.Join(opts.DestDir, rel)
		}

		if err := r.restoreFile(ctx, snap, entry, destPath, opts); err != nil {
			return fmt.Errorf("restoring %s: %w", entry.Path, err)
		}
	}
	return nil
}

func (r *Restorer) restoreFile(ctx context.Context, snap *backup.Snapshot, entry backup.FileEntry, destPath string, opts RestoreOptions) error {
	r.logger.Info("restoring file", "path", entry.Path, "dest", destPath)

	// Download blob
	blobPath := backup.BlobPath(entry.SHA256)
	if entry.Encrypted {
		blobPath += ".age"
	}
	remotePath := snap.MachineID + "/" + blobPath

	var buf bytes.Buffer
	if err := r.storage.Download(ctx, remotePath, &buf); err != nil {
		return fmt.Errorf("downloading blob: %w", err)
	}

	var content io.Reader = &buf

	// Decrypt if needed
	if entry.Encrypted && r.cfg.Encryption.Enabled {
		identity, err := r.getIdentity()
		if err != nil {
			return fmt.Errorf("getting decryption key: %w", err)
		}
		decrypted, err := crypto.Decrypt(content, identity)
		if err != nil {
			return fmt.Errorf("decrypting: %w", err)
		}
		content = decrypted
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Write file
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("reading content: %w", err)
	}

	mode := entry.Mode
	if mode == 0 {
		mode = 0o644
	}

	if err := os.WriteFile(destPath, data, mode); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

func (r *Restorer) getIdentity() (string, error) {
	switch r.cfg.Encryption.KeySource {
	case "file":
		return crypto.LoadKeyFromFile(r.cfg.Encryption.KeyFile)
	case "keyring":
		return crypto.RetrieveKey()
	case "prompt":
		return "", fmt.Errorf("prompt key source requires interactive input")
	default:
		return crypto.RetrieveKey()
	}
}

// GetSnapshotByID finds a snapshot by ID from the state DB.
func (r *Restorer) GetSnapshotByID(id string) (*backup.Snapshot, error) {
	return r.stateDB.GetSnapshotByID(id)
}

// GetLatestSnapshot returns the most recent snapshot for a group.
func (r *Restorer) GetLatestSnapshot(group string) (*backup.Snapshot, error) {
	return r.stateDB.GetLastSnapshot(group)
}
