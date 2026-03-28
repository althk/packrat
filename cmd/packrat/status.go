package main

import (
	"fmt"
	"time"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/platform"
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
		platform.Success(fmt.Sprintf("Daemon running (PID %d)", pid))
	} else {
		platform.Warn("Daemon stopped")
	}
	fmt.Println()

	// Per-group status
	cols := []platform.TableCol{
		{Name: "GROUP", Width: 20},
		{Name: "LAST BACKUP", Width: 25},
		{Name: "STATUS", Width: 10},
	}
	platform.TableHeader(cols...)

	// Check if a backup is currently running and which groups are active
	backupRunning, activeGroups := backup.ReadLockStatus()
	activeSet := make(map[string]bool, len(activeGroups))
	for _, g := range activeGroups {
		activeSet[g] = true
	}

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

		if backupRunning && activeSet[bg.Name] {
			status = "in-progress"
		}

		platform.TableRow(cols, bg.Name, lastStr, platform.StatusTag(status))
	}

	return nil
}
