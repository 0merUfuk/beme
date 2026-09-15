package storage

// Strict reads: surface every query, scan, and decode error. Read surfaces
// and purge planning must never mistake a failed read for an empty store or
// an empty tombstone set.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/0merUfuk/beme/internal/contracts"
)

// AllRecords returns every record in record_id order.
func (s *Store) AllRecords() ([]contracts.Record, error) {
	rows, err := s.db.Query(`SELECT payload FROM records ORDER BY record_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Record{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var r contracts.Record
		if err := json.Unmarshal([]byte(payload), &r); err != nil {
			return nil, fmt.Errorf("record payload corrupt: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecordByID fetches one record; ok is false when it does not exist.
func (s *Store) RecordByID(id string) (rec contracts.Record, ok bool, err error) {
	var payload string
	err = s.db.QueryRow(`SELECT payload FROM records WHERE record_id = ?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.Record{}, false, nil
	}
	if err != nil {
		return contracts.Record{}, false, err
	}
	if err := json.Unmarshal([]byte(payload), &rec); err != nil {
		return contracts.Record{}, false, fmt.Errorf("record payload corrupt: %w", err)
	}
	return rec, true, nil
}

// RevokedKeys returns all tombstoned keys.
func (s *Store) RevokedKeys() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT key FROM tombstones`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}
