package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/harish/packrat/internal/platform"
)

type Config struct {
	General       GeneralConfig      `toml:"general"`
	Scheduler     SchedulerConfig    `toml:"scheduler"`
	Storage       StorageConfig      `toml:"storage"`
	Encryption    EncryptionConfig   `toml:"encryption"`
	Versioning    VersioningConfig   `toml:"versioning"`
	Notifications NotificationConfig `toml:"notifications"`
	Backups       []BackupGroup      `toml:"backup"`
	Hooks         []HookConfig       `toml:"hook"`
}

type GeneralConfig struct {
	MachineName     string `toml:"machine_name"`
	MachineID       string `toml:"machine_id"`
	LogLevel        string `toml:"log_level"`
	LogFile         string `toml:"log_file"`
	ParallelUploads int    `toml:"parallel_uploads"`
}

type SchedulerConfig struct {
	Enabled         bool   `toml:"enabled"`
	DefaultInterval string `toml:"default_interval"`
	QuietHoursStart string `toml:"quiet_hours_start"`
	QuietHoursEnd   string `toml:"quiet_hours_end"`
}

type StorageConfig struct {
	Backend        string `toml:"backend"`
	RcloneRemote   string `toml:"rclone_remote"`
	RemoteBasePath string `toml:"remote_base_path"`
	BandwidthLimit string `toml:"bandwidth_limit"`
}

type EncryptionConfig struct {
	Enabled   bool   `toml:"enabled"`
	KeySource string `toml:"key_source"`
	KeyFile   string `toml:"key_file"`
	Recipient string `toml:"recipient"`
}

type VersioningConfig struct {
	Strategy       string `toml:"strategy"`
	RetentionCount int    `toml:"retention_count"`
	RetentionDays  int    `toml:"retention_days"`
}

type NotificationConfig struct {
	Enabled    bool   `toml:"enabled"`
	OnFailure  bool   `toml:"on_failure"`
	OnSuccess  bool   `toml:"on_success"`
	WebhookURL string `toml:"webhook_url"`
}

type BackupGroup struct {
	Name     string   `toml:"name"`
	Paths    []string `toml:"paths"`
	Encrypt  bool     `toml:"encrypt"`
	Interval string   `toml:"interval"`
	Exclude  []string `toml:"exclude"`
}

type HookConfig struct {
	Name       string `toml:"name"`
	When       string `toml:"when"`
	Command    string `toml:"command"`
	Timeout    string `toml:"timeout"`
	FailAction string `toml:"fail_action"`
}

// Load reads and parses a TOML config file.
func Load(path string) (*Config, error) {
	path = platform.ExpandPath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", platform.ErrConfigNotFound, path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", platform.ErrConfigInvalid, err)
	}

	// Default parallel_uploads to 3 if not set
	if cfg.General.ParallelUploads == 0 {
		cfg.General.ParallelUploads = 3
	}

	cfg.expandPaths()
	return &cfg, nil
}

// Validate checks the config for errors.
func Validate(c *Config) error {
	if c.General.MachineID == "" {
		return fmt.Errorf("%w: machine_id is required", platform.ErrConfigInvalid)
	}
	if c.Storage.RcloneRemote == "" {
		return fmt.Errorf("%w: rclone_remote is required", platform.ErrConfigInvalid)
	}
	if c.General.ParallelUploads < 1 || c.General.ParallelUploads > 10 {
		return fmt.Errorf("%w: parallel_uploads must be between 1 and 10 (got %d)", platform.ErrConfigInvalid, c.General.ParallelUploads)
	}

	// Validate scheduler interval
	if c.Scheduler.DefaultInterval != "" {
		if _, err := time.ParseDuration(c.Scheduler.DefaultInterval); err != nil {
			return fmt.Errorf("%w: invalid default_interval %q: %v", platform.ErrConfigInvalid, c.Scheduler.DefaultInterval, err)
		}
	}

	// Validate backup groups
	for _, bg := range c.Backups {
		if bg.Name == "" {
			return fmt.Errorf("%w: backup group must have a name", platform.ErrConfigInvalid)
		}
		if len(bg.Paths) == 0 {
			return fmt.Errorf("%w: backup group %q has no paths", platform.ErrConfigInvalid, bg.Name)
		}
		if bg.Interval != "" {
			d, err := time.ParseDuration(bg.Interval)
			if err != nil {
				return fmt.Errorf("%w: invalid interval %q for group %q: %v", platform.ErrConfigInvalid, bg.Interval, bg.Name, err)
			}
			if d < 5*time.Minute {
				return fmt.Errorf("%w: interval for group %q must be at least 5m (got %s)", platform.ErrConfigInvalid, bg.Name, bg.Interval)
			}
		}
	}

	// Validate hooks
	for _, h := range c.Hooks {
		if h.When != "pre-backup" && h.When != "post-backup" {
			return fmt.Errorf("%w: hook %q has invalid when value %q (must be pre-backup or post-backup)", platform.ErrConfigInvalid, h.Name, h.When)
		}
		if h.FailAction != "" && h.FailAction != "continue" && h.FailAction != "abort" {
			return fmt.Errorf("%w: hook %q has invalid fail_action %q", platform.ErrConfigInvalid, h.Name, h.FailAction)
		}
		if h.Timeout != "" {
			if _, err := time.ParseDuration(h.Timeout); err != nil {
				return fmt.Errorf("%w: hook %q has invalid timeout %q", platform.ErrConfigInvalid, h.Name, h.Timeout)
			}
		}
	}

	// Validate encryption key_source
	if c.Encryption.Enabled {
		switch c.Encryption.KeySource {
		case "keyring", "file", "prompt", "":
		default:
			return fmt.Errorf("%w: invalid key_source %q", platform.ErrConfigInvalid, c.Encryption.KeySource)
		}
		if c.Encryption.KeySource == "file" && c.Encryption.KeyFile == "" {
			return fmt.Errorf("%w: key_file is required when key_source is 'file'", platform.ErrConfigInvalid)
		}
	}

	// Warn about duplicate paths (non-fatal)
	seen := make(map[string]string)
	for _, bg := range c.Backups {
		for _, p := range bg.Paths {
			if prev, ok := seen[p]; ok {
				fmt.Fprintf(os.Stderr, "Warning: path %q appears in both %q and %q backup groups\n", p, prev, bg.Name)
			}
			seen[p] = bg.Name
		}
	}

	return nil
}

// SaveConfig writes a config to a TOML file.
func SaveConfig(path string, c *Config) error {
	path = platform.ExpandPath(path)
	if err := platform.EnsureDir(platform.ConfigDir()); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(c)
}

// GetInterval returns the effective interval for a backup group.
func (bg *BackupGroup) GetInterval(defaultInterval string) time.Duration {
	interval := bg.Interval
	if interval == "" {
		interval = defaultInterval
	}
	if interval == "" {
		interval = "1h"
	}
	d, err := time.ParseDuration(interval)
	if err != nil {
		return time.Hour
	}
	return d
}

func (c *Config) expandPaths() {
	c.General.LogFile = platform.ExpandPath(c.General.LogFile)
	c.Encryption.KeyFile = platform.ExpandPath(c.Encryption.KeyFile)
	for i := range c.Backups {
		for j := range c.Backups[i].Paths {
			c.Backups[i].Paths[j] = platform.ExpandPath(c.Backups[i].Paths[j])
		}
	}
}
