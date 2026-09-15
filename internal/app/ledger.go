package app

// Durable tombstone ledger (ADR-027, ADR-030). Projection stores are derived
// and can be wiped, recovered from corruption, rolled back, or restored from a
// backup — any of which loses the tombstones stored inside them. The ledger
// lives under the canonical root (operator-owned configuration, never inside
// the data dir), so restoring or rebuilding derived data cannot resurrect
// forgotten or purged content.
//
// Minimality (ADR-027): purge entries are keyed HMAC-SHA256 fingerprints of
// record identity (source ID + record ID), of the purge key, and of purged
// observation IDs — nothing else. No content or text hashes (private data is
// often low-entropy and an unkeyed hash is a dictionary oracle), no plain IDs,
// no timestamps, no reasons; entries are stored sorted so order reveals no
// chronology. The key lives in a separate file, so a leaked or committed
// ledger alone cannot be tested against guesses.
//
// Integrity (ADR-030): enforcement state is two files, ledger/tombstones.json
// and ledger/purge.key. The key file carries a key ID, the generation of the
// last committed ledger write, and whether the first write committed; the
// ledger carries the same key ID and its own generation. Loading fails closed
// when one file is missing while the other proves it existed, when they
// belong to different keys, when the ledger is older than the key's recorded
// generation (partial rollback), or when a pending purge journal exists
// without a ledger. An interrupted first write (key created, ledger not yet
// written) is recognizable and recovers. Undetectable locally: removing or
// rolling back BOTH files together (and every pending journal), because the
// resulting state is indistinguishable from an older or fresh deployment.

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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/durable"
)

const (
	ledgerSchemaVersion = "3"
	keySchemaVersion    = "3"
)

// ErrLedgerUnusable: the tombstone ledger (or the purge key it depends on)
// cannot be read or fails its integrity checks. Every read surface, learning
// surface, and rebuild fails closed on it.
var ErrLedgerUnusable = errors.New("tombstone ledger unusable")

// ErrPurgeKeyMissing: the ledger depends on a purge key that is missing or
// unreadable. Resolution, rebuild, and purge fail closed until it is restored.
var ErrPurgeKeyMissing = errors.New("purge ledger key missing or unreadable: purged records cannot be recognized, so resolution and rebuild fail closed — restore ledger/purge.key from the same backup as ledger/tombstones.json")

// fingerprintPattern is the only accepted ledger entry shape.
var fingerprintPattern = regexp.MustCompile(`^hmac-sha256:[0-9a-f]{64}$`)

// Ledger is the durable anti-resurrection record.
type Ledger struct {
	SchemaVersion string `json:"schema_version"`
	// KeyID binds the ledger to its purge key (a digest of the key, not the
	// key). Generation increases with every write.
	KeyID       string             `json:"key_id"`
	Generation  uint64             `json:"generation"`
	Revocations []LedgerRevocation `json:"revocations"`
	// Purges are opaque keyed fingerprints of record identities and purge
	// keys ("hmac-sha256:<64 hex>").
	Purges []string `json:"purges"`
	// Observations are keyed fingerprints of purged observation IDs.
	Observations []string `json:"observations"`

	key     []byte
	keyFile *purgeKeyFile
	set     map[string]bool
	obsSet  map[string]bool
}

// LedgerRevocation mirrors a logical forget (the content is still on disk,
// so its key is not secret beyond the store). Profile is empty for all
// profiles.
type LedgerRevocation struct {
	Key     string `json:"key"`
	Profile string `json:"profile,omitempty"`
	At      string `json:"at"`
}

// purgeKeyFile is the on-disk key record. Legacy (pre-v3) keys are a bare
// hex line.
type purgeKeyFile struct {
	SchemaVersion string `json:"schema_version"`
	Key           string `json:"key"`
	KeyID         string `json:"key_id"`
	Generation    uint64 `json:"generation"`
	Committed     bool   `json:"committed"`

	legacy bool
	raw    []byte
}

func (rt *Runtime) ledgerDir() string { return filepath.Join(rt.Config.CanonicalRoot, "ledger") }

// LedgerPath returns the ledger location under the canonical root.
func (rt *Runtime) LedgerPath() string { return filepath.Join(rt.ledgerDir(), "tombstones.json") }

// PurgeKeyPath returns the ledger HMAC key location.
func (rt *Runtime) PurgeKeyPath() string { return filepath.Join(rt.ledgerDir(), "purge.key") }

func (rt *Runtime) lockPath() string { return filepath.Join(rt.ledgerDir(), ".lock") }

func keyID(key []byte) string {
	sum := sha256.Sum256(append([]byte("beme-purge-key-id\x00"), key...))
	return hex.EncodeToString(sum[:16])
}

func unusable(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrLedgerUnusable}, args...)...)
}

func keyMissing(detail any) error {
	return fmt.Errorf("%w: %w (%v)", ErrLedgerUnusable, ErrPurgeKeyMissing, detail)
}

