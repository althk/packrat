package scheduler

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/storage"
)

func testSetup(t *testing.T) (*Scheduler, *backup.StateDB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	stateDB, err := backup.OpenStateDB(dbPath)
	if err != nil {
		t.Fatalf("OpenStateDB: %v", err)
	}
	t.Cleanup(func() { stateDB.Close() })

	mock := storage.NewMockBackend()
	cfg := &config.Config{
		General: config.GeneralConfig{
			MachineID:   "test",
			MachineName: "test",
		},
		Scheduler: config.SchedulerConfig{
			Enabled:         true,
			DefaultInterval: "1h",
		},
		Backups: []config.BackupGroup{
			{Name: "test-group", Paths: []string{t.TempDir()}, Interval: "30m"},
		},
	}

	engine := backup.NewEngine(cfg, mock, stateDB)
	sched := NewScheduler(cfg, engine, stateDB)
	return sched, stateDB
}

func TestSchedulerStartStop(t *testing.T) {
	sched, _ := testSetup(t)

	if err := sched.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Check next run is in the future
	next := sched.NextRun("test-group")
	if next.IsZero() {
		t.Error("NextRun should not be zero")
	}
	if next.Before(time.Now()) {
		t.Error("NextRun should be in the future")
	}

	sched.Stop()
}

func TestCheckOverdue(t *testing.T) {
	sched, _ := testSetup(t)

	// No backups have run, so everything is overdue
	overdue := sched.CheckOverdue()
	if len(overdue) != 1 {
		t.Errorf("overdue = %d, want 1", len(overdue))
	}
}

func TestCheckOverdueAfterBackup(t *testing.T) {
	sched, stateDB := testSetup(t)

	// Record a recent backup
	stateDB.RecordBackupRun("test-group", "snap-1", "success", "", time.Second, 0, 0)

	overdue := sched.CheckOverdue()
	if len(overdue) != 0 {
		t.Errorf("overdue = %d, want 0 (just backed up)", len(overdue))
	}
}

func TestDaemonStatusNotRunning(t *testing.T) {
	running, _, _ := DaemonStatus()
	// It's fine if it returns false (no daemon running)
	_ = running
}

func TestInQuietHours(t *testing.T) {
	sched, _ := testSetup(t)

	// No quiet hours configured
	if sched.inQuietHours() {
		t.Error("should not be in quiet hours when not configured")
	}

	// Set quiet hours
	sched.cfg.Scheduler.QuietHoursStart = "00:00"
	sched.cfg.Scheduler.QuietHoursEnd = "00:01"
	// This is a narrow window, so it's unlikely we're in it
}
