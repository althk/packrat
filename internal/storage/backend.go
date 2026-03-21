package storage

import (
	"bytes"
	"context"
	"io"
	"sort"
	"sync"
	"time"
)

// StorageBackend abstracts remote storage operations.
type StorageBackend interface {
	Upload(ctx context.Context, remotePath string, reader io.Reader) error
	Download(ctx context.Context, remotePath string, writer io.Writer) error
	List(ctx context.Context, prefix string) ([]RemoteEntry, error)
	Delete(ctx context.Context, remotePath string) error
	Exists(ctx context.Context, remotePath string) (bool, error)
}

// RemoteEntry represents a file or directory in remote storage.
type RemoteEntry struct {
	Path    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

// MockBackend is an in-memory StorageBackend for testing.
type MockBackend struct {
	mu    sync.RWMutex
	files map[string][]byte
}

// NewMockBackend creates a new in-memory mock storage backend.
func NewMockBackend() *MockBackend {
	return &MockBackend{files: make(map[string][]byte)}
}

func (m *MockBackend) Upload(_ context.Context, remotePath string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[remotePath] = data
	return nil
}

func (m *MockBackend) Download(_ context.Context, remotePath string, writer io.Writer) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.files[remotePath]
	if !ok {
		return io.EOF
	}
	_, err := io.Copy(writer, bytes.NewReader(data))
	return err
}

func (m *MockBackend) List(_ context.Context, prefix string) ([]RemoteEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var entries []RemoteEntry
	for path, data := range m.files {
		if len(prefix) == 0 || len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			entries = append(entries, RemoteEntry{
				Path:    path,
				Size:    int64(len(data)),
				ModTime: time.Now(),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (m *MockBackend) Delete(_ context.Context, remotePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, remotePath)
	return nil
}

func (m *MockBackend) Exists(_ context.Context, remotePath string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[remotePath]
	return ok, nil
}

// GetData returns the raw data for a path (for testing).
func (m *MockBackend) GetData(path string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.files[path]
	return data, ok
}
