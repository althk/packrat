package storage

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestMockBackendRoundTrip(t *testing.T) {
	ctx := context.Background()
	m := NewMockBackend()

	// Upload
	content := "hello, packrat!"
	if err := m.Upload(ctx, "test/file.txt", strings.NewReader(content)); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// Exists
	ok, err := m.Exists(ctx, "test/file.txt")
	if err != nil || !ok {
		t.Fatalf("Exists: got %v, %v", ok, err)
	}

	// Download
	var buf bytes.Buffer
	if err := m.Download(ctx, "test/file.txt", &buf); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if buf.String() != content {
		t.Errorf("Download content = %q, want %q", buf.String(), content)
	}

	// List
	entries, err := m.List(ctx, "test/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List len = %d, want 1", len(entries))
	}

	// Delete
	if err := m.Delete(ctx, "test/file.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ok, err = m.Exists(ctx, "test/file.txt")
	if err != nil || ok {
		t.Fatalf("after delete, Exists: got %v, %v", ok, err)
	}
}

func TestLocalBackendRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	l := NewLocalBackend(dir)

	content := "local backend test"
	if err := l.Upload(ctx, "subdir/test.txt", strings.NewReader(content)); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	ok, err := l.Exists(ctx, "subdir/test.txt")
	if err != nil || !ok {
		t.Fatalf("Exists: %v, %v", ok, err)
	}

	var buf bytes.Buffer
	if err := l.Download(ctx, "subdir/test.txt", &buf); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if buf.String() != content {
		t.Errorf("content = %q, want %q", buf.String(), content)
	}

	entries, err := l.List(ctx, "subdir")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 { // dir + file
		t.Errorf("List len = %d, want 2", len(entries))
	}

	if err := l.Delete(ctx, "subdir/test.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ok, _ = l.Exists(ctx, "subdir/test.txt")
	if ok {
		t.Error("file should not exist after delete")
	}
}

func TestMockBackendDownloadMissing(t *testing.T) {
	m := NewMockBackend()
	var buf bytes.Buffer
	err := m.Download(context.Background(), "nonexistent", &buf)
	if err == nil {
		t.Error("expected error downloading nonexistent file")
	}
}

func TestNewRcloneBackend(t *testing.T) {
	r := NewRcloneBackend("gdrive", "backups", "1M")
	if r.remote != "gdrive" {
		t.Errorf("remote = %q, want gdrive", r.remote)
	}
	if r.basePath != "backups" {
		t.Errorf("basePath = %q, want backups", r.basePath)
	}
	if r.bandwidthLimit != "1M" {
		t.Errorf("bandwidthLimit = %q, want 1M", r.bandwidthLimit)
	}
}
