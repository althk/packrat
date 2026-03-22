package main

import (
	"fmt"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/platform"
	"github.com/spf13/cobra"
)

var (
	backupGroup   string
	backupDryRun  bool
	backupForce   bool
	backupVerbose bool
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Run backup now",
	RunE:  runBackup,
}

func init() {
	backupCmd.Flags().StringVar(&backupGroup, "group", "", "backup specific group only")
	backupCmd.Flags().BoolVar(&backupDryRun, "dry-run", false, "show what would be backed up")
	backupCmd.Flags().BoolVar(&backupForce, "force", false, "force backup even if no changes detected")
	backupCmd.Flags().BoolVar(&backupVerbose, "verbose", false, "show detailed file-level progress")
	rootCmd.AddCommand(backupCmd)
}

func runBackup(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()
	if err := openStateDB(); err != nil {
		return err
	}
	defer stateDB.Close()

	store := newStorageBackend()
	engine := backup.NewEngine(appCfg, store, stateDB)

	ctx := cmd.Context()

	if backupDryRun {
		var groups []string
		if backupGroup != "" {
			groups = []string{backupGroup}
		}
		changes, err := engine.DryRun(ctx, groups...)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			platform.Info("No changes detected.")
			return nil
		}
		for group, fileChanges := range changes {
			platform.Header("\n" + group)
			for _, c := range fileChanges {
				platform.FileChange(c.Status, c.Path)
			}
		}
		return nil
	}

	var groups []string
	if backupGroup != "" {
		groups = []string{backupGroup}
	}

	opts := backup.BackupOptions{
		Force:   backupForce,
		Verbose: backupVerbose,
	}
	if !quiet {
		opts.OnProgress = func(group, stage string, current, total int, bytes, totalBytes int64) {
			switch stage {
			case "scanning":
				platform.Infof("[%s] scanning files...", group)
			case "uploading":
				platform.Infof("[%s] uploading %d/%d files (%s / %s)",
					group, current, total, formatBytes(bytes), formatBytes(totalBytes))
			case "done":
				platform.Infof("[%s] done (%d files, %s)", group, total, formatBytes(totalBytes))
			}
		}
	}

	if err := engine.Run(ctx, opts, groups...); err != nil {
		return err
	}

	if !quiet {
		platform.Success("Backup complete.")
	}
	return nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
