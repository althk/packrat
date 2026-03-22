package main

import (
	"fmt"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/platform"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify integrity of local files vs last snapshot",
	RunE:  runVerify,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()
	if err := openStateDB(); err != nil {
		return err
	}
	defer stateDB.Close()

	var totalMismatch int

	for _, bg := range appCfg.Backups {
		snap, err := stateDB.GetLastSnapshot(bg.Name)
		if err != nil || snap == nil {
			continue
		}

		for _, entry := range snap.Files {
			if entry.Status == "deleted" {
				continue
			}
			currentHash, err := backup.ComputeFileHash(entry.Path)
			if err != nil {
				platform.FileChange("deleted", entry.Path+" (MISSING)")
				totalMismatch++
				continue
			}
			if currentHash != entry.SHA256 {
				platform.FileChange("modified", entry.Path+" (CHANGED)")
				totalMismatch++
			}
		}
	}

	if totalMismatch == 0 {
		platform.Success("All files match their last snapshot.")
	} else {
		fmt.Println()
		platform.Warn(fmt.Sprintf("%d file(s) have changed since last snapshot.", totalMismatch))
	}

	return nil
}
