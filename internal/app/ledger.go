package app

// Durable tombstone ledger (ADR-027). Projection stores are derived and can
// be wiped, recovered from corruption, rolled back, or restored from a
// backup — any of which loses the tombstones stored inside them. The ledger
// lives under the canonical root (operator-owned configuration, never inside
// the data dir), so restoring or rebuilding derived data cannot resurrect
// forgotten or purged content.
//
// Minimality (ADR-027): purge entries are keyed HMAC-SHA256 fingerprints of
// record identity (source ID + record ID) and of the purge key — nothing
// else. No content or text hashes (private data is often low-entropy and an
// unkeyed hash is a dictionary oracle), no plain IDs, no timestamps, no
// reasons; entries are stored sorted so order reveals no chronology. The key
// lives in a separate file, so a leaked or committed ledger alone cannot be
// tested against guesses.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
)

const ledgerSchemaVersion = "2"

// ErrLedgerUnusable: the tombstone ledger (or the purge key it depends on)
// cannot be read. Every read surface and rebuild fails closed on it.
var ErrLedgerUnusable = errors.New("tombstone ledger unusable")

// ErrPurgeKeyMissing: the ledger holds purge fingerprints but the key needed
// to recognize them is missing or unreadable. Resolution, rebuild, and purge
// fail closed until it is restored.
var ErrPurgeKeyMissing = errors.New("purge ledger key missing or unreadable: purged records cannot be recognized, so resolution and rebuild fail closed — restore ledger/purge.key from the same backup as ledger/tombstones.json")

// Ledger is the durable anti-resurrection record.
type Ledger struct {
	SchemaVersion string             `json:"schema_version"`
	Revocations   []LedgerRevocation `json:"revocations"`
	// Purges are opaque keyed fingerprints ("hmac-sha256:<hex>").
	Purges []string `json:"purges"`

	key []byte
	set map[string]bool
}

// LedgerRevocation mirrors a logical forget (the content is still on disk,
// so its key is not secret beyond the store). Profile is empty for all
// profiles.
type LedgerRevocation struct {
	Key     string `json:"key"`
	Profile string `json:"profile,omitempty"`
	At      string `json:"at"`
}

func (rt *Runtime) ledgerDir() string { return filepath.Join(rt.Config.CanonicalRoot, "ledger") }

// LedgerPath returns the ledger location under the canonical root.
func (rt *Runtime) LedgerPath() string { return filepath.Join(rt.ledgerDir(), "tombstones.json") }

// PurgeKeyPath returns the ledger HMAC key location.
func (rt *Runtime) PurgeKeyPath() string { return filepath.Join(rt.ledgerDir(), "purge.key") }

