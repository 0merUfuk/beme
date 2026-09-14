// Migrations: versioned, atomic, rollback-aware schema evolution for
// projection stores (FR-064). Each migration is a numbered pair (up/down).
// Migrations run inside a transaction; a failed migration leaves the previous
// store intact (§23.3). Schema version is recorded in the meta table.
package storage

import (
	"fmt"
	"sort"
)

// Migration is one versioned schema step.
type Migration struct {
	Version int
	Name    string
	Up      string // SQL executed inside a transaction
	Down    string // SQL reverting Up (best-effort for SQLite DDL limits)
}

// Migrations is the ordered migration set. Append-only: never edit a
// shipped migration; add a new one (expand-contract discipline).
var Migrations = []Migration{
	{
		Version: 1,
		Name:    "base-schema",
		Up: `CREATE TABLE IF NOT EXISTS records (
  record_id        TEXT PRIMARY KEY,
  source_id        TEXT NOT NULL,
  source_record_id TEXT NOT NULL,
  kind             TEXT NOT NULL,
  decision_key      TEXT NOT NULL DEFAULT '',
  payload          TEXT NOT NULL,
  sensitivity      TEXT NOT NULL,
  status           TEXT NOT NULL,
  authority        TEXT NOT NULL,
  source_role      TEXT NOT NULL,
  trust            TEXT NOT NULL,
  criticality      TEXT NOT NULL DEFAULT '',
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_records_source ON records(source_id);
CREATE INDEX IF NOT EXISTS idx_records_kind ON records(kind);
CREATE VIRTUAL TABLE IF NOT EXISTS records_fts USING fts5(
  record_id UNINDEXED, body, tokenize='unicode61'
);
CREATE TABLE IF NOT EXISTS provenance (
  provenance_id    TEXT PRIMARY KEY,
  source_id        TEXT NOT NULL,
  source_record_id TEXT NOT NULL,
  payload          TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tombstones (
  key TEXT PRIMARY KEY, reason TEXT NOT NULL, at TEXT NOT NULL
);`,
		Down: `DROP TABLE IF EXISTS records_fts;
DROP TABLE IF EXISTS records;
DROP TABLE IF EXISTS provenance;
DROP TABLE IF EXISTS tombstones;`,
	},
}

// Migrate applies all pending migrations atomically. A failure mid-way
// rolls back that migration only; previously applied versions remain.
func (s *Store) Migrate() error {
	current, err := s.schemaVersion()
	if err != nil {
		return err
	}
	pending := []Migration{}
	for _, m := range Migrations {
		if m.Version > current {
			pending = append(pending, m)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Version < pending[j].Version })
	for _, m := range pending {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.Up); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s) failed: %w — previous schema intact", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES ('schema_version', ?)`, fmt.Sprintf("%d", m.Version)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d commit failed: %w — previous schema intact", m.Version, err)
		}
	}
	return nil
}

// Rollback reverts the latest applied migration (best-effort; SQLite DDL is
// transactional but destructive down steps must be written carefully).
func (s *Store) Rollback() error {
	current, err := s.schemaVersion()
	if err != nil {
		return err
	}
	if current == 0 {
		return fmt.Errorf("no migrations applied")
	}
	var target Migration
	found := false
	for _, m := range Migrations {
		if m.Version == current {
			target = m
			found = true
		}
	}
	if !found {
		return fmt.Errorf("schema version %d has no registered migration", current)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(target.Down); err != nil {
		tx.Rollback()
		return fmt.Errorf("rollback of %d failed: %w — schema unchanged", current, err)
	}
	prev := current - 1
	if _, err := tx.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES ('schema_version', ?)`, fmt.Sprintf("%d", prev)); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// schemaVersion reads the applied version; 0 = fresh store. meta is created
// by Open() before migrations run.
func (s *Store) schemaVersion() (int, error) {
	v, ok := s.GetMeta("schema_version")
	if !ok || v == "" {
		return 0, nil
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return 0, fmt.Errorf("corrupt schema_version %q: %w", v, err)
	}
	return n, nil
}

// SchemaVersion exposes the applied version (health/doctor).
func (s *Store) SchemaVersion() (int, error) { return s.schemaVersion() }
