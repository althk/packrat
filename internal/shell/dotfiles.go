package shell

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDotfiles returns the list of commonly tracked dotfiles.
func DefaultDotfiles() []string {
	return []string{
		"~/.bashrc",
		"~/.zshrc",
		"~/.bash_profile",
		"~/.profile",
		"~/.aliases",
		"~/.vimrc",
		"~/.tmux.conf",
		"~/.gitconfig",
		"~/.ssh/config",
	}
}

// DiscoverDotfiles checks which default dotfiles actually exist in the given home directory.
func DiscoverDotfiles(home string) []string {
	defaults := DefaultDotfiles()
	var found []string
	for _, df := range defaults {
		// Replace ~ with actual home
		path := df
		if len(path) > 1 && path[:2] == "~/" {
			path = filepath.Join(home, path[2:])
		}
		if _, err := os.Stat(path); err == nil {
			found = append(found, df)
		}
	}
	return found
}

// SymlinkRestore creates symlinks from a dotfiles directory to their original locations.
// For each file in the dotfiles dir, it creates a symlink at the original location
// pointing back to the file in dotfilesDir.
func SymlinkRestore(files []string, dotfilesDir string, home string) error {
	for _, file := range files {
		// Determine target path (where the symlink will be created)
		target := file
		if len(target) > 1 && target[:2] == "~/" {
			target = filepath.Join(home, target[2:])
		}

		// Source path (in the dotfiles directory)
		baseName := filepath.Base(file)
		source := filepath.Join(dotfilesDir, baseName)

		// Ensure source exists
		if _, err := os.Stat(source); os.IsNotExist(err) {
			continue
		}

		// Ensure target directory exists
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", target, err)
		}

		// Remove existing file/symlink at target
		os.Remove(target)

		if err := os.Symlink(source, target); err != nil {
			return fmt.Errorf("creating symlink %s -> %s: %w", target, source, err)
		}
	}
	return nil
}