// readKeyFile returns the key record, an error wrapping os.ErrNotExist when
// absent, or a descriptive error when unreadable or malformed.
func (rt *Runtime) readKeyFile() (*purgeKeyFile, error) {
	data, err := os.ReadFile(rt.PurgeKeyPath())
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	kf := &purgeKeyFile{}
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal([]byte(trimmed), kf); err != nil {
			return nil, errors.New("purge key file is corrupt")
		}
		if kf.SchemaVersion != keySchemaVersion {
			return nil, fmt.Errorf("purge key schema %q unsupported", kf.SchemaVersion)
		}
	} else {
		kf.Key, kf.legacy = trimmed, true
	}
	key, err := hex.DecodeString(kf.Key)
	if err != nil || len(key) != 32 || kf.Key != strings.ToLower(kf.Key) {
		return nil, errors.New("purge key is not 32 hex-encoded bytes")
	}
	id := keyID(key)
	if kf.legacy {
		kf.KeyID = id
	} else if kf.KeyID != id {
		return nil, errors.New("purge key file is corrupt: key ID does not match the key")
	}
	kf.raw = key
	return kf, nil
}

// LoadLedger reads and verifies the enforcement state. It fails closed
// (ErrLedgerUnusable) on anything it cannot prove consistent.
func (rt *Runtime) LoadLedger() (*Ledger, error) {
	l := &Ledger{SchemaVersion: ledgerSchemaVersion}
	kf, kerr := rt.readKeyFile()
	keyAbsent := errors.Is(kerr, os.ErrNotExist)
	if kerr == nil {
		l.key, l.keyFile = kf.raw, kf
	}

	data, err := os.ReadFile(rt.LedgerPath())
	if errors.Is(err, os.ErrNotExist) {
		pending, perr := rt.pendingJournals()
		if perr != nil {
			return nil, unusable("pending purge journals uninspectable: %v", perr)
		}
		switch {
		case pending > 0:
			return nil, unusable("ledger/tombstones.json is missing but %d pending purge journal(s) exist: restore the ledger from the same backup as ledger/pending/", pending)
		case keyAbsent:
			return &Ledger{SchemaVersion: ledgerSchemaVersion}, nil
		case kerr != nil:
			return nil, keyMissing(kerr)
		case kf.legacy || kf.Committed:
			return nil, unusable("ledger/tombstones.json is missing but ledger/purge.key records a committed ledger: restore both files from the same backup")
		}
		// Interrupted first write: the key was created, the ledger never
		// committed. Nothing was enforced yet; the key is reused.
		return l, nil
	}
	if err != nil {
		return nil, unusable("unreadable: %v", err)
	}

	var raw struct {
		SchemaVersion string             `json:"schema_version"`
		KeyID         string             `json:"key_id"`
		Generation    uint64             `json:"generation"`
		Revocations   []LedgerRevocation `json:"revocations"`
		Purges        []string           `json:"purges"`
		Observations  []string           `json:"observations"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, unusable("corrupt: %v", err)
	}
	l.Revocations, l.Purges, l.Observations = raw.Revocations, raw.Purges, raw.Observations
	for _, fp := range append(append([]string{}, l.Purges...), l.Observations...) {
		if !fingerprintPattern.MatchString(fp) {
			return nil, unusable("corrupt: entry is not a full keyed fingerprint")
		}
	}
	committedKey := kerr == nil && !kf.legacy && kf.Committed

	switch raw.SchemaVersion {
	case ledgerSchemaVersion:
		if raw.KeyID == "" || raw.Generation == 0 {
			return nil, unusable("corrupt: key binding missing")
		}
		if kerr != nil {
			return nil, keyMissing(kerr)
		}
		if kf.legacy {
			return nil, unusable("ledger/purge.key is older than ledger/tombstones.json: restore both files from the same backup")
		}
		if kf.KeyID != raw.KeyID {
			return nil, unusable("ledger/purge.key belongs to a different ledger: restore both files from the same backup")
		}
		if raw.Generation < kf.Generation {
			return nil, unusable("ledger/tombstones.json (generation %d) is older than ledger/purge.key records (generation %d): it was rolled back or partially restored — restore both files from the same backup", raw.Generation, kf.Generation)
		}
		l.KeyID, l.Generation = raw.KeyID, raw.Generation
	case "2", "1":
		if len(l.Observations) > 0 {
			return nil, unusable("corrupt: schema %s cannot hold observation entries", raw.SchemaVersion)
		}
		if raw.SchemaVersion == "1" && len(l.Purges) > 0 {
			// v1 purge entries carried content-derived hashes.
			return nil, unusable("v1 purge entries are unsupported (they held content-derived hashes): re-run the purge with the current version after removing them")
		}
		if committedKey {
			return nil, unusable("ledger/tombstones.json predates ledger/purge.key: it was rolled back or partially restored — restore both files from the same backup")
		}
		if len(l.Purges) > 0 && kerr != nil {
			return nil, keyMissing(kerr)
		}
		if kerr != nil && !keyAbsent {
			return nil, keyMissing(kerr)
		}
	default:
		return nil, unusable("schema %q unsupported", raw.SchemaVersion)
	}
	return l, nil
}

// pendingJournals counts pending purge journals, failing on an uninspectable
// directory.
func (rt *Runtime) pendingJournals() (int, error) {
	entries, err := os.ReadDir(rt.pendingDir())
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n, nil
}

// ensureKey makes sure a purge key exists before fingerprints are computed:
// a new key is written uncommitted (step 1 of the first-write protocol), and
// a legacy bare-hex key is rewritten in the v3 format with its bytes kept so
// existing fingerprints still match.
func (l *Ledger) ensureKey(rt *Runtime) error {
	if err := rt.ensureLedgerDir(); err != nil {
		return err
	}
	if l.keyFile != nil && !l.keyFile.legacy {
		return nil
	}
	if l.keyFile != nil {
		kf := &purgeKeyFile{SchemaVersion: keySchemaVersion, Key: l.keyFile.Key, KeyID: l.keyFile.KeyID, raw: l.keyFile.raw}
		if err := writeKeyFile(rt, kf, false); err != nil {
			return err
		}
		l.keyFile = kf
		return nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	kf := &purgeKeyFile{SchemaVersion: keySchemaVersion, Key: hex.EncodeToString(key), KeyID: keyID(key), raw: key}
	if err := writeKeyFile(rt, kf, true); err != nil {
		return err
	}
	l.key, l.keyFile = key, kf
	return nil
}

func writeKeyFile(rt *Runtime, kf *purgeKeyFile, create bool) error {
	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if create {
		return durable.CreateExclusive(rt.PurgeKeyPath(), data, 0o600)
	}
	return durable.WriteFile(rt.PurgeKeyPath(), data, 0o600)
}

// ledgerIgnoreRules must always be present in ledger/.gitignore so a
// Git-tracked canonical root never commits the key, pending journals, or the
// maintenance lock.
var ledgerIgnoreRules = []string{"purge.key", "pending/", ".lock"}

// ensureLedgerDir creates the ledger dir and makes sure ledger/.gitignore
// ignores the key, pending journals, and lock, appending missing rules to an
// existing file without touching unrelated rules.
func (rt *Runtime) ensureLedgerDir() error {
	if err := durable.EnsureDir(rt.ledgerDir()); err != nil {
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
	return durable.WriteFile(ignore, []byte(merged), 0o600)
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

// saveLedger commits the ledger: key present (uncommitted on first use),
// ledger written with the next generation, then the key file records that
// generation as committed. A crash between the last two steps leaves a
// ledger newer than the key, which loads normally. Callers hold the
// maintenance lock.
func (rt *Runtime) saveLedger(l *Ledger) error {
	if err := l.ensureKey(rt); err != nil {
		return err
	}
	l.SchemaVersion = ledgerSchemaVersion
	l.KeyID = l.keyFile.KeyID
	if l.keyFile.Generation > l.Generation {
		l.Generation = l.keyFile.Generation
	}
	l.Generation++
	sort.Strings(l.Purges)
	sort.Strings(l.Observations)
	if l.Revocations == nil {
		l.Revocations = []LedgerRevocation{}
	}
	if l.Purges == nil {
		l.Purges = []string{}
	}
	if l.Observations == nil {
		l.Observations = []string{}
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := durable.WriteFile(rt.LedgerPath(), data, 0o600); err != nil {
		return err
	}
	committed := *l.keyFile
	committed.Generation, committed.Committed = l.Generation, true
	if err := writeKeyFile(rt, &committed, false); err != nil {
		return fmt.Errorf("commit purge key generation: %w", err)
	}
	*l.keyFile = committed
	return nil
}

// maintenanceLockTimeout bounds how long a maintenance operation waits for
// another one to finish.
var maintenanceLockTimeout = 2 * time.Minute

// lock takes the exclusive maintenance lock that serializes every writer of
// enforcement and derived state (forget, purge, build, learning writes)
// across processes, so none loses another's ledger update or re-ingests
// content a concurrent purge is erasing.
func (rt *Runtime) lock() (*durable.Lock, error) {
	if err := rt.ensureLedgerDir(); err != nil {
		return nil, fmt.Errorf("maintenance lock: %w", err)
	}
	lk, err := durable.AcquireLock(rt.lockPath(), maintenanceLockTimeout)
	if err != nil {
		return nil, fmt.Errorf("maintenance lock: %w", err)
	}
	return lk, nil
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

func (l *Ledger) observationFingerprint(id string) string { return l.mac("observation", id) }

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

// HiddenObservation reports whether an observation ID was physically purged.
// A restored copy of its file stays invisible on every learning surface.
func (l *Ledger) HiddenObservation(id string) bool {
	if len(l.Observations) == 0 || l.key == nil {
		return false
	}
	if l.obsSet == nil {
		l.obsSet = make(map[string]bool, len(l.Observations))
		for _, p := range l.Observations {
			l.obsSet[p] = true
		}
	}
	return l.obsSet[l.observationFingerprint(id)]
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

func (l *Ledger) addObservation(id string) {
	if l.HiddenObservation(id) {
		return
	}
	fp := l.observationFingerprint(id)
	l.Observations = append(l.Observations, fp)
	if l.obsSet != nil {
		l.obsSet[fp] = true
	}
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
