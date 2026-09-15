package app

// Forget and physical purge (blueprint §7.8, FR-026/FR-055, ADR-027).
//
//   - Forget: logical tombstone, recorded in the store AND the durable ledger.
//   - PhysicalPurge: RED, irreversible. Removes the content from every
//     projection store (with on-disk erasure), persisted traces, and pending
//     observations; optionally deletes the canonical source file; leaves only
//     a non-content fingerprint in the ledger so rebuild, sync, rollback, or a
//     restored backup cannot resurrect it. Git history and external backups
//     are outside Be Me's reach and are reported as explicit residuals.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/learning"
	"github.com/0merUfuk/beme/internal/storage"
)

var (
	// ErrPurgeNotConfirmed: the typed confirmation did not match the key.
	ErrPurgeNotConfirmed = errors.New("physical purge is irreversible (RED): --confirm must repeat the exact key")
	// ErrPurgeNotFound: no projection holds the key.
	ErrPurgeNotFound = errors.New("nothing to purge: key not present in any projection")
)

// Forget tombstones a record or source for one profile, durably.
func (rt *Runtime) Forget(profile contracts.Profile, key, reason string) error {
	l, err := rt.LoadLedger()
	if err != nil {
		return err
	}
	l.addRevocation(key, string(profile))
	if err := rt.saveLedger(l); err != nil {
		return err
	}
	store, err := storage.Open(rt.ProjectionPath(profile))
	if err != nil {
		return err
	}
	defer store.Close()
	return store.Tombstone(key, reason)
}

// EffectiveRevoked merges store tombstones with the durable ledger and marks
// any record matching a purge fingerprint. restoredPurged reports whether the
// store still holds purged records (e.g. restored from a backup).
func (rt *Runtime) EffectiveRevoked(profile contracts.Profile, store *storage.Store) (revoked map[string]bool, restoredPurged bool, err error) {
	revoked = store.RevokedSet()
	l, err := rt.LoadLedger()
	if err != nil {
		return nil, false, err
	}
	for _, r := range l.Revocations {
		if r.Profile == "" || r.Profile == string(profile) {
			revoked[r.Key] = true
		}
	}
	if len(l.Purges) > 0 {
		for _, rec := range store.Records() {
			if l.Purged(rec, contentHashOf(store, rec)) {
				revoked[rec.RecordID] = true
				restoredPurged = true
			}
		}
	}
	return revoked, restoredPurged, nil
}

func contentHashOf(store *storage.Store, rec contracts.Record) string {
	for _, ref := range rec.ProvenanceRefs {
		if p, ok := store.Provenance(ref); ok {
			return p.ContentHash
		}
	}
	return ""
}

// PurgeRequest describes one physical purge.
type PurgeRequest struct {
	Key             string // record ID or "source:<source_id>"
	Confirm         string // must equal Key
	RemoveCanonical bool   // also delete the canonical source file(s)
	DryRun          bool
}

// PurgeStep is one reported step. Reports never carry content.
type PurgeStep struct {
	Step    string `json:"step"`
	Outcome string `json:"outcome"` // done | planned | skipped
	Count   int    `json:"count"`
}

// PurgeReport is the result of a purge.
type PurgeReport struct {
	Key       string      `json:"key"`
	DryRun    bool        `json:"dry_run"`
	Records   int         `json:"records"`
	Steps     []PurgeStep `json:"steps"`
	Residuals []string    `json:"residuals"`
}

type purgeTarget struct {
	recordID    string
	sourceID    string
	locator     string
	contentHash string
	textFP      string
	texts       []string
}

