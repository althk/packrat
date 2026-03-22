package main

import (
	"log/slog"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/platform"
	"github.com/harish/packrat/internal/storage"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
	quiet   bool
	noColor bool

	// Loaded at pre-run
	appCfg  *config.Config
	stateDB *backup.StateDB
)

var rootCmd = &cobra.Command{
	Use:           "packrat",
	Short:         "Packrat — automatic backup for shell history, dotfiles, and configs",
	Long:          `Packrat is a CLI tool + background daemon that automatically backs up shell history, dotfiles, config directories, and arbitrary paths to remote storage via rclone.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/packrat/config.toml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress all output except errors")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
}

// loadConfig loads the config file. Returns nil error if config is optional for the command.
func loadConfig() error {
	path := cfgFile
	if path == "" {
		path = platform.DefaultConfigPath()
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	appCfg = cfg
	return nil
}

// setupLogger initializes the logger based on config and flags.
func setupLogger() {
	platform.SetColorEnabled(!noColor)
	level := "info"
	if appCfg != nil && appCfg.General.LogLevel != "" {
		level = appCfg.General.LogLevel
	}
	if verbose {
		level = "debug"
	}
	if quiet {
		level = "error"
	}

	logFile := ""
	if appCfg != nil {
		logFile = appCfg.General.LogFile
	}

	platform.SetupLogger(level, logFile, true)
}

// openStateDB opens the SQLite state database.
func openStateDB() error {
	if err := platform.EnsureDir(platform.DataDir()); err != nil {
		return err
	}
	db, err := backup.OpenStateDB(platform.StateDatabasePath())
	if err != nil {
		return err
	}
	stateDB = db
	return nil
}

// newStorageBackend creates a storage backend from config.
func newStorageBackend() storage.StorageBackend {
	return storage.NewRcloneBackend(
		appCfg.Storage.RcloneRemote,
		appCfg.Storage.RemoteBasePath,
		appCfg.Storage.BandwidthLimit,
	)
}

// quietPrint prints unless quiet mode is enabled.
func quietPrint(format string, args ...interface{}) {
	if !quiet {
		slog.Info(format, args...)
	}
}