// LoadLedger reads the ledger. A missing ledger is empty; an unreadable one,
// or purge entries without their key, is an error so callers fail closed.
func (rt *Runtime) LoadLedger() (*Ledger, error) {
	l := &Ledger{SchemaVersion: ledgerSchemaVersion}
	data, err := os.ReadFile(rt.LedgerPath())
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: unreadable: %v", ErrLedgerUnusable, err)
	}
	var raw struct {
		SchemaVersion string             `json:"schema_version"`
		Revocations   []LedgerRevocation `json:"revocations"`
		Purges        json.RawMessage    `json:"purges"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: corrupt: %v", ErrLedgerUnusable, err)
	}
	l.Revocations = raw.Revocations
	switch raw.SchemaVersion {
	case ledgerSchemaVersion:
		if len(raw.Purges) > 0 && string(raw.Purges) != "null" {
			if err := json.Unmarshal(raw.Purges, &l.Purges); err != nil {
				return nil, fmt.Errorf("%w: corrupt: %v", ErrLedgerUnusable, err)
			}
		}
	case "1":
		// Pre-release format: revocations are compatible; v1 purge entries
		// carried content-derived hashes and cannot be converted.
		if len(raw.Purges) > 0 && string(raw.Purges) != "null" && string(raw.Purges) != "[]" {
			return nil, fmt.Errorf("%w: v1 purge entries are unsupported (they held content-derived hashes): re-run the purge with the current version after removing them", ErrLedgerUnusable)
		}
	default:
		return nil, fmt.Errorf("%w: schema %q unsupported", ErrLedgerUnusable, raw.SchemaVersion)
	}
	for _, fp := range l.Purges {
		if !strings.HasPrefix(fp, "hmac-sha256:") {
			return nil, fmt.Errorf("%w: corrupt: purge entry is not a keyed fingerprint", ErrLedgerUnusable)
		}
	}
	if len(l.Purges) > 0 {
		key, err := rt.readPurgeKey()
		if err != nil {
			return nil, fmt.Errorf("%w: %w (%v)", ErrLedgerUnusable, ErrPurgeKeyMissing, err)
		}
		l.key = key
	}
	return l, nil
}

func (rt *Runtime) readPurgeKey() ([]byte, error) {
	data, err := os.ReadFile(rt.PurgeKeyPath())
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != 32 {
		return nil, errors.New("purge key is not 32 hex-encoded bytes")
	}
	return key, nil
}

// ensureKey loads the ledger key, creating it (random, 0600, durably) on
// first use. A missing or invalid key is replaced only while the ledger holds
// no purge fingerprints — otherwise existing tombstones would stop matching.
func (l *Ledger) ensureKey(rt *Runtime) error {
	if l.key != nil {
		return nil
	}
	key, err := rt.readPurgeKey()
	if err == nil {
		l.key = key
		return nil
	}
	if len(l.Purges) > 0 {
		return fmt.Errorf("%w: %w (%v)", ErrLedgerUnusable, ErrPurgeKeyMissing, err)
	}
	if err := rt.ensureLedgerDir(); err != nil {
		return err
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	encoded := []byte(hex.EncodeToString(key) + "\n")
	if errors.Is(err, os.ErrNotExist) {
		err = createFileDurable(rt.PurgeKeyPath(), encoded, 0o600)
	} else {
		// unreadable or truncated key with no dependent fingerprints
		err = writeFileDurable(rt.PurgeKeyPath(), encoded, 0o600)
	}
	if err != nil {
		return err
	}
	l.key = key
	return nil
}

// ledgerIgnoreRules must always be present in ledger/.gitignore so a
// Git-tracked canonical root never commits the key or pending journals.
var ledgerIgnoreRules = []string{"purge.key", "pending/"}

// ensureLedgerDir creates the ledger dir and makes sure ledger/.gitignore
// ignores the key and pending journals, appending missing rules to an existing
// file without touching unrelated rules.
func (rt *Runtime) ensureLedgerDir() error {
	if err := ensureDirDurable(rt.ledgerDir()); err != nil {
		return err
	}
	ignore := filepath.Join(rt.ledgerDir(), ".gitignore")
	existing, err := os.ReadFile(ignore)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read ledger .gitignore: %w", err)
	}
	merged, changed := mergeIgnoreRules(string(existing), ledgerIgnoreRules)
	if !changed {
		return nil
	}
	return writeFileDurable(ignore, []byte(merged), 0o600)
}

// mergeIgnoreRules appends each rule not already present. Rules compare after
// trimming a leading or trailing "/", so "/purge.key" and "pending" count as
// present; unrelated lines are preserved byte for byte.
func mergeIgnoreRules(existing string, rules []string) (string, bool) {
	norm := func(r string) string { return strings.Trim(strings.TrimSpace(r), "/") }
	have := map[string]bool{}
	for _, line := range strings.Split(existing, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") {
			have[norm(t)] = true
		}
	}
	out := existing
	changed := false
	for _, r := range rules {
		if have[norm(r)] {
			continue
		}
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += r + "\n"
		have[norm(r)] = true
		changed = true
	}
	return out, changed
}

func (rt *Runtime) saveLedger(l *Ledger) error {
	if err := rt.ensureLedgerDir(); err != nil {
		return err
	}
	l.SchemaVersion = ledgerSchemaVersion
	sort.Strings(l.Purges)
	if l.Revocations == nil {
		l.Revocations = []LedgerRevocation{}
	}
	if l.Purges == nil {
		l.Purges = []string{}
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return writeFileDurable(rt.LedgerPath(), data, 0o600)
}

func (l *Ledger) mac(parts ...string) string {
	h := hmac.New(sha256.New, l.key)
	h.Write([]byte("beme-purge-v2"))
	for _, p := range parts {
		h.Write([]byte{0})
		h.Write([]byte(p))
	}
	return "hmac-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (l *Ledger) recordFingerprint(sourceID, recordID string) string {
	return l.mac("record", sourceID, recordID)
}

func (l *Ledger) keyFingerprint(key string) string { return l.mac("key", key) }

func (l *Ledger) has(fp string) bool {
	if l.set == nil {
		l.set = make(map[string]bool, len(l.Purges))
		for _, p := range l.Purges {
			l.set[p] = true
		}
	}
	return l.set[fp]
}

// Purged reports whether a record was physically purged — by its identity or
// because its whole source was purged. Content is never compared.
func (l *Ledger) Purged(rec contracts.Record) bool {
	if len(l.Purges) == 0 || l.key == nil {
		return false
	}
	return l.has(l.recordFingerprint(rec.SourceID, rec.RecordID)) || l.has(l.keyFingerprint("source:"+rec.SourceID))
}

// PurgedKey reports whether a purge with this exact key completed its ledger
// write.
func (l *Ledger) PurgedKey(key string) bool {
	if len(l.Purges) == 0 || l.key == nil {
		return false
	}
	return l.has(l.keyFingerprint(key))
}

func (l *Ledger) addRevocation(key, profile string) {
	for _, r := range l.Revocations {
		if r.Key == key && r.Profile == profile {
			return
		}
	}
	l.Revocations = append(l.Revocations, LedgerRevocation{Key: key, Profile: profile, At: nowUTC()})
}

func (l *Ledger) addPurge(fp string) {
	if l.has(fp) {
		return
	}
	l.Purges = append(l.Purges, fp)
	l.set[fp] = true
}

// dropRevocations removes plain revocation keys superseded by a purge, so the
// ledger does not keep a readable ID of purged content.
func (l *Ledger) dropRevocations(keys map[string]bool) {
	kept := l.Revocations[:0]
	for _, r := range l.Revocations {
		if !keys[r.Key] {
			kept = append(kept, r)
		}
	}
	l.Revocations = kept
}
