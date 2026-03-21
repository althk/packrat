package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/config"
	"github.com/robfig/cron/v3"
)

// Scheduler manages periodic backup jobs.
type Scheduler struct {
	cfg     *config.Config
	engine  *backup.Engine
	cron    *cron.Cron
	logger  *slog.Logger
	stateDB *backup.StateDB
	jobs    map[string]cron.EntryID
}

// NewScheduler creates a new backup scheduler.
func NewScheduler(cfg *config.Config, engine *backup.Engine, stateDB *backup.StateDB) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		engine:  engine,
		stateDB: stateDB,
		cron:    cron.New(cron.WithSeconds()),
		logger:  slog.Default(),
		jobs:    make(map[string]cron.EntryID),
	}
}

// Start registers cron jobs for each backup group and starts the scheduler.
func (s *Scheduler) Start() error {
	for _, bg := range s.cfg.Backups {
		interval := bg.GetInterval(s.cfg.Scheduler.DefaultInterval)
		spec := fmt.Sprintf("@every %s", interval)

		group := bg // capture
		id, err := s.cron.AddFunc(spec, func() {
			s.runGroup(group)
		})
		if err != nil {
			return fmt.Errorf("scheduling %s: %w", bg.Name, err)
		}
		s.jobs[bg.Name] = id
		s.logger.Info("scheduled backup", "group", bg.Name, "interval", interval)
	}

	s.cron.Start()
	return nil
}

// Stop stops the scheduler and waits for running jobs.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// CheckOverdue returns group names that are overdue for a backup.
func (s *Scheduler) CheckOverdue() []string {
	var overdue []string
	for _, bg := range s.cfg.Backups {
		interval := bg.GetInterval(s.cfg.Scheduler.DefaultInterval)
		lastBackup, err := s.stateDB.GetLastBackupTime(bg.Name)
		if err != nil {
			continue
		}
		if lastBackup.IsZero() || time.Since(lastBackup) > interval {
			overdue = append(overdue, bg.Name)
		}
	}
	return overdue
}

// RunOverdue runs all overdue backup groups immediately.
func (s *Scheduler) RunOverdue(ctx context.Context) error {
	overdue := s.CheckOverdue()
	if len(overdue) == 0 {
		return nil
	}
	s.logger.Info("running overdue backups", "groups", overdue)
	return s.engine.Run(ctx, overdue...)
}

// NextRun returns the next scheduled run time for a group.
func (s *Scheduler) NextRun(group string) time.Time {
	id, ok := s.jobs[group]
	if !ok {
		return time.Time{}
	}
	entry := s.cron.Entry(id)
	return entry.Next
}

func (s *Scheduler) runGroup(bg config.BackupGroup) {
	// Check quiet hours
	if s.inQuietHours() {
		s.logger.Debug("skipping backup during quiet hours", "group", bg.Name)
		return
	}

	ctx := context.Background()
	if err := s.engine.Run(ctx, bg.Name); err != nil {
		s.logger.Error("scheduled backup failed", "group", bg.Name, "error", err)
	}
}

func (s *Scheduler) inQuietHours() bool {
	if s.cfg.Scheduler.QuietHoursStart == "" || s.cfg.Scheduler.QuietHoursEnd == "" {
		return false
	}

	now := time.Now()
	start, err1 := time.Parse("15:04", s.cfg.Scheduler.QuietHoursStart)
	end, err2 := time.Parse("15:04", s.cfg.Scheduler.QuietHoursEnd)
	if err1 != nil || err2 != nil {
		return false
	}

	nowMinutes := now.Hour()*60 + now.Minute()
	startMinutes := start.Hour()*60 + start.Minute()
	endMinutes := end.Hour()*60 + end.Minute()

	if startMinutes <= endMinutes {
		return nowMinutes >= startMinutes && nowMinutes < endMinutes
	}
	// Wraps midnight
	return nowMinutes >= startMinutes || nowMinutes < endMinutes
}
