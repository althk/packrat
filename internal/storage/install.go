package storage

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/harish/packrat/internal/platform"
)

const defaultBaseURL = "https://downloads.rclone.org"

// InstallOptions allows overriding defaults for testing.
type InstallOptions struct {
	BaseURL    string // override download URL base
	InstallDir string // override install directory
}

// InstallRclone downloads and installs rclone for the current platform.
// Progress and status messages are written to w.
// Returns the path to the installed binary.
func InstallRclone(w io.Writer) (string, error) {
	return InstallRcloneWithOptions(w, InstallOptions{})
}

// InstallRcloneWithOptions downloads and installs rclone with custom options.
func InstallRcloneWithOptions(w io.Writer, opts InstallOptions) (string, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// rclone uses "osx" instead of "darwin"
	dlOS := osName
	if dlOS == "darwin" {
		dlOS = "osx"
	}

	if dlOS != "linux" && dlOS != "osx" {
		return "", fmt.Errorf("%w: unsupported platform %s/%s", platform.ErrRcloneInstallFailed, osName, arch)
	}
	if arch != "amd64" && arch != "arm64" {
		return "", fmt.Errorf("%w: unsupported architecture %s", platform.ErrRcloneInstallFailed, arch)
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	url := fmt.Sprintf("%s/rclone-current-%s-%s.zip", baseURL, dlOS, arch)
	fmt.Fprintf(w, "  > Downloading rclone from %s...\n", url)

	// Download to temp file
	tmpFile, err := os.CreateTemp("", "rclone-*.zip")
	if err != nil {
		return "", fmt.Errorf("%w: creating temp file: %v", platform.ErrRcloneInstallFailed, err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("%w: downloading: %v", platform.ErrRcloneInstallFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: download returned HTTP %d", platform.ErrRcloneInstallFailed, resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return "", fmt.Errorf("%w: saving download: %v", platform.ErrRcloneInstallFailed, err)
	}
	tmpFile.Close()

	// Extract rclone binary from zip
	binaryData, err := extractRcloneBinary(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("%w: extracting: %v", platform.ErrRcloneInstallFailed, err)
	}

	// Determine install directory
	installDir := opts.InstallDir
	if installDir == "" {
		installDir = findInstallDir()
	}

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("%w: creating install dir %s: %v", platform.ErrRcloneInstallFailed, installDir, err)
	}

	binPath := filepath.Join(installDir, "rclone")

	// Write binary atomically via temp file + rename
	tmpBin, err := os.CreateTemp(installDir, "rclone-*.tmp")
	if err != nil {
		return "", fmt.Errorf("%w: creating temp binary: %v", platform.ErrRcloneInstallFailed, err)
	}
	tmpBinPath := tmpBin.Name()

	if _, err := tmpBin.Write(binaryData); err != nil {
		tmpBin.Close()
		os.Remove(tmpBinPath)
		return "", fmt.Errorf("%w: writing binary: %v", platform.ErrRcloneInstallFailed, err)
	}
	tmpBin.Close()

	if err := os.Chmod(tmpBinPath, 0o755); err != nil {
		os.Remove(tmpBinPath)
		return "", fmt.Errorf("%w: chmod: %v", platform.ErrRcloneInstallFailed, err)
	}

	if err := os.Rename(tmpBinPath, binPath); err != nil {
		os.Remove(tmpBinPath)
		return "", fmt.Errorf("%w: installing binary: %v", platform.ErrRcloneInstallFailed, err)
	}

	// On macOS, clear quarantine flag
	if runtime.GOOS == "darwin" {
		_ = exec.Command("xattr", "-d", "com.apple.quarantine", binPath).Run()
	}

	// Verify the installed binary works
	out, err := exec.Command(binPath, "version").Output()
	if err != nil {
		return "", fmt.Errorf("%w: installed binary failed verification: %v", platform.ErrRcloneInstallFailed, err)
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		fmt.Fprintf(w, "  > Installed %s\n", strings.TrimSpace(lines[0]))
	}

	if !isInPATH(installDir) {
		fmt.Fprintf(w, "  > ⚠️  %s is not in your PATH.\n", installDir)
		fmt.Fprintf(w, "  >    Add it with: export PATH=\"%s:$PATH\"\n", installDir)
	}

	fmt.Fprintf(w, "  > ✓ rclone installed to %s\n", binPath)
	return binPath, nil
}

// extractRcloneBinary reads a zip file and returns the rclone binary contents.
func extractRcloneBinary(zipPath string) ([]byte, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == "rclone" && !f.FileInfo().IsDir() {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("rclone binary not found in zip archive")
}

// findInstallDir returns the first writable directory for installing rclone.
func findInstallDir() string {
	home, _ := os.UserHomeDir()

	candidates := []string{
		filepath.Join(home, ".local", "bin"),
		"/usr/local/bin",
	}

	for _, dir := range candidates {
		if isWritableDir(dir) {
			return dir
		}
		// If the dir doesn't exist but parent is writable, we can create it
		parent := filepath.Dir(dir)
		if isWritableDir(parent) {
			return dir
		}
	}

	// Fallback to packrat's data directory
	return filepath.Join(platform.DataDir(), "bin")
}

// isWritableDir checks if a directory exists and is writable.
func isWritableDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	// Try creating a temp file to check write access
	f, err := os.CreateTemp(dir, ".packrat-write-test-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// isInPATH checks if a directory is in the user's PATH.
func isInPATH(dir string) bool {
	pathEnv := os.Getenv("PATH")
	for _, p := range filepath.SplitList(pathEnv) {
		if p == dir {
			return true
		}
	}
	return false
}
