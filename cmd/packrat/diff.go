package main

import (
	"fmt"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/platform"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff [snapshot1] [snapshot2]",
	Short: "Diff current state vs last snapshot, or between two snapshots",
	RunE:  runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()
	if err := openStateDB(); err != nil {
		return err
	}
	defer stateDB.Close()

	if len(args) == 2 {
		// Diff between two snapshots
		snap1, err := stateDB.GetSnapshotByID(args[0])
		if err != nil || snap1 == nil {
			return fmt.Errorf("snapshot %q not found", args[0])
		}
		snap2, err := stateDB.GetSnapshotByID(args[1])
		if err != nil || snap2 == nil {
			return fmt.Errorf("snapshot %q not found", args[1])
		}

		changes := backup.DiffSnapshots(snap1, snap2)
		printChanges(changes)
		return nil
	}

	// Diff current state vs last snapshots
	store := newStorageBackend()
	engine := backup.NewEngine(appCfg, store, stateDB)

	changes, err := engine.DryRun(cmd.Context())
	if err != nil {
		return err
	}

	if len(changes) == 0 {
		platform.Info("No changes detected.")
		return nil
	}

	for group, fileChanges := range changes {
		platform.Header("\n" + group)
		printChanges(fileChanges)
	}
	return nil
}

func printChanges(changes []backup.FileChange) {
	for _, c := range changes {
		platform.FileChange(c.Status, c.Path)
	}
}
