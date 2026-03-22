package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/platform"
	"github.com/harish/packrat/internal/storage"
)

// StartDaemon launches the packrat daemon as a background process.
func StartDaemon(executablePath string, cfgPath string) error {
	// Check if already running
	if running, pid, _ := DaemonStatus(); running {
		return fmt.Errorf("%w (PID %d)", platform.ErrDaemonRunning, pid)
	}

	if err := platform.EnsureDir(platform.DataDir()); err != nil {
		return err
	}

	// Re-exec with hidden flag
	args := []string{"daemon", "run"}
	if cfgPath != "" {
		args = append(args, "--config", cfgPath)
	}

	cmd := exec.Command(executablePath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	// Write PID file
	pid := cmd.Process.Pid
	if err := os.WriteFile(platform.DaemonPIDPath(), []byte(strconv.Itoa(pid)), 0o600); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}

	// Detach
	cmd.Process.Release()
	return nil
}

// StopDaemon sends SIGTERM to the running daemon.
func StopDaemon() error {
	running, pid, err := DaemonStatus()
	if err != nil {
		return err
	}
	if !running {
		return platform.ErrDaemonNotRunning
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM: %w", err)
	}

	// Clean up PID file
	os.Remove(platform.DaemonPIDPath())
	return nil
}

// DaemonStatus checks if the daemon is running.
func DaemonStatus() (running bool, pid int, err error) {
	data, err := os.ReadFile(platform.DaemonPIDPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}

	pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false, 0, nil
	}

	// Check if process is alive
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		// Stale PID file
		os.Remove(platform.DaemonPIDPath())
		return false, 0, nil
	}

	return true, pid, nil
}

// RunDaemonLoop is the main daemon process loop. It sets up the engine,
// scheduler, runs overdue backups, and blocks until SIGTERM/SIGINT.
func RunDaemonLoop(cfg *config.Config, store storage.StorageBackend) error {
	logger := platform.SetupLogger(cfg.General.LogLevel, cfg.General.LogFile, false)
	logger.Info("daemon starting", "machine_id", cfg.General.MachineID)

	if err := platform.EnsureDir(platform.DataDir()); err != nil {
		return err
	}

	stateDB, err := backup.OpenStateDB(platform.StateDatabasePath())
	if err != nil {
		return fmt.Errorf("opening state db: %w", err)
	}
	defer stateDB.Close()

	engine := backup.NewEngine(cfg, store, stateDB)
	sched := NewScheduler(cfg, engine, stateDB)

	// Run overdue backups first
	ctx := context.Background()
	if err := sched.RunOverdue(ctx); err != nil {
		logger.Warn("overdue backup failed", "error", err)
	}

	// Start scheduler
	if err := sched.Start(); err != nil {
		return fmt.Errorf("starting scheduler: %w", err)
	}

	// Block on signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh

	logger.Info("daemon stopping", "signal", sig)
	sched.Stop()

	// Clean up PID file
	os.Remove(platform.DaemonPIDPath())

	logger.Info("daemon stopped")
	return nil
}
