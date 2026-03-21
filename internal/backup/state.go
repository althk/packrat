package backup

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// StateDB manages the local SQLite state database.
type StateDB struct {
	db *sql.DB
}

// BackupRecord represents a historical backup run.
type BackupRecord struct {
	ID         int64
	Timestamp  time.Time
	Group      string
	SnapshotID string
	Status     string
	Error      string
	Duration   time.Duration
	Files      int
	Bytes      int64
}

// OpenStateDB opens or creates the SQLite state database.
func OpenStateDB(path string) (*StateDB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening state db: %w", err)
	}

	if err := createTables(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	return &StateDB{db: db}, nil
}

func createTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS snapshots (
			id TEXT PRIMARY KEY,
			timestamp TEXT NOT NULL,
			machine_id TEXT NOT NULL,
			group_name TEXT NOT NULL,
			manifest TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS backup_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			group_name TEXT NOT NULL,
			snapshot_id TEXT,
			status TEXT NOT NULL,
			error_msg TEXT,
			duration_ms INTEGER,
			files_changed INTEGER,
			bytes_uploaded INTEGER
		);

		CREATE INDEX IF NOT EXISTS idx_snapshots_group ON snapshots(group_name);
		CREATE INDEX IF NOT EXISTS idx_backup_runs_group ON backup_runs(group_name);
	`)
	return err
}

// Close closes the database connection.
func (s *StateDB) Close() error {
	return s.db.Close()
}

// SaveSnapshot stores a snapshot manifest in the database.
func (s *StateDB) SaveSnapshot(snap *Snapshot) error {
	data, err := MarshalSnapshot(snap)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		"INSERT OR REPLACE INTO snapshots (id, timestamp, machine_id, group_name, manifest) VALUES (?, ?, ?, ?, ?)",
		snap.ID, snap.Timestamp.UTC().Format(time.RFC3339), snap.MachineID, snap.Group, string(data),
	)
	return err
}

// GetLastSnapshot retrieves the most recent snapshot for a group.
func (s *StateDB) GetLastSnapshot(group string) (*Snapshot, error) {
	var manifest string
	err := s.db.QueryRow(
		"SELECT manifest FROM snapshots WHERE group_name = ? ORDER BY timestamp DESC LIMIT 1",
		group,
	).Scan(&manifest)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying last snapshot: %w", err)
	}
	return UnmarshalSnapshot([]byte(manifest))
}

// ListSnapshots returns all snapshots for a group, newest first.
func (s *StateDB) ListSnapshots(group string) ([]*Snapshot, error) {
	query := "SELECT manifest FROM snapshots"
	var args []any

	if group != "" {
		query += " WHERE group_name = ?"
		args = append(args, group)
	}
	query += " ORDER BY timestamp DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*Snapshot
	for rows.Next() {
		var manifest string
		if err := rows.Scan(&manifest); err != nil {
			return nil, err
		}
		snap, err := UnmarshalSnapshot([]byte(manifest))
		if err != nil {
			continue
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, rows.Err()
}

// RecordBackupRun saves a backup run record.
func (s *StateDB) RecordBackupRun(group, snapshotID, status, errMsg string, duration time.Duration, filesChanged int, bytesUploaded int64) error {
	_, err := s.db.Exec(
		"INSERT INTO backup_runs (timestamp, group_name, snapshot_id, status, error_msg, duration_ms, files_changed, bytes_uploaded) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		time.Now().UTC().Format(time.RFC3339),
		group, snapshotID, status, errMsg,
		duration.Milliseconds(), filesChanged, bytesUploaded,
	)
	return err
}

// GetBackupHistory returns recent backup runs for a group.
func (s *StateDB) GetBackupHistory(group string, limit int) ([]BackupRecord, error) {
	query := "SELECT id, timestamp, group_name, COALESCE(snapshot_id, ''), status, COALESCE(error_msg, ''), COALESCE(duration_ms, 0), COALESCE(files_changed, 0), COALESCE(bytes_uploaded, 0) FROM backup_runs"
	var args []any

	if group != "" {
		query += " WHERE group_name = ?"
		args = append(args, group)
	}
	query += " ORDER BY timestamp DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying backup history: %w", err)
	}
	defer rows.Close()

	var records []BackupRecord
	for rows.Next() {
		var r BackupRecord
		var ts string
		var durationMs int64
		if err := rows.Scan(&r.ID, &ts, &r.Group, &r.SnapshotID, &r.Status, &r.Error, &durationMs, &r.Files, &r.Bytes); err != nil {
			return nil, err
		}
		r.Timestamp, _ = time.Parse(time.RFC3339, ts)
		r.Duration = time.Duration(durationMs) * time.Millisecond
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetLastBackupTime returns the last successful backup time for a group.
func (s *StateDB) GetLastBackupTime(group string) (time.Time, error) {
	var ts string
	err := s.db.QueryRow(
		"SELECT timestamp FROM backup_runs WHERE group_name = ? AND status = 'success' ORDER BY timestamp DESC LIMIT 1",
		group,
	).Scan(&ts)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, ts)
}

// DeleteSnapshot removes a snapshot from the database.
func (s *StateDB) DeleteSnapshot(id string) error {
	_, err := s.db.Exec("DELETE FROM snapshots WHERE id = ?", id)
	return err
}

// GetSnapshotByID retrieves a specific snapshot.
func (s *StateDB) GetSnapshotByID(id string) (*Snapshot, error) {
	var manifest string
	err := s.db.QueryRow("SELECT manifest FROM snapshots WHERE id = ?", id).Scan(&manifest)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return UnmarshalSnapshot([]byte(manifest))
}
