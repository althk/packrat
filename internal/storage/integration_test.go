//go:build integration

package storage

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLocalBackendIntegration(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := NewLocalBackend(dir)

	// Upload multiple files
	files := map[string]string{
		"dir1/file1.txt":     "content1",
		"dir1/file2.txt":     "content2",
		"dir2/subdir/f3.txt": "content3",
	}

	for path, content := range files {
		if err := store.Upload(ctx, path, strings.NewReader(content)); err != nil {
			t.Fatalf("Upload %s: %v", path, err)
		}
	}

	// Verify all exist
	for path := range files {
		exists, err := store.Exists(ctx, path)
		if err != nil || !exists {
			t.Errorf("file %s should exist", path)
		}
	}

	// Download and verify
	for path, expected := range files {
		var buf bytes.Buffer
		if err := store.Download(ctx, path, &buf); err != nil {
			t.Fatalf("Download %s: %v", path, err)
		}
		if buf.String() != expected {
			t.Errorf("content of %s = %q, want %q", path, buf.String(), expected)
		}
	}

	// List
	entries, err := store.List(ctx, "dir1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should have dir1, file1.txt, file2.txt
	if len(entries) < 2 {
		t.Errorf("List dir1: got %d entries, want >= 2", len(entries))
	}

	// Delete and verify
	if err := store.Delete(ctx, "dir1/file1.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	exists, _ := store.Exists(ctx, "dir1/file1.txt")
	if exists {
		t.Error("file should not exist after delete")
	}
}
