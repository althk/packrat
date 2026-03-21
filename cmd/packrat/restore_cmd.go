package main

import (
	"fmt"

	"github.com/harish/packrat/internal/restore"
	"github.com/harish/packrat/internal/tui"
	"github.com/spf13/cobra"
)

var (
	restoreSnapshot string
	restoreFile     string
	restoreList     bool
	restoreGroup    string
	restoreDest     string
	restoreLatest   bool
	restoreYes      bool
)

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore files from backup",
	Long:  "Launch TUI restore interface, or use flags for non-interactive restore.",
	RunE:  runRestore,
}

func init() {
	restoreCmd.Flags().StringVar(&restoreSnapshot, "snapshot", "", "restore specific snapshot (non-interactive)")
	restoreCmd.Flags().StringVar(&restoreFile, "file", "", "restore a specific file")
	restoreCmd.Flags().BoolVar(&restoreList, "list", false, "list available snapshots")
	restoreCmd.Flags().StringVar(&restoreGroup, "group", "", "filter by backup group")
	restoreCmd.Flags().StringVar(&restoreDest, "dest", "", "restore to alternate directory")
	restoreCmd.Flags().BoolVar(&restoreLatest, "latest", false, "restore from most recent snapshot")
	restoreCmd.Flags().BoolVar(&restoreYes, "yes", false, "skip confirmation prompts")
	rootCmd.AddCommand(restoreCmd)
}

func runRestore(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()
	if err := openStateDB(); err != nil {
		return err
	}
	defer stateDB.Close()

	store := newStorageBackend()
	r := restore.NewRestorer(appCfg, store, stateDB)

	ctx := cmd.Context()

	// List mode
	if restoreList {
		snapshots, err := r.ListSnapshots(restoreGroup)
		if err != nil {
			return err
		}
		if len(snapshots) == 0 {
			fmt.Println("No snapshots found.")
			return nil
		}
		for _, s := range snapshots {
			fmt.Printf("  %s  %-15s  %d files  %s\n",
				s.ID, s.Group, s.Stats.TotalFiles, s.Timestamp.Format("2006-01-02 15:04:05"))
		}
		return nil
	}

	if restoreLatest {
		if restoreGroup == "" {
			return fmt.Errorf("--latest requires --group")
		}
		s, err := r.GetLatestSnapshot(restoreGroup)
		if err != nil {
			return err
		}
		if s == nil {
			return fmt.Errorf("no snapshots found for group %q", restoreGroup)
		}

		opts := restore.RestoreOptions{
			DestDir: restoreDest,
			Force:   restoreYes,
			Yes:     restoreYes,
		}

		if restoreFile != "" {
			return r.RestoreFiles(ctx, s, []string{restoreFile}, opts)
		}
		return r.RestoreSnapshot(ctx, s, opts)
	}

	if restoreSnapshot != "" {
		s, err := r.GetSnapshotByID(restoreSnapshot)
		if err != nil {
			return err
		}
		if s == nil {
			return fmt.Errorf("snapshot %q not found", restoreSnapshot)
		}

		opts := restore.RestoreOptions{
			DestDir: restoreDest,
			Force:   restoreYes,
			Yes:     restoreYes,
		}

		if restoreFile != "" {
			return r.RestoreFiles(ctx, s, []string{restoreFile}, opts)
		}
		return r.RestoreSnapshot(ctx, s, opts)
	}

	// No flags — launch TUI
	return tui.Run(appCfg, store, stateDB)
}
