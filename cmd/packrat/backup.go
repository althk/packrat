package main

import (
	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/platform"
	"github.com/spf13/cobra"
)

var (
	backupGroup  string
	backupDryRun bool
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Run backup now",
	RunE:  runBackup,
}

func init() {
	backupCmd.Flags().StringVar(&backupGroup, "group", "", "backup specific group only")
	backupCmd.Flags().BoolVar(&backupDryRun, "dry-run", false, "show what would be backed up")
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

	if err := engine.Run(ctx, groups...); err != nil {
		return err
	}

	if !quiet {
		platform.Success("Backup complete.")
	}
	return nil
}
