// Package storage implements the rebuildable operational index: SQLite +
// FTS5 via modernc.org/sqlite (CGo-free, ADR-004). One store file per
// projection; personal and work-safe never share files (ADR-005).
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
	_ "modernc.org/sqlite"
)

// Store is a projection store. Fully rebuildable from authorized sources;
// never canonical.
type Store struct {
	db   *sql.DB
	path string
}

const schemaMigrations = `
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS records (
  record_id        TEXT PRIMARY KEY,
  source_id        TEXT NOT NULL,
  source_record_id TEXT NOT NULL,
  kind             TEXT NOT NULL,
  decision_key     TEXT NOT NULL DEFAULT '',
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
);
`

// Open creates/opens a projection store at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store dir: %w", err)
	}
	// Restrictive file permissions (blueprint §7.7).
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single-writer discipline
	if _, err := db.Exec(schemaMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the store file path.
func (s *Store) Path() string { return s.path }

// Rebuildable marker: Wipe deletes all derived content (FR-004).
func (s *Store) Wipe() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"records", "records_fts", "provenance"} {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PutRecords stores normalized records + provenance atomically.
func (s *Store) PutRecords(recs []contracts.Record, provs []contracts.Provenance) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range recs {
		payload, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO records
			(record_id, source_id, source_record_id, kind, decision_key, payload, sensitivity, status, authority, source_role, trust, criticality)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			r.RecordID, r.SourceID, r.SourceRecordID, string(r.Kind), r.DecisionKey, string(payload),
			r.Sensitivity, string(r.Status), string(r.Authority), string(r.SourceRole), string(r.Trust), r.Criticality); err != nil {
			return fmt.Errorf("put record %s: %w", r.RecordID, err)
		}
		body := r.Title + " " + r.Statement + " " + r.CompactText + " " + r.Key
		if _, err := tx.Exec(`INSERT INTO records_fts (record_id, body) VALUES (?, ?)`,
			r.RecordID, body); err != nil {
			return err
		}
	}
	for _, p := range provs {
		payload, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO provenance
			(provenance_id, source_id, source_record_id, payload) VALUES (?,?,?,?)`,
			p.ProvenanceID, p.SourceID, p.SourceRecordID, string(payload)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Records returns all records (deterministic order by record_id).
func (s *Store) Records() []contracts.Record {
	rows, err := s.db.Query(`SELECT payload FROM records ORDER BY record_id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []contracts.Record{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return out
		}
		var r contracts.Record
		if err := json.Unmarshal([]byte(payload), &r); err == nil {
			out = append(out, r)
		}
	}
	return out
}

// Provenance fetches one provenance record.
func (s *Store) Provenance(id string) (contracts.Provenance, bool) {
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM provenance WHERE provenance_id = ?`, id).Scan(&payload)
	if err != nil {
		return contracts.Provenance{}, false
	}
	var p contracts.Provenance
	if json.Unmarshal([]byte(payload), &p) != nil {
		return contracts.Provenance{}, false
	}
	return p, true
}

// SearchFTS runs a bounded FTS5 query over the pack-ready text.
func (s *Store) SearchFTS(query string, limit int) []string {
	if limit <= 0 {
		limit = 20
	}
	q := sanitizeFTS(query)
	if q == "" {
		return nil
	}
	rows, err := s.db.Query(`SELECT record_id FROM records_fts WHERE records_fts MATCH ? LIMIT ?`, q, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

// sanitizeFTS strips operators and quotes to keep MATCH safe and bounded.
func sanitizeFTS(q string) string {
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r > 127)
	})
	words := []string{}
	for _, f := range fields {
		if len(f) < 2 {
			continue
		}
		words = append(words, f+"*")
	}
	return strings.Join(words, " ")
}

// Tombstone marks revocation/forget (FR-026). key is a record ID or
// "source:<source_id>".
func (s *Store) Tombstone(key, reason string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO tombstones (key, reason, at) VALUES (?,?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`, key, reason)
	return err
}

// RevokedSet returns all tombstoned keys.
func (s *Store) RevokedSet() map[string]bool {
	rows, err := s.db.Query(`SELECT key FROM tombstones`)
	if err != nil {
		return map[string]bool{}
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			out[k] = true
		}
	}
	return out
}

// Count returns record count (health/status).
func (s *Store) Count() int {
	var n int
	if s.db.QueryRow(`SELECT count(*) FROM records`).Scan(&n) != nil {
		return 0
	}
	return n
}

// SetMeta/getMeta manage store metadata (index revision, digests).
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES (?,?)`, key, value)
	return err
}

func (s *Store) GetMeta(key string) (string, bool) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	return v, err == nil
}