// PhysicalPurge executes the RED physical-purge workflow.
func (rt *Runtime) PhysicalPurge(req PurgeRequest) (*PurgeReport, error) {
	if req.Key == "" || req.Confirm != req.Key {
		return nil, ErrPurgeNotConfirmed
	}
	profiles := []contracts.Profile{contracts.ProfilePersonal, contracts.ProfileWorkSafe}

	// 1. Locate every target across projections (read-only).
	targets := map[string]*purgeTarget{}
	for _, p := range profiles {
		path := rt.ProjectionPath(p)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		store, err := storage.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s projection: %w", p, err)
		}
		for _, rec := range store.Records() {
			match := rec.RecordID == req.Key
			if src, ok := strings.CutPrefix(req.Key, "source:"); ok {
				match = rec.SourceID == src
			}
			if !match || targets[rec.RecordID] != nil {
				continue
			}
			t := &purgeTarget{recordID: rec.RecordID, sourceID: rec.SourceID, textFP: TextFingerprint(rec)}
			for _, ref := range rec.ProvenanceRefs {
				if prov, ok := store.Provenance(ref); ok {
					t.locator, t.contentHash = prov.Locator, prov.ContentHash
					break
				}
			}
			for _, s := range []string{rec.Title, rec.Statement, rec.CompactText} {
				if len(strings.TrimSpace(s)) >= 12 {
					t.texts = append(t.texts, s)
				}
			}
			targets[rec.RecordID] = t
		}
		store.Close()
	}
	if len(targets) == 0 {
		return nil, ErrPurgeNotFound
	}
	ids := make([]string, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	sortStrings(ids)

	rep := &PurgeReport{Key: req.Key, DryRun: req.DryRun, Records: len(ids)}
	outcome := "done"
	if req.DryRun {
		outcome = "planned"
	}

	// 2. Anti-resurrection tombstone first: a crash after this point can
	// leave content on disk but never resolvable or re-ingestable.
	if !req.DryRun {
		l, err := rt.LoadLedger()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			l.addPurge(id, targets[id].contentHash, targets[id].textFP)
		}
		if err := rt.saveLedger(l); err != nil {
			return nil, fmt.Errorf("write tombstone ledger: %w", err)
		}
	}
	rep.Steps = append(rep.Steps, PurgeStep{Step: "anti_resurrection_tombstone", Outcome: outcome, Count: len(ids)})

	// 3. Derived purge: projection stores, with on-disk erasure.
	for _, p := range profiles {
		path := rt.ProjectionPath(p)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		store, err := storage.Open(path)
		if err != nil {
			return rep, fmt.Errorf("open %s projection: %w", p, err)
		}
		n := 0
		for _, id := range ids {
			if store.HasRecord(id) {
				n++
			}
		}
		if !req.DryRun && n > 0 {
			if _, err := store.PurgeRecords(ids); err != nil {
				store.Close()
				return rep, fmt.Errorf("purge %s projection: %w", p, err)
			}
			if err := store.Compact(); err != nil {
				store.Close()
				return rep, fmt.Errorf("compact %s projection: %w", p, err)
			}
		}
		store.Close()
		rep.Steps = append(rep.Steps, PurgeStep{Step: "derived_purge_projection_" + string(p), Outcome: outcome, Count: n})
	}

	// 4. Derived purge: persisted resolver traces naming a purged record.
	traceDir := filepath.Join(rt.Config.CacheDir, "traces")
	traces := 0
	if entries, err := os.ReadDir(traceDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			fp := filepath.Join(traceDir, e.Name())
			data, err := os.ReadFile(fp)
			if err != nil {
				continue
			}
			for _, id := range ids {
				if strings.Contains(string(data), `"`+id+`"`) {
					traces++
					if !req.DryRun {
						if err := os.Remove(fp); err != nil {
							return rep, fmt.Errorf("remove trace: %w", err)
						}
					}
					break
				}
			}
		}
	}
	rep.Steps = append(rep.Steps, PurgeStep{Step: "derived_purge_traces", Outcome: outcome, Count: traces})

	// 5. Derived purge: pending observations that restate purged content.
	obsRemoved := 0
	if obsStore, err := learning.Open(rt.Config.DataDir); err == nil {
		for _, obs := range obsStore.List("") {
			hay := strings.Join([]string{obs.Hypothesis, obs.SupportingEvidence, obs.Counterevidence, obs.Task}, "\n")
			if !mentionsAny(hay, targets) {
				continue
			}
			obsRemoved++
			if !req.DryRun {
				if err := obsStore.Remove(obs.ObservationID); err != nil {
					return rep, fmt.Errorf("remove observation: %w", err)
				}
			}
		}
	}
	rep.Steps = append(rep.Steps, PurgeStep{Step: "derived_purge_observations", Outcome: outcome, Count: obsRemoved})

	// 6. Canonical source files (only when explicitly requested).
	descriptors := map[string]contracts.SourceDescriptor{}
	for _, sd := range rt.Sources {
		descriptors[sd.SourceID] = sd
	}
	gitSources := map[string]bool{}
	if req.RemoveCanonical {
		removed := 0
		for _, id := range ids {
			t := targets[id]
			sd, ok := descriptors[t.sourceID]
			if !ok || t.locator == "" {
				rep.Residuals = append(rep.Residuals, "canonical file for "+id+" not located (source no longer registered): remove it manually")
				continue
			}
			path, err := containedPath(sd.Root, t.locator)
			if err != nil {
				rep.Residuals = append(rep.Residuals, "canonical file for "+id+" not removed: "+err.Error())
				continue
			}
			if sd.Type == "git_repository" {
				gitSources[sd.SourceID] = true
			}
			if _, err := os.Stat(path); err != nil {
				continue // already gone
			}
			removed++
			if !req.DryRun {
				if err := os.Remove(path); err != nil {
					return rep, fmt.Errorf("remove canonical file for %s: %w", id, err)
				}
			}
		}
		rep.Steps = append(rep.Steps, PurgeStep{Step: "canonical_source_removed", Outcome: outcome, Count: removed})
	} else {
		rep.Steps = append(rep.Steps, PurgeStep{Step: "canonical_source_removed", Outcome: "skipped", Count: 0})
		rep.Residuals = append(rep.Residuals, "canonical source files retained (run with --remove-canonical or delete them yourself); rebuilds will not re-ingest them")
		for _, id := range ids {
			if sd, ok := descriptors[targets[id].sourceID]; ok && sd.Type == "git_repository" {
				gitSources[sd.SourceID] = true
			}
		}
	}
	gitIDs := make([]string, 0, len(gitSources))
	for id := range gitSources {
		gitIDs = append(gitIDs, id)
	}
	sortStrings(gitIDs)
	for _, id := range gitIDs {
		rep.Residuals = append(rep.Residuals, "source "+id+" is a Git repository: history still contains the content; rewrite history (e.g. git filter-repo), expire reflogs, and force-push — Be Me does not rewrite Git history")
	}
	rep.Residuals = append(rep.Residuals, "backups and sync copies outside Be Me are not erased; restored stores stay filtered by the tombstone ledger")
	return rep, nil
}

