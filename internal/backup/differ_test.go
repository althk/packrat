package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFileHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	hash, err := ComputeFileHash(path)
	if err != nil {
		t.Fatalf("ComputeFileHash: %v", err)
	}
	// SHA-256 of "hello world"
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if hash != expected {
		t.Errorf("hash = %q, want %q", hash, expected)
	}
}

func TestWalkPaths(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.log"), []byte("bbb"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("ccc"), 0o644)

	files, err := WalkPaths([]string{dir}, []string{"*.log"}, nil)
	if err != nil {
		t.Fatalf("WalkPaths: %v", err)
	}

	// Should have a.txt and sub/c.txt (b.log excluded)
	if len(files) != 2 {
		t.Errorf("len = %d, want 2", len(files))
		for _, f := range files {
			t.Logf("  %s", f.Path)
		}
	}
}

func TestWalkPathsMissing(t *testing.T) {
	files, err := WalkPaths([]string{"/nonexistent/path"}, nil, nil)
	if err != nil {
		t.Fatalf("WalkPaths should not error on missing: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestDiffSnapshots(t *testing.T) {
	old := &Snapshot{
		Files: []FileEntry{
			{Path: "a.txt", SHA256: "hash1"},
			{Path: "b.txt", SHA256: "hash2"},
			{Path: "c.txt", SHA256: "hash3"},
		},
	}
	new := &Snapshot{
		Files: []FileEntry{
			{Path: "a.txt", SHA256: "hash1"},   // unchanged
			{Path: "b.txt", SHA256: "hash2b"},  // modified
			{Path: "d.txt", SHA256: "hash4"},   // added
		},
	}

	changes := DiffSnapshots(old, new)

	statusMap := make(map[string]string)
	for _, c := range changes {
		statusMap[c.Path] = c.Status
	}

	if statusMap["b.txt"] != "modified" {
		t.Errorf("b.txt status = %q, want modified", statusMap["b.txt"])
	}
	if statusMap["c.txt"] != "deleted" {
		t.Errorf("c.txt status = %q, want deleted", statusMap["c.txt"])
	}
	if statusMap["d.txt"] != "added" {
		t.Errorf("d.txt status = %q, want added", statusMap["d.txt"])
	}
	if _, ok := statusMap["a.txt"]; ok {
		t.Error("unchanged a.txt should not appear in changes")
	}
}

func TestContentDiff(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nline2modified\nline3\n"
	diff := ContentDiff(old, new)
	if diff == "" {
		t.Error("diff should not be empty")
	}
}

func TestWalkPathsWithInclude(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# readme"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# notes"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "data.json"), []byte("{}"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "deep.md"), []byte("# deep"), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "code.go"), []byte("package sub"), 0o644)

	files, err := WalkPaths([]string{dir}, nil, []string{"*.md"})
	if err != nil {
		t.Fatalf("WalkPaths: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("len = %d, want 3", len(files))
		for _, f := range files {
			t.Logf("  %s", f.Path)
		}
	}

	for _, f := range files {
		if filepath.Ext(f.Path) != ".md" {
			t.Errorf("unexpected non-.md file: %s", f.Path)
		}
	}
}

func TestWalkPathsIncludeAndExclude(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# readme"), 0o644)
	os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# changes"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	// Include only .md, but exclude CHANGELOG.md
	files, err := WalkPaths([]string{dir}, []string{"CHANGELOG.md"}, []string{"*.md"})
	if err != nil {
		t.Fatalf("WalkPaths: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("len = %d, want 1", len(files))
		for _, f := range files {
			t.Logf("  %s", f.Path)
		}
	}
}

func TestShouldInclude(t *testing.T) {
	tests := []struct {
		path     string
		includes []string
		want     bool
	}{
		{"/tmp/readme.md", []string{"*.md"}, true},
		{"/tmp/main.go", []string{"*.md"}, false},
		{"/tmp/test.txt", nil, true},
		{"/tmp/test.txt", []string{}, true},
		{"/tmp/notes.MD", []string{"*.md"}, false}, // case-sensitive
		{"/tmp/file.tar.gz", []string{"*.gz", "*.md"}, true},
	}

	for _, tt := range tests {
		got := shouldInclude(tt.path, tt.includes)
		if got != tt.want {
			t.Errorf("shouldInclude(%q, %v) = %v, want %v", tt.path, tt.includes, got, tt.want)
		}
	}
}

func TestShouldExclude(t *testing.T) {
	tests := []struct {
		path     string
		excludes []string
		want     bool
	}{
		{"/tmp/test.log", []string{"*.log"}, true},
		{"/tmp/test.txt", []string{"*.log"}, false},
		{"/tmp/cache/file", []string{"cache/"}, true},
		{"/tmp/test.cache", []string{"*.cache"}, true},
	}

	for _, tt := range tests {
		got := shouldExclude(tt.path, tt.excludes)
		if got != tt.want {
			t.Errorf("shouldExclude(%q, %v) = %v, want %v", tt.path, tt.excludes, got, tt.want)
		}
	}
}
