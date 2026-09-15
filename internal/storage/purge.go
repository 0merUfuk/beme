package storage

// Physical purge support (blueprint §7.8, FR-055). Logical forget only
// tombstones; these functions remove derived content and then rewrite the
// store file so deleted bytes do not survive in free pages, FTS segments,
// or the write-ahead log.

import (
	"fmt"
)

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

// PurgeRecords deletes records, their FTS rows, their provenance, and any
// plain-key tombstones naming them, in one transaction. It returns the number
// of records removed. Call Compact afterwards to erase the freed bytes.
func (s *Store) PurgeRecords(ids []string) (int, error) {
	if len(ids) == 0 {
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
	for _, id := range ids {
		var provRefs string
		if err := tx.QueryRow(`SELECT payload FROM records WHERE record_id = ?`, id).Scan(&provRefs); err != nil {
			continue // not in this store
		}
		res, err := tx.Exec(`DELETE FROM records WHERE record_id = ?`, id)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed++
		}
		if _, err := tx.Exec(`DELETE FROM records_fts WHERE record_id = ?`, id); err != nil {
			return 0, err
		}
		// Provenance IDs mirror record IDs ("prov_" + suffix of "rec_").
		if len(id) > len("rec_") {
			if _, err := tx.Exec(`DELETE FROM provenance WHERE provenance_id = ?`, "prov_"+id[len("rec_"):]); err != nil {
				return 0, err
			}
		}
		// The anti-resurrection tombstone lives in the durable ledger as a
		// fingerprint; the plain-key tombstone is dropped with the content.
		if _, err := tx.Exec(`DELETE FROM tombstones WHERE key = ?`, id); err != nil {
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
// rebuilt with VACUUM, and the WAL is checkpointed and truncated.
func (s *Store) Compact() error {
	if _, err := s.db.Exec(`INSERT INTO records_fts(records_fts) VALUES('optimize')`); err != nil {
		return fmt.Errorf("fts optimize: %w", err)
	}
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("wal checkpoint: %w", err)
	}
	return nil
}
