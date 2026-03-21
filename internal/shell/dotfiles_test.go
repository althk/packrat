package shell

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDotfiles(t *testing.T) {
	df := DefaultDotfiles()
	if len(df) == 0 {
		t.Error("should return default dotfiles")
	}
	// All should start with ~/
	for _, f := range df {
		if f[:2] != "~/" {
			t.Errorf("dotfile %q should start with ~/", f)
		}
	}
}

func TestDiscoverDotfiles(t *testing.T) {
	home := t.TempDir()

	// Create some dotfiles
	os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# bashrc"), 0o644)
	os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[user]"), 0o644)

	found := DiscoverDotfiles(home)
	if len(found) != 2 {
		t.Errorf("found %d dotfiles, want 2", len(found))
	}
}

func TestSymlinkRestore(t *testing.T) {
	home := t.TempDir()
	dotfilesDir := t.TempDir()

	// Create a file in the dotfiles dir
	os.WriteFile(filepath.Join(dotfilesDir, ".bashrc"), []byte("# restored"), 0o644)

	err := SymlinkRestore([]string{"~/.bashrc"}, dotfilesDir, home)
	if err != nil {
		t.Fatalf("SymlinkRestore: %v", err)
	}

	// Check symlink exists
	target := filepath.Join(home, ".bashrc")
	link, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	expected := filepath.Join(dotfilesDir, ".bashrc")
	if link != expected {
		t.Errorf("symlink = %q, want %q", link, expected)
	}
}
