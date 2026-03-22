package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsInPATH(t *testing.T) {
	// /usr/bin should always be in PATH
	if !isInPATH("/usr/bin") {
		t.Error("expected /usr/bin to be in PATH")
	}

	if isInPATH("/some/nonexistent/path/that/does/not/exist") {
		t.Error("expected bogus path to not be in PATH")
	}
}

func TestIsWritableDir(t *testing.T) {
	dir := t.TempDir()
	if !isWritableDir(dir) {
		t.Error("expected temp dir to be writable")
	}

	if isWritableDir("/nonexistent/dir") {
		t.Error("expected nonexistent dir to not be writable")
	}
}

func TestFindInstallDir(t *testing.T) {
	dir := findInstallDir()
	if dir == "" {
		t.Fatal("expected findInstallDir to return a non-empty path")
	}
}

func TestExtractRcloneBinaryNotFound(t *testing.T) {
	// Create a zip with no rclone binary
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "empty.zip")

	// Create a minimal valid zip file with no entries
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	// Write the end-of-central-directory record for an empty zip
	// PK\x05\x06 + 18 zero bytes
	eocdr := []byte{0x50, 0x4b, 0x05, 0x06, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if _, err := f.Write(eocdr); err != nil {
		t.Fatal(err)
	}
	f.Close()

	_, err = extractRcloneBinary(zipPath)
	if err == nil {
		t.Error("expected error extracting from zip with no rclone binary")
	}
}

func TestInstallRcloneWithOptionsBadURL(t *testing.T) {
	dir := t.TempDir()
	opts := InstallOptions{
		BaseURL:    "http://127.0.0.1:1", // unreachable
		InstallDir: dir,
	}
	_, err := InstallRcloneWithOptions(os.Stdout, opts)
	if err == nil {
		t.Error("expected error with unreachable URL")
	}
}

func TestInstallRcloneUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		t.Skip("this test only validates unsupported platform error messages")
	}
	_, err := InstallRclone(os.Stdout)
	if err == nil {
		t.Error("expected error on unsupported platform")
	}
}
