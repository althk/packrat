package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath resolves ~ to home directory and expands environment variables.
func ExpandPath(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	return os.ExpandEnv(p)
}

// ConfigDir returns the packrat config directory (~/.config/packrat/).
func ConfigDir() string {
	if dir := os.Getenv("PACKRAT_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "packrat")
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	if p := os.Getenv("PACKRAT_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(ConfigDir(), "config.toml")
}

// DataDir returns the packrat data directory (~/.local/share/packrat/).
func DataDir() string {
	if dir := os.Getenv("PACKRAT_DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "packrat")
}

// LogFilePath returns the path to the packrat log file.
func LogFilePath() string {
	return filepath.Join(DataDir(), "packrat.log")
}

// StateDatabasePath returns the path to the SQLite state database.
func StateDatabasePath() string {
	return filepath.Join(DataDir(), "state.db")
}

// LockFilePath returns the path to the backup lock file.
func LockFilePath() string {
	return filepath.Join(DataDir(), "packrat.lock")
}

// DaemonPIDPath returns the path to the daemon PID file.
func DaemonPIDPath() string {
	return filepath.Join(DataDir(), "daemon.pid")
}

// EnsureDir creates a directory and all parents if they don't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
