package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/harish/packrat/internal/platform"
)

const (
	defaultTimeout = 5 * time.Minute
	maxRetries     = 3
)

// RcloneBinary is the path to the rclone binary. Defaults to "rclone" (found via PATH).
// Set this after auto-install when the binary may not yet be in PATH.
var RcloneBinary = "rclone"

// RcloneBackend implements StorageBackend using rclone CLI.
type RcloneBackend struct {
	remote         string
	basePath       string
	bandwidthLimit string
	timeout        time.Duration
}

// NewRcloneBackend creates a new rclone storage backend.
func NewRcloneBackend(remote, basePath, bandwidthLimit string) *RcloneBackend {
	return &RcloneBackend{
		remote:         remote,
		basePath:       basePath,
		bandwidthLimit: bandwidthLimit,
		timeout:        defaultTimeout,
	}
}

// CheckRcloneInstalled verifies rclone is available and returns its version.
func CheckRcloneInstalled() (string, error) {
	out, err := exec.Command(RcloneBinary, "version").Output()
	if err != nil {
		return "", platform.ErrRcloneNotFound
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "unknown", nil
}

// ValidateRemote checks that the named remote exists in rclone config.
func ValidateRemote(remote string) error {
	out, err := exec.Command(RcloneBinary, "listremotes").Output()
	if err != nil {
		return fmt.Errorf("listing rclone remotes: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSuffix(strings.TrimSpace(line), ":")
		if name == remote {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", platform.ErrRemoteNotFound, remote)
}

// TestConnection uploads and removes a small test file to verify access.
func (r *RcloneBackend) TestConnection(ctx context.Context) error {
	testPath := r.remotePath(".packrat-test")
	if err := r.Upload(ctx, ".packrat-test", strings.NewReader("test")); err != nil {
		return fmt.Errorf("%w: %v", platform.ErrRemoteUnreachable, err)
	}
	return r.Delete(ctx, testPath)
}

func (r *RcloneBackend) remotePath(path string) string {
	return fmt.Sprintf("%s:%s/%s", r.remote, r.basePath, path)
}

func (r *RcloneBackend) baseArgs() []string {
	var args []string
	if r.bandwidthLimit != "" {
		args = append(args, "--bwlimit", r.bandwidthLimit)
	}
	return args
}

func (r *RcloneBackend) Upload(ctx context.Context, remotePath string, reader io.Reader) error {
	return r.withRetry(func() error {
		data, err := io.ReadAll(reader)
		if err != nil {
			return err
		}

		args := append(r.baseArgs(), "rcat", r.remotePath(remotePath))
		cmd := exec.CommandContext(ctx, RcloneBinary, args...)
		cmd.Stdin = bytes.NewReader(data)

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("rclone rcat %s: %s: %w", remotePath, stderr.String(), err)
		}
		return nil
	})
}

func (r *RcloneBackend) Download(ctx context.Context, remotePath string, writer io.Writer) error {
	return r.withRetry(func() error {
		args := append(r.baseArgs(), "cat", r.remotePath(remotePath))
		cmd := exec.CommandContext(ctx, RcloneBinary, args...)

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("rclone cat %s: %s: %w", remotePath, stderr.String(), err)
		}

		_, err := io.Copy(writer, &stdout)
		return err
	})
}

func (r *RcloneBackend) List(ctx context.Context, prefix string) ([]RemoteEntry, error) {
	var entries []RemoteEntry
	err := r.withRetry(func() error {
		args := append(r.baseArgs(), "lsjson", r.remotePath(prefix))
		cmd := exec.CommandContext(ctx, RcloneBinary, args...)

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("rclone lsjson %s: %s: %w", prefix, stderr.String(), err)
		}

		var items []struct {
			Path    string    `json:"Path"`
			Name    string    `json:"Name"`
			Size    int64     `json:"Size"`
			ModTime time.Time `json:"ModTime"`
			IsDir   bool      `json:"IsDir"`
		}

		if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
			return fmt.Errorf("parsing rclone lsjson: %w", err)
		}

		entries = make([]RemoteEntry, len(items))
		for i, item := range items {
			p := item.Path
			if p == "" {
				p = item.Name
			}
			entries[i] = RemoteEntry{
				Path:    prefix + "/" + p,
				Size:    item.Size,
				ModTime: item.ModTime,
				IsDir:   item.IsDir,
			}
		}
		return nil
	})
	return entries, err
}

func (r *RcloneBackend) Delete(ctx context.Context, remotePath string) error {
	return r.withRetry(func() error {
		args := append(r.baseArgs(), "deletefile", r.remotePath(remotePath))
		cmd := exec.CommandContext(ctx, RcloneBinary, args...)

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("rclone deletefile %s: %s: %w", remotePath, stderr.String(), err)
		}
		return nil
	})
}

func (r *RcloneBackend) Exists(ctx context.Context, remotePath string) (bool, error) {
	args := append(r.baseArgs(), "lsjson", r.remotePath(remotePath))
	cmd := exec.CommandContext(ctx, RcloneBinary, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

func (r *RcloneBackend) withRetry(fn func() error) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := fn(); err != nil {
			lastErr = err
			backoff := time.Duration(math.Pow(2, float64(i))) * time.Second
			time.Sleep(backoff)
			continue
		}
		return nil
	}
	return lastErr
}
