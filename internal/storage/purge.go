package storage

// Physical purge support (blueprint §7.8, FR-055). Logical forget only
// tombstones; these functions remove derived content and then rewrite the
// store file so deleted bytes do not survive in free pages, FTS segments,
// or the write-ahead log.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/durable"
)

// PurgeTarget names one record to erase. Provenance is resolved from the
// record's actual payload, never from an ID convention; the fields here let a
// resumed purge still remove provenance whose record row is already gone.
type PurgeTarget struct {
	RecordID       string
	SourceID       string
	SourceRecordID string
	ProvenanceRefs []string
}

// RecordIDsForSource returns the IDs of every record ingested from a source.
func (s *Store) RecordIDsForSource(sourceID string) []string {
	rows, err := s.db.Query(`SELECT record_id FROM records WHERE source_id = ? ORDER BY record_id`, sourceID)
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

// HasRecord reports whether a record ID is present in the store.
func (s *Store) HasRecord(id string) bool {
	var n int
	if s.db.QueryRow(`SELECT count(*) FROM records WHERE record_id = ?`, id).Scan(&n) != nil {
		return false
	}
	return n > 0
}

// PurgeRecords deletes each target's record row, FTS rows, plain-key
// tombstone, and every provenance row it owns, in one transaction. Owned
// provenance is the union of:
//   - the ProvenanceRefs in the stored record payload,
//   - the ProvenanceRefs carried by the target (from a purge plan), and
//   - provenance rows with the record's (source_id, source_record_id).
//
// It is idempotent: absent rows are skipped. It returns the number of record
// rows removed. Call Compact afterwards to erase the freed bytes.
func (s *Store) PurgeRecords(targets []PurgeTarget) (int, error) {
	if len(targets) == 0 {
		return 0, nil
	}
	// secure_delete zeroes freed content as it is released (per connection;
	// the store uses a single connection).
	if _, err := s.db.Exec(`PRAGMA secure_delete = ON`); err != nil {
		return 0, fmt.Errorf("secure_delete: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	removed := 0
	for _, t := range targets {
		refs := map[string]bool{}
		for _, r := range t.ProvenanceRefs {
			if r != "" {
				refs[r] = true
			}
		}
		sourceID, sourceRecordID := t.SourceID, t.SourceRecordID
		var payload, rowSource, rowSourceRecord string
		err := tx.QueryRow(`SELECT payload, source_id, source_record_id FROM records WHERE record_id = ?`, t.RecordID).
			Scan(&payload, &rowSource, &rowSourceRecord)
		switch {
		case err == nil:
			sourceID, sourceRecordID = rowSource, rowSourceRecord
			var rec contracts.Record
			// An unreadable payload still purges: the column match below
			// covers its provenance.
			if json.Unmarshal([]byte(payload), &rec) == nil {
				for _, r := range rec.ProvenanceRefs {
					if r != "" {
						refs[r] = true
					}
				}
			}
		case errors.Is(err, sql.ErrNoRows):
			// already removed (resumed purge) — still clean up leftovers
		default:
			return 0, err
		}

		res, err := tx.Exec(`DELETE FROM records WHERE record_id = ?`, t.RecordID)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed++
		}
		if _, err := tx.Exec(`DELETE FROM records_fts WHERE record_id = ?`, t.RecordID); err != nil {
			return 0, err
		}
		for ref := range refs {
			if _, err := tx.Exec(`DELETE FROM provenance WHERE provenance_id = ?`, ref); err != nil {
				return 0, err
			}
		}
		if sourceID != "" && sourceRecordID != "" {
			if _, err := tx.Exec(`DELETE FROM provenance WHERE source_id = ? AND source_record_id = ?`, sourceID, sourceRecordID); err != nil {
				return 0, err
			}
		}
		// The anti-resurrection tombstone lives in the durable ledger as a
		// keyed fingerprint; the plain-key tombstone is dropped with the content.
		if _, err := tx.Exec(`DELETE FROM tombstones WHERE key = ?`, t.RecordID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return removed, nil
}

// Compact rewrites the store so purged content does not survive on disk:
// FTS5 segments are merged (dropping deleted postings), the database file is
// rebuilt with VACUUM, and the WAL is checkpointed and truncated — all with
// synchronous=FULL — and then the database file, the WAL, and their directory
// are flushed explicitly, so the rewrite reaches stable storage before a
// purge can finalize. Idempotent: a retry repeats every flush.
func (s *Store) Compact() error {
	if _, err := s.db.Exec(`PRAGMA synchronous=FULL`); err != nil {
		return fmt.Errorf("synchronous: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO records_fts(records_fts) VALUES('optimize')`); err != nil {
		return fmt.Errorf("fts optimize: %w", err)
	}
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("wal checkpoint: %w", err)
	}
	for _, f := range []string{s.path, s.path + "-wal"} {
		if err := durable.SyncFile(f); err != nil {
			return fmt.Errorf("flush store: %w", err)
		}
	}
	if err := durable.SyncDir(filepath.Dir(s.path)); err != nil {
		return fmt.Errorf("flush store: %w", err)
	}
	return nil
}
