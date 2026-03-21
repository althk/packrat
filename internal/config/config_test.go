package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndValidate(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	content := `
[general]
machine_name = "test-machine"
machine_id = "abcd1234"
log_level = "info"
log_file = "/tmp/packrat-test.log"

[scheduler]
enabled = true
default_interval = "1h"

[storage]
backend = "rclone"
rclone_remote = "gdrive"
remote_base_path = "packrat-backups"

[encryption]
enabled = false

[versioning]
strategy = "snapshot"
retention_count = 50
retention_days = 30

[notifications]
enabled = false

[[backup]]
name = "dotfiles"
paths = ["~/.bashrc", "~/.zshrc"]
encrypt = false
interval = "1h"
exclude = []
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.General.MachineID != "abcd1234" {
		t.Errorf("MachineID = %q, want abcd1234", cfg.General.MachineID)
	}
	if cfg.Storage.RcloneRemote != "gdrive" {
		t.Errorf("RcloneRemote = %q, want gdrive", cfg.Storage.RcloneRemote)
	}
	if len(cfg.Backups) != 1 {
		t.Fatalf("Backups len = %d, want 1", len(cfg.Backups))
	}
	if cfg.Backups[0].Name != "dotfiles" {
		t.Errorf("Backup name = %q, want dotfiles", cfg.Backups[0].Name)
	}

	if err := Validate(cfg); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "missing machine_id",
			cfg:  Config{Storage: StorageConfig{RcloneRemote: "x"}},
		},
		{
			name: "missing rclone_remote",
			cfg:  Config{General: GeneralConfig{MachineID: "x"}},
		},
		{
			name: "bad interval",
			cfg: Config{
				General: GeneralConfig{MachineID: "x"},
				Storage: StorageConfig{RcloneRemote: "x"},
				Backups: []BackupGroup{{Name: "test", Paths: []string{"/tmp"}, Interval: "badval"}},
			},
		},
		{
			name: "interval too short",
			cfg: Config{
				General: GeneralConfig{MachineID: "x"},
				Storage: StorageConfig{RcloneRemote: "x"},
				Backups: []BackupGroup{{Name: "test", Paths: []string{"/tmp"}, Interval: "1m"}},
			},
		},
		{
			name: "invalid hook when",
			cfg: Config{
				General: GeneralConfig{MachineID: "x"},
				Storage: StorageConfig{RcloneRemote: "x"},
				Hooks:   []HookConfig{{Name: "h", When: "invalid"}},
			},
		},
		{
			name: "encryption file mode without key_file",
			cfg: Config{
				General:    GeneralConfig{MachineID: "x"},
				Storage:    StorageConfig{RcloneRemote: "x"},
				Encryption: EncryptionConfig{Enabled: true, KeySource: "file"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(&tt.cfg); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	// Override config dir for save
	t.Setenv("PACKRAT_CONFIG_DIR", dir)

	cfg := DefaultConfig()
	cfg.Storage.RcloneRemote = "test-remote"

	if err := SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Storage.RcloneRemote != "test-remote" {
		t.Errorf("RcloneRemote = %q, want test-remote", loaded.Storage.RcloneRemote)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.General.MachineID == "" {
		t.Error("MachineID should be generated")
	}
	if len(cfg.Backups) == 0 {
		t.Error("should have default backup groups")
	}
}

func TestDetectShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	if s := DetectShell(); s != "zsh" {
		t.Errorf("DetectShell = %q, want zsh", s)
	}

	t.Setenv("SHELL", "/usr/bin/fish")
	if s := DetectShell(); s != "fish" {
		t.Errorf("DetectShell = %q, want fish", s)
	}
}

func TestGetInterval(t *testing.T) {
	bg := BackupGroup{Interval: "30m"}
	if d := bg.GetInterval("1h"); d.Minutes() != 30 {
		t.Errorf("GetInterval = %v, want 30m", d)
	}

	bg2 := BackupGroup{}
	if d := bg2.GetInterval("2h"); d.Hours() != 2 {
		t.Errorf("GetInterval fallback = %v, want 2h", d)
	}
}

func TestMigrateConfig(t *testing.T) {
	cfg := DefaultConfig()
	migrated, err := MigrateConfig(cfg)
	if err != nil {
		t.Fatalf("MigrateConfig: %v", err)
	}
	if migrated != cfg {
		t.Error("expected same config pointer for v1 migration")
	}
}
