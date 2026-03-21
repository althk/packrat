package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LocalBackend implements StorageBackend using the local filesystem.
type LocalBackend struct {
	basePath string
}

// NewLocalBackend creates a new local filesystem storage backend.
func NewLocalBackend(basePath string) *LocalBackend {
	return &LocalBackend{basePath: basePath}
}

func (l *LocalBackend) fullPath(remotePath string) string {
	return filepath.Join(l.basePath, remotePath)
}

func (l *LocalBackend) Upload(_ context.Context, remotePath string, reader io.Reader) error {
	full := l.fullPath(remotePath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	f, err := os.Create(full)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}

func (l *LocalBackend) Download(_ context.Context, remotePath string, writer io.Writer) error {
	f, err := os.Open(l.fullPath(remotePath))
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(writer, f); err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	return nil
}

func (l *LocalBackend) List(_ context.Context, prefix string) ([]RemoteEntry, error) {
	dir := l.fullPath(prefix)
	var entries []RemoteEntry

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		rel, _ := filepath.Rel(l.basePath, path)
		entries = append(entries, RemoteEntry{
			Path:    rel,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return entries, nil
}

func (l *LocalBackend) Delete(_ context.Context, remotePath string) error {
	return os.Remove(l.fullPath(remotePath))
}

func (l *LocalBackend) Exists(_ context.Context, remotePath string) (bool, error) {
	_, err := os.Stat(l.fullPath(remotePath))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// BasePath returns the base path of the local backend (for testing).
func (l *LocalBackend) BasePath() string {
	return l.basePath
}

// Ensure interfaces are satisfied.
var (
	_ StorageBackend = (*LocalBackend)(nil)
	_ StorageBackend = (*MockBackend)(nil)
	_ StorageBackend = (*RcloneBackend)(nil)
)

// dummy use of time to avoid import issues
var _ = time.Now
