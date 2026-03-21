package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde prefix", "~/Documents", filepath.Join(home, "Documents")},
		{"tilde alone", "~", home},
		{"no tilde", "/tmp/test", "/tmp/test"},
		{"relative", "relative/path", "relative/path"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPath(tt.input)
			if got != tt.expected {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExpandPathEnvVar(t *testing.T) {
	t.Setenv("PACKRAT_TEST_VAR", "/custom/path")
	got := ExpandPath("$PACKRAT_TEST_VAR/subdir")
	if got != "/custom/path/subdir" {
		t.Errorf("ExpandPath with env var = %q, want /custom/path/subdir", got)
	}
}

func TestConfigDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := ConfigDir()
	if !strings.HasPrefix(got, home) {
		t.Errorf("ConfigDir() = %q, expected prefix %q", got, home)
	}
	if !strings.HasSuffix(got, filepath.Join(".config", "packrat")) {
		t.Errorf("ConfigDir() = %q, expected suffix .config/packrat", got)
	}
}

func TestConfigDirOverride(t *testing.T) {
	t.Setenv("PACKRAT_CONFIG_DIR", "/custom/config")
	got := ConfigDir()
	if got != "/custom/config" {
		t.Errorf("ConfigDir() = %q, want /custom/config", got)
	}
}

func TestDataDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := DataDir()
	if !strings.HasPrefix(got, home) {
		t.Errorf("DataDir() = %q, expected prefix %q", got, home)
	}
}

func TestDataDirOverride(t *testing.T) {
	t.Setenv("PACKRAT_DATA_DIR", "/custom/data")
	got := DataDir()
	if got != "/custom/data" {
		t.Errorf("DataDir() = %q, want /custom/data", got)
	}
}

func TestPathFunctions(t *testing.T) {
	dataDir := DataDir()
	if LogFilePath() != filepath.Join(dataDir, "packrat.log") {
		t.Error("LogFilePath unexpected")
	}
	if StateDatabasePath() != filepath.Join(dataDir, "state.db") {
		t.Error("StateDatabasePath unexpected")
	}
	if LockFilePath() != filepath.Join(dataDir, "packrat.lock") {
		t.Error("LockFilePath unexpected")
	}
	if DaemonPIDPath() != filepath.Join(dataDir, "daemon.pid") {
		t.Error("DaemonPIDPath unexpected")
	}
}

func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "c")
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}
