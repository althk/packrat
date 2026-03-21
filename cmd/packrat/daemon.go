package main

import (
	"fmt"
	"os"

	"github.com/harish/packrat/internal/scheduler"
	"github.com/harish/packrat/internal/storage"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the background daemon",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start background scheduler",
	RunE:  runDaemonStart,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop background scheduler",
	RunE:  runDaemonStop,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if daemon is running",
	RunE:  runDaemonStatus,
}

// Hidden command: the actual daemon process loop
var daemonRunCmd = &cobra.Command{
	Use:    "run",
	Hidden: true,
	RunE:   runDaemonLoop,
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd, daemonStopCmd, daemonStatusCmd, daemonRunCmd)
	rootCmd.AddCommand(daemonCmd)
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	exe, err := getExecutable()
	if err != nil {
		return err
	}

	cfgPath := cfgFile
	if err := scheduler.StartDaemon(exe, cfgPath); err != nil {
		return err
	}

	fmt.Println("Daemon started.")
	return nil
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	if err := scheduler.StopDaemon(); err != nil {
		return err
	}
	fmt.Println("Daemon stopped.")
	return nil
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	running, pid, err := scheduler.DaemonStatus()
	if err != nil {
		return err
	}
	if running {
		fmt.Printf("Daemon is running (PID %d)\n", pid)
	} else {
		fmt.Println("Daemon is not running")
	}
	return nil
}

func runDaemonLoop(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	store := storage.NewRcloneBackend(
		appCfg.Storage.RcloneRemote,
		appCfg.Storage.RemoteBasePath,
		appCfg.Storage.BandwidthLimit,
	)

	return scheduler.RunDaemonLoop(appCfg, store)
}

func getExecutable() (string, error) {
	return os.Executable()
}
