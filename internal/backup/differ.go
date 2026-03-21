package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	difflib "github.com/sergi/go-diff/diffmatchpatch"
)

// FileInfo holds metadata about a local file.
type FileInfo struct {
	Path    string
	SHA256  string
	Size    int64
	Mode    fs.FileMode
	ModTime string
}

// ComputeFileHash computes the SHA-256 hash of a file.
func ComputeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing file %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// WalkPaths walks the given paths, applying exclude patterns, and returns file info.
func WalkPaths(paths []string, excludes []string) ([]FileInfo, error) {
	var files []FileInfo
	seen := make(map[string]bool)

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}

		if info.IsDir() {
			err := filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return nil // skip inaccessible files
				}
				if fi.IsDir() {
					// Check if directory should be excluded
					if shouldExclude(path, excludes) {
						return filepath.SkipDir
					}
					return nil
				}
				if shouldExclude(path, excludes) {
					return nil
				}
				if seen[path] {
					return nil
				}
				seen[path] = true

				hash, err := ComputeFileHash(path)
				if err != nil {
					return nil // skip unhashable files
				}
				files = append(files, FileInfo{
					Path:    path,
					SHA256:  hash,
					Size:    fi.Size(),
					Mode:    fi.Mode(),
					ModTime: fi.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
				})
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("walking %s: %w", p, err)
			}
		} else {
			if shouldExclude(p, excludes) {
				continue
			}
			if seen[p] {
				continue
			}
			seen[p] = true

			hash, err := ComputeFileHash(p)
			if err != nil {
				continue
			}
			files = append(files, FileInfo{
				Path:    p,
				SHA256:  hash,
				Size:    info.Size(),
				Mode:    info.Mode(),
				ModTime: info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
			})
		}
	}

	return files, nil
}

// shouldExclude checks if a path matches any exclude patterns.
func shouldExclude(path string, excludes []string) bool {
	base := filepath.Base(path)
	for _, pattern := range excludes {
		// Try matching against base name
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Try matching against full path
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Check suffix for directory patterns like "workspaceStorage/"
		if strings.HasSuffix(pattern, "/") {
			dirName := strings.TrimSuffix(pattern, "/")
			if base == dirName || strings.Contains(path, "/"+dirName+"/") {
				return true
			}
		}
	}
	return false
}

// FileChange represents a change between two snapshots.
type FileChange struct {
	Path     string
	OldEntry *FileEntry
	NewEntry *FileEntry
	Status   string // "added", "modified", "deleted", "unchanged"
}

// DiffSnapshots compares two snapshots and returns changes.
func DiffSnapshots(old, new *Snapshot) []FileChange {
	oldMap := make(map[string]*FileEntry)
	if old != nil {
		for i := range old.Files {
			oldMap[old.Files[i].Path] = &old.Files[i]
		}
	}

	newMap := make(map[string]*FileEntry)
	if new != nil {
		for i := range new.Files {
			newMap[new.Files[i].Path] = &new.Files[i]
		}
	}

	var changes []FileChange

	// Check for modified and deleted
	for path, oldEntry := range oldMap {
		if newEntry, ok := newMap[path]; ok {
			if oldEntry.SHA256 != newEntry.SHA256 {
				changes = append(changes, FileChange{
					Path: path, OldEntry: oldEntry, NewEntry: newEntry, Status: "modified",
				})
			}
		} else {
			changes = append(changes, FileChange{
				Path: path, OldEntry: oldEntry, Status: "deleted",
			})
		}
	}

	// Check for added
	for path, newEntry := range newMap {
		if _, ok := oldMap[path]; !ok {
			changes = append(changes, FileChange{
				Path: path, NewEntry: newEntry, Status: "added",
			})
		}
	}

	return changes
}

// ContentDiff returns a unified diff between two text strings.
func ContentDiff(oldContent, newContent string) string {
	dmp := difflib.New()
	diffs := dmp.DiffMain(oldContent, newContent, true)
	return dmp.DiffPrettyText(diffs)
}

// UnifiedDiff returns a unified diff format string.
func UnifiedDiff(oldContent, newContent, oldName, newName string) string {
	dmp := difflib.New()
	diffs := dmp.DiffMain(oldContent, newContent, true)
	patches := dmp.PatchMake(diffs)
	return dmp.PatchToText(patches)
}
