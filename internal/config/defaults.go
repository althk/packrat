package config

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/harish/packrat/internal/platform"
)

// DetectShell returns the current shell name (bash, zsh, or fish).
func DetectShell() string {
	shell := os.Getenv("SHELL")
	base := filepath.Base(shell)
	switch base {
	case "zsh":
		return "zsh"
	case "fish":
		return "fish"
	case "bash":
		return "bash"
	default:
		return "bash"
	}
}

// GenerateMachineID returns a short random hex string for use as machine_id.
func GenerateMachineID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// DefaultMachineName returns a human-readable machine name.
func DefaultMachineName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	user := os.Getenv("USER")
	if user == "" {
		return hostname
	}
	return user + "-" + hostname
}

// DefaultBackupGroups returns the default backup groups based on detected shell.
func DefaultBackupGroups(shell string) []BackupGroup {
	groups := []BackupGroup{
		{
			Name:     "shell-history",
			Interval: "30m",
			Encrypt:  false,
			Exclude:  []string{},
		},
		{
			Name: "dotfiles",
			Paths: []string{
				"~/.bashrc",
				"~/.zshrc",
				"~/.bash_profile",
				"~/.profile",
				"~/.aliases",
				"~/.vimrc",
				"~/.tmux.conf",
				"~/.gitconfig",
				"~/.ssh/config",
			},
			Interval: "1h",
			Encrypt:  false,
			Exclude:  []string{},
		},
		{
			Name: "ai-configs",
			Paths: []string{
				"~/.claude/",
				"~/.gemini/",
				"~/.config/github-copilot/",
			},
			Interval: "2h",
			Encrypt:  true,
			Exclude:  []string{"*.log", "*.cache"},
		},
		{
			Name: "editor-configs",
			Paths: []string{
				"~/.config/nvim/",
				"~/.config/Code/User/settings.json",
				"~/.config/Code/User/keybindings.json",
				"~/.config/Code/User/snippets/",
			},
			Interval: "1h",
			Encrypt:  false,
			Exclude:  []string{"*.cache", "workspaceStorage/"},
		},
		{
			Name: "gnupg",
			Paths: []string{
				"~/.gnupg/",
			},
			Interval: "6h",
			Encrypt:  true,
			Exclude:  []string{"*.lock", "S.gpg-agent*", "random_seed"},
		},
	}

	// Set history paths based on detected shell
	var histPaths []string
	switch shell {
	case "zsh":
		histPaths = []string{"~/.zsh_history"}
	case "fish":
		histPaths = []string{"~/.local/share/fish/fish_history"}
	case "bash":
		histPaths = []string{"~/.bash_history"}
	default:
		histPaths = []string{
			"~/.bash_history",
			"~/.zsh_history",
			"~/.local/share/fish/fish_history",
		}
	}
	groups[0].Paths = histPaths

	return groups
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	shell := DetectShell()
	return &Config{
		General: GeneralConfig{
			MachineName: DefaultMachineName(),
			MachineID:   GenerateMachineID(),
			LogLevel:    "info",
			LogFile:     platform.LogFilePath(),
		},
		Scheduler: SchedulerConfig{
			Enabled:         true,
			DefaultInterval: "1h",
		},
		Storage: StorageConfig{
			Backend:        "rclone",
			RemoteBasePath: "packrat-backups",
		},
		Encryption: EncryptionConfig{
			Enabled:   true,
			KeySource: "keyring",
		},
		Versioning: VersioningConfig{
			Strategy:       "snapshot",
			RetentionCount: 50,
			RetentionDays:  30,
		},
		Notifications: NotificationConfig{
			Enabled:   false,
			OnFailure: true,
			OnSuccess: false,
		},
		Backups: DefaultBackupGroups(shell),
		Hooks:   defaultHooks(),
	}
}

func defaultHooks() []HookConfig {
	return []HookConfig{
		{
			Name: "dump-package-lists",
			When: "pre-backup",
			Command: strings.TrimSpace(`
dpkg --get-selections > ~/.config/packrat/installed-packages.txt 2>/dev/null || true
pip list --format=freeze > ~/.config/packrat/pip-packages.txt 2>/dev/null || true
`),
			Timeout:    "30s",
			FailAction: "continue",
		},
	}
}
