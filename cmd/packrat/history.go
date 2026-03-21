package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var historyGroup string

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show backup history",
	RunE:  runHistory,
}

func init() {
	historyCmd.Flags().StringVar(&historyGroup, "group", "", "filter by backup group")
	rootCmd.AddCommand(historyCmd)
}

func runHistory(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()
	if err := openStateDB(); err != nil {
		return err
	}
	defer stateDB.Close()

	records, err := stateDB.GetBackupHistory(historyGroup, 20)
	if err != nil {
		return err
	}

	if len(records) == 0 {
		fmt.Println("No backup history found.")
		return nil
	}

	fmt.Printf("%-25s %-15s %-12s %-10s %-8s %-10s\n",
		"TIMESTAMP", "GROUP", "SNAPSHOT", "STATUS", "FILES", "DURATION")
	fmt.Printf("%-25s %-15s %-12s %-10s %-8s %-10s\n",
		"─────────", "─────", "────────", "──────", "─────", "────────")

	for _, r := range records {
		snapID := r.SnapshotID
		if len(snapID) > 12 {
			snapID = snapID[:12]
		}
		fmt.Printf("%-25s %-15s %-12s %-10s %-8d %-10s\n",
			r.Timestamp.Format("2006-01-02 15:04:05"),
			r.Group,
			snapID,
			r.Status,
			r.Files,
			r.Duration.Round(1e6),
		)
	}
	return nil
}
