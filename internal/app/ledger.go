package app

// Durable tombstone ledger (ADR-027). Projection stores are derived and can
// be wiped, recovered from corruption, rolled back, or restored from a
// backup — any of which loses the tombstones stored inside them. The ledger
// lives under the canonical root (operator-owned configuration, never inside
// the data dir), so restoring or rebuilding derived data cannot resurrect
// forgotten or purged content.
//
// The ledger never holds content: revocations carry the record/source key
// (an identifier), purges carry only SHA-256 fingerprints.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
)

// Ledger is the durable anti-resurrection record.
type Ledger struct {
	SchemaVersion string             `json:"schema_version"`
	Revocations   []LedgerRevocation `json:"revocations"`
	Purges        []LedgerPurge      `json:"purges"`
}

// LedgerRevocation mirrors a logical forget. Profile is empty for all
// profiles.
type LedgerRevocation struct {
	Key     string `json:"key"`
	Profile string `json:"profile,omitempty"`
	At      string `json:"at"`
}

// LedgerPurge is a non-content physical-purge tombstone.
type LedgerPurge struct {
	RecordFingerprint string `json:"record_fingerprint"`
	ContentHash       string `json:"content_hash,omitempty"`
	// TextFingerprint hashes the normalized statement, so the same content
	// re-keyed under a new ID or with a changed header is still refused.
	TextFingerprint string `json:"text_fingerprint,omitempty"`
	At              string `json:"at"`
}

// LedgerPath returns the ledger location under the canonical root.
func (rt *Runtime) LedgerPath() string {
	return filepath.Join(rt.Config.CanonicalRoot, "ledger", "tombstones.json")
}

// LoadLedger reads the ledger. A missing ledger is empty; an unreadable one
// is an error so callers fail closed.
func (rt *Runtime) LoadLedger() (*Ledger, error) {
	l := &Ledger{SchemaVersion: contracts.SchemaVersion}
	data, err := os.ReadFile(rt.LedgerPath())
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tombstone ledger unreadable: %w", err)
	}
	if err := json.Unmarshal(data, l); err != nil {
		return nil, fmt.Errorf("tombstone ledger corrupt: %w", err)
	}
	return l, nil
}

func (rt *Runtime) saveLedger(l *Ledger) error {
	path := rt.LedgerPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RecordFingerprint is the non-content purge fingerprint of a record ID.
func RecordFingerprint(recordID string) string {
	sum := sha256.Sum256([]byte("beme-purge-v1\x00" + recordID))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// TextFingerprint is the non-content fingerprint of a record's normalized
// statement text ("" when the record has no usable text).
func TextFingerprint(rec contracts.Record) string {
	text := rec.Statement
	if strings.TrimSpace(text) == "" {
		text = rec.CompactText
	}
	n := normalizeText(text)
	if len(n) < 12 {
		return ""
	}
	sum := sha256.Sum256([]byte("beme-purge-text-v1\x00" + n))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Purged reports whether a record was physically purged, matching by record
// ID, source content hash, or normalized statement text.
func (l *Ledger) Purged(rec contracts.Record, contentHash string) bool {
	fp := RecordFingerprint(rec.RecordID)
	tfp := TextFingerprint(rec)
	for _, p := range l.Purges {
		if p.RecordFingerprint == fp {
			return true
		}
		if contentHash != "" && p.ContentHash == contentHash {
			return true
		}
		if tfp != "" && p.TextFingerprint == tfp {
			return true
		}
	}
	return false
}

func (l *Ledger) addRevocation(key, profile string) {
	for _, r := range l.Revocations {
		if r.Key == key && r.Profile == profile {
			return
		}
	}
	l.Revocations = append(l.Revocations, LedgerRevocation{Key: key, Profile: profile, At: nowUTC()})
}

func (l *Ledger) addPurge(recordID, contentHash, textFingerprint string) {
	fp := RecordFingerprint(recordID)
	for _, p := range l.Purges {
		if p.RecordFingerprint == fp && p.ContentHash == contentHash && p.TextFingerprint == textFingerprint {
			return
		}
	}
	l.Purges = append(l.Purges, LedgerPurge{RecordFingerprint: fp, ContentHash: contentHash, TextFingerprint: textFingerprint, At: nowUTC()})
}
