package main

import (
	"github.com/harish/packrat/internal/platform"
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
		platform.Info("No backup history found.")
		return nil
	}

	cols := []platform.TableCol{
		{Name: "TIMESTAMP", Width: 25},
		{Name: "GROUP", Width: 15},
		{Name: "SNAPSHOT", Width: 12},
		{Name: "STATUS", Width: 10},
		{Name: "FILES", Width: 8},
		{Name: "DURATION", Width: 10},
	}
	platform.TableHeader(cols...)

	for _, r := range records {
		snapID := r.SnapshotID
		if len(snapID) > 12 {
			snapID = snapID[:12]
		}
		platform.TableRow(cols,
			r.Timestamp.Format("2006-01-02 15:04:05"),
			r.Group,
			snapID,
			platform.StatusTag(r.Status),
			platform.Itoa(r.Files),
			r.Duration.Round(1e6).String(),
		)
	}
	return nil
}
