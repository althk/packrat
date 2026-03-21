package main

import (
	"fmt"
	"time"

	"github.com/harish/packrat/internal/scheduler"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status, last backup time, next scheduled",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()
	if err := openStateDB(); err != nil {
		return err
	}
	defer stateDB.Close()

	// Daemon status
	running, pid, _ := scheduler.DaemonStatus()
	if running {
		fmt.Printf("Daemon: running (PID %d)\n", pid)
	} else {
		fmt.Println("Daemon: stopped")
	}
	fmt.Println()

	// Per-group status
	fmt.Printf("%-20s %-25s %-10s\n", "GROUP", "LAST BACKUP", "STATUS")
	fmt.Printf("%-20s %-25s %-10s\n", "─────", "───────────", "──────")

	for _, bg := range appCfg.Backups {
		lastTime, err := stateDB.GetLastBackupTime(bg.Name)
		lastStr := "never"
		status := "pending"

		if err == nil && !lastTime.IsZero() {
			lastStr = lastTime.Format("2006-01-02 15:04:05")
			ago := time.Since(lastTime)
			interval := bg.GetInterval(appCfg.Scheduler.DefaultInterval)
			if ago > interval*2 {
				status = "overdue"
			} else {
				status = "ok"
			}
		}

		fmt.Printf("%-20s %-25s %-10s\n", bg.Name, lastStr, status)
	}

	return nil
}