func mentionsAny(hay string, targets map[string]*purgeTarget) bool {
	h := normalizeText(hay)
	for id, t := range targets {
		if strings.Contains(hay, id) {
			return true
		}
		for _, s := range t.texts {
			n := normalizeText(s)
			if len(n) >= 12 && strings.Contains(h, n) {
				return true
			}
		}
	}
	// The observation may restate a record in a shorter form: check each
	// observation line against the record texts too.
	for _, line := range strings.Split(hay, "\n") {
		n := normalizeText(line)
		if len(n) < 12 {
			continue
		}
		for _, t := range targets {
			for _, s := range t.texts {
				if strings.Contains(normalizeText(s), n) {
					return true
				}
			}
		}
	}
	return false
}

// normalizeText lowercases, collapses whitespace, and trims surrounding
// punctuation so restatements match regardless of formatting.
func normalizeText(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	return strings.Trim(s, " .,;:!?\"'`")
}

// containedPath joins a provenance locator to a source root and refuses any
// result that escapes the root (including via symlinks).
func containedPath(root, locator string) (string, error) {
	if filepath.IsAbs(locator) || strings.Contains(filepath.ToSlash(locator), "../") {
		return "", errors.New("locator escapes source root")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("source root unavailable")
	}
	path := filepath.Join(root, filepath.FromSlash(locator))
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
		return "", err
	}
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("locator escapes source root")
	}
	return realPath, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
