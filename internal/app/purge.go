package app

// Forget and physical purge (blueprint §7.8, FR-026/FR-055, ADR-027).
//
//   - Forget: logical tombstone, recorded in the store AND the durable ledger.
//   - PhysicalPurge: RED, irreversible, idempotent, resumable. It writes the
//     keyed anti-resurrection fingerprints first, then a content-free journal
//     of the planned deletions — both durably (durable.go) — and only then
//     erases the record from every projection
//     store (with on-disk erasure), persisted traces, pending observations,
//     and (optionally) canonical source files. Every step tolerates work
//     already done, so re-running the same purge after a partial failure
//     completes the remaining cleanup; a completed purge reports
//     already_purged. Git history and external backups are outside Be Me's
//     reach and are reported as explicit residuals.
//
// Durability (ADR-030): every file a purge removes is zeroized, flushed,
// unlinked, and its directory flushed (durable.Erase) before the journal is
// finalized; a retry re-flushes the directories of files an earlier attempt
// already removed. Projection stores are compacted with synchronous=FULL and
// their files and directory flushed. Forget, purge, build, and learning
// writes are serialized by the maintenance lock.

import (
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
	"github.com/0merUfuk/beme/internal/durable"
	"github.com/0merUfuk/beme/internal/learning"
	"github.com/0merUfuk/beme/internal/storage"
)

var (
	// ErrPurgeNotConfirmed: the typed confirmation did not match the key.
	ErrPurgeNotConfirmed = errors.New("physical purge is irreversible (RED): --confirm must repeat the exact key")
	// ErrPurgeNotFound: no projection holds the key and it was never purged.
	ErrPurgeNotFound = errors.New("nothing to purge: key not present in any projection")
)

// Purge stages, in execution order. Stages that act on several items
// (projection/compact per profile store, trace, observation, canonical per
// file) invoke PurgeRequest.FailAt once per item; projection and compact
// stages are suffixed ":<profile>".
const (
	StageLedger      = "ledger"
	StageJournal     = "journal"
	StageProjection  = "projection"
	StageCompact     = "compact"
	StageTrace       = "trace"
	StageObservation = "observation"
	StageCanonical   = "canonical"
	StageFinalize    = "finalize"
)

// PurgeStages lists every stage in execution order.
var PurgeStages = []string{StageLedger, StageJournal, StageProjection, StageCompact, StageTrace, StageObservation, StageCanonical, StageFinalize}

var allProfiles = []contracts.Profile{contracts.ProfilePersonal, contracts.ProfileWorkSafe}

// Forget tombstones a record or source for one profile, durably.
func (rt *Runtime) Forget(profile contracts.Profile, key, reason string) error {
	lk, err := rt.lock()
	if err != nil {
		return err
	}
	defer lk.Release()
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

// PurgeRequest describes one physical purge.
type PurgeRequest struct {
	Key             string // record ID or "source:<source_id>"
	Confirm         string // must equal Key
	RemoveCanonical bool   // also delete the canonical source file(s)
	DryRun          bool
	// FailAt is a verification hook: when set, it is called before each
	// deletion step (see the Stage constants) and a returned error aborts
	// the purge at that point, leaving the resumable journal in place. The
	// CLI and MCP surfaces never set it.
	FailAt func(stage string) error
}

// PurgeStep is one reported step. Reports never carry content.
type PurgeStep struct {
	Step    string `json:"step"`
	Outcome string `json:"outcome"` // done | planned | skipped
	Count   int    `json:"count"`
}

// PurgeReport is the result of a purge.
type PurgeReport struct {
	Key           string      `json:"key"`
	DryRun        bool        `json:"dry_run"`
	Resumed       bool        `json:"resumed"`
	AlreadyPurged bool        `json:"already_purged"`
	Records       int         `json:"records"`
	Steps         []PurgeStep `json:"steps"`
	Residuals     []string    `json:"residuals"`
}

// purgeJournal is the transient, resumable plan. It holds identifiers only —
// record/source IDs, provenance refs, relative locators, trace file names,
// observation IDs — never record text. It lives under
// <canonical_root>/ledger/pending/ and is removed when the purge completes.
type purgeJournal struct {
	SchemaVersion   string          `json:"schema_version"`
	RemoveCanonical bool            `json:"remove_canonical"`
	Records         []journalRecord `json:"records"`
	Traces          []string        `json:"traces"`
	Observations    []string        `json:"observations"`
}

type journalRecord struct {
	RecordID       string   `json:"record_id"`
	SourceID       string   `json:"source_id"`
	SourceRecordID string   `json:"source_record_id"`
	ProvenanceRefs []string `json:"provenance_refs"`
	Locators       []string `json:"locators"`
	Profiles       []string `json:"profiles"`
}

func (rt *Runtime) pendingDir() string { return filepath.Join(rt.ledgerDir(), "pending") }

func (rt *Runtime) journalPath(key string) string {
	sum := sha256.Sum256([]byte("beme-purge-journal\x00" + key))
	return filepath.Join(rt.pendingDir(), hex.EncodeToString(sum[:12])+".json")
}

// PendingPurges counts interrupted purges awaiting a resuming run. An
// uninspectable journal directory is an error, never "none pending": doctor
// and the CLI must not report a clean deployment over state they could not
// read.
func (rt *Runtime) PendingPurges() (int, error) { return rt.pendingJournals() }

func (rt *Runtime) loadJournal(key string) (*purgeJournal, bool, error) {
	data, err := os.ReadFile(rt.journalPath(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("pending purge journal unreadable: %w", err)
	}
	var j purgeJournal
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, false, fmt.Errorf("pending purge journal corrupt (remove %s and re-run the purge): %w", rt.journalPath(key), err)
	}
	return &j, true, nil
}

func (rt *Runtime) saveJournal(key string, j *purgeJournal) error {
	if err := rt.ensureLedgerDir(); err != nil {
		return err
	}
	if err := durable.EnsureDir(rt.pendingDir()); err != nil {
		return err
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return durable.WriteFile(rt.journalPath(key), data, 0o600)
}

// PhysicalPurge executes (or resumes) the RED physical-purge workflow.
func (rt *Runtime) PhysicalPurge(req PurgeRequest) (*PurgeReport, error) {
	if req.Key == "" || req.Confirm != req.Key {
		return nil, ErrPurgeNotConfirmed
	}
	fail := req.FailAt
	if fail == nil {
		fail = func(string) error { return nil }
	}
	lk, err := rt.lock()
	if err != nil {
		return nil, err
	}
	defer lk.Release()
	ledger, err := rt.LoadLedger()
	if err != nil {
		return nil, err
	}

	plan, resumed, err := rt.loadJournal(req.Key)
	if err != nil {
		return nil, err
	}
	if !resumed {
		if plan, err = rt.planPurge(req.Key); err != nil {
			return nil, err
		}
		if len(plan.Records) == 0 {
			if ledger.PurgedKey(req.Key) {
				return &PurgeReport{Key: req.Key, DryRun: req.DryRun, AlreadyPurged: true, Steps: []PurgeStep{}, Residuals: []string{}}, nil
			}
			return nil, ErrPurgeNotFound
		}
	}
	plan.RemoveCanonical = plan.RemoveCanonical || req.RemoveCanonical

	rep := &PurgeReport{Key: req.Key, DryRun: req.DryRun, Resumed: resumed, Records: len(plan.Records), Residuals: []string{}}
	outcome := "done"
	if req.DryRun {
		outcome = "planned"
	}
	ids := make([]string, 0, len(plan.Records))
	for _, r := range plan.Records {
		ids = append(ids, r.RecordID)
	}

	// 1. Anti-resurrection fingerprints first: from here on the content can
	// never resolve or be re-ingested, even if the process dies.
	if !req.DryRun {
		if err := fail(StageLedger); err != nil {
			return rep, err
		}
		if err := ledger.ensureKey(rt); err != nil {
			return rep, err
		}
		superseded := map[string]bool{req.Key: true}
		ledger.addPurge(ledger.keyFingerprint(req.Key))
		for _, r := range plan.Records {
			ledger.addPurge(ledger.recordFingerprint(r.SourceID, r.RecordID))
			superseded[r.RecordID] = true
		}
		// Observation identities too: a restored copy of a removed
		// observation file stays hidden on every learning surface.
		for _, id := range plan.Observations {
			ledger.addObservation(id)
		}
		ledger.dropRevocations(superseded)
		if err := rt.saveLedger(ledger); err != nil {
			return rep, fmt.Errorf("write tombstone ledger: %w", err)
		}
	}
	rep.Steps = append(rep.Steps, PurgeStep{Step: "anti_resurrection_tombstone", Outcome: outcome, Count: len(plan.Records)})

	// 2. Journal: a failure after this point resumes from the saved plan.
	if !req.DryRun {
		if err := fail(StageJournal); err != nil {
			return rep, err
		}
		if err := rt.saveJournal(req.Key, plan); err != nil {
			return rep, fmt.Errorf("write purge journal: %w", err)
		}
	}

	// 3. Projection stores, with on-disk erasure.
	targets := make([]storage.PurgeTarget, 0, len(plan.Records))
	planProfiles := map[string]bool{}
	for _, r := range plan.Records {
		targets = append(targets, storage.PurgeTarget{RecordID: r.RecordID, SourceID: r.SourceID, SourceRecordID: r.SourceRecordID, ProvenanceRefs: r.ProvenanceRefs})
		for _, p := range r.Profiles {
			planProfiles[p] = true
		}
	}
	for _, p := range allProfiles {
		path := rt.ProjectionPath(p)
		exists, err := durable.Exists(path)
		if err != nil {
			return rep, fmt.Errorf("inspect %s projection: %w", p, err)
		}
		if !exists {
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
		if n == 0 && !planProfiles[string(p)] {
			store.Close()
			continue
		}
		if !req.DryRun {
			if err := fail(StageProjection + ":" + string(p)); err != nil {
				store.Close()
				return rep, err
			}
			if _, err := store.PurgeRecords(targets); err != nil {
				store.Close()
				return rep, fmt.Errorf("purge %s projection: %w", p, err)
			}
			if err := fail(StageCompact + ":" + string(p)); err != nil {
				store.Close()
				return rep, err
			}
			if err := store.Compact(); err != nil {
				store.Close()
				return rep, fmt.Errorf("compact %s projection: %w", p, err)
			}
		}
		store.Close()
		rep.Steps = append(rep.Steps, PurgeStep{Step: "derived_purge_projection_" + string(p), Outcome: outcome, Count: n})
	}

	// 4. Persisted resolver traces: planned files plus any naming a target
	// that appeared since planning.
	traceDir := filepath.Join(rt.Config.CacheDir, "traces")
	traces := 0
	current, err := rt.tracesNaming(ids)
	if err != nil {
		return rep, fmt.Errorf("inspect traces: %w", err)
	}
	for _, name := range unionSorted(plan.Traces, current) {
		path := filepath.Join(traceDir, filepath.Base(name))
		exists, err := durable.Exists(path)
		if err != nil {
			return rep, fmt.Errorf("inspect trace: %w", err)
		}
		if exists {
			traces++
		}
		if req.DryRun {
			continue
		}
		if exists {
			if err := fail(StageTrace); err != nil {
				return rep, err
			}
		}
		// Erase also when already absent: a retry flushes the directory
		// entry an earlier attempt removed but could not flush.
		if _, err := durable.Erase(path); err != nil {
			return rep, fmt.Errorf("remove trace: %w", err)
		}
	}
	rep.Steps = append(rep.Steps, PurgeStep{Step: "derived_purge_traces", Outcome: outcome, Count: traces})

	// 5. Pending observations that restate the content (matched at planning,
	// when the store was fully inspected). A store that cannot be inspected
	// aborts the purge; the step is never reported done.
	obsRemoved := 0
	if len(plan.Observations) > 0 {
		obsDir := filepath.Join(rt.Config.DataDir, "observations")
		exists, err := durable.Exists(obsDir)
		if err != nil {
			return rep, fmt.Errorf("inspect observations: %w", err)
		}
		if exists {
			obsStore, err := learning.Open(rt.Config.DataDir)
			if err != nil {
				return rep, fmt.Errorf("inspect observations: %w", err)
			}
			for _, id := range plan.Observations {
				present, err := obsStore.Exists(id)
				if err != nil {
					return rep, fmt.Errorf("inspect observation: %w", err)
				}
				if present {
					obsRemoved++
				}
				if req.DryRun {
					continue
				}
				if present {
					if err := fail(StageObservation); err != nil {
						return rep, err
					}
				}
				if _, err := obsStore.Remove(id); err != nil {
					return rep, fmt.Errorf("remove observation: %w", err)
				}
			}
		}
	}
	rep.Steps = append(rep.Steps, PurgeStep{Step: "derived_purge_observations", Outcome: outcome, Count: obsRemoved})

	// 6. Canonical source files (only when requested).
	descriptors := map[string]contracts.SourceDescriptor{}
	for _, sd := range rt.Sources {
		descriptors[sd.SourceID] = sd
	}
	gitSources := map[string]bool{}
	if plan.RemoveCanonical {
		removed := 0
		for _, r := range plan.Records {
			sd, ok := descriptors[r.SourceID]
			if !ok || len(r.Locators) == 0 {
				rep.Residuals = append(rep.Residuals, "canonical file for "+r.RecordID+" not located (source no longer registered or no locator): remove it manually")
				continue
			}
			if sd.Type == "git_repository" {
				gitSources[sd.SourceID] = true
			}
			for _, loc := range r.Locators {
				path, err := containedPath(sd.Root, loc)
				switch {
				case errors.Is(err, errLocatorEscapes):
					// refusing to delete outside the registered root is a
					// policy decision, reported as a residual
					rep.Residuals = append(rep.Residuals, "canonical file for "+r.RecordID+" not removed: "+err.Error())
					continue
				case errors.Is(err, errSourceRootGone):
					continue // the registered root no longer exists: nothing left there
				case err != nil:
					return rep, fmt.Errorf("inspect canonical file for %s: %w", r.RecordID, err)
				}
				exists, err := durable.Exists(path)
				if err != nil {
					return rep, fmt.Errorf("inspect canonical file for %s: %w", r.RecordID, err)
				}
				if exists {
					removed++
				}
				if req.DryRun {
					continue
				}
				if exists {
					if err := fail(StageCanonical); err != nil {
						return rep, err
					}
				}
				_, err = durable.Erase(path)
				switch {
				case errors.Is(err, durable.ErrMultipleLinks), errors.Is(err, durable.ErrNotRegular):
					// the content is shared with names Be Me was not asked
					// to erase: report instead of zeroizing them
					removed--
					rep.Residuals = append(rep.Residuals, "canonical file for "+r.RecordID+" not removed: "+err.Error()+"; remove every link yourself")
				case err != nil:
					return rep, fmt.Errorf("remove canonical file for %s: %w", r.RecordID, err)
				}
			}
		}
		rep.Steps = append(rep.Steps, PurgeStep{Step: "canonical_source_removed", Outcome: outcome, Count: removed})
	} else {
		rep.Steps = append(rep.Steps, PurgeStep{Step: "canonical_source_removed", Outcome: "skipped", Count: 0})
		rep.Residuals = append(rep.Residuals, "canonical source files retained (run with --remove-canonical or delete them yourself); rebuilds will not re-ingest them")
		for _, r := range plan.Records {
			if sd, ok := descriptors[r.SourceID]; ok && sd.Type == "git_repository" {
				gitSources[sd.SourceID] = true
			}
		}
	}
	gitIDs := make([]string, 0, len(gitSources))
	for id := range gitSources {
		gitIDs = append(gitIDs, id)
	}
	sort.Strings(gitIDs)
	for _, id := range gitIDs {
		rep.Residuals = append(rep.Residuals, "source "+id+" is a Git repository: history still contains the content; rewrite history (e.g. git filter-repo), expire reflogs, and force-push — Be Me does not rewrite Git history")
	}

	// 7. Finalize: the purge is complete only once the journal is gone.
	if !req.DryRun {
		if err := fail(StageFinalize); err != nil {
			return rep, err
		}
		if err := durable.Remove(rt.journalPath(req.Key)); err != nil {
			return rep, fmt.Errorf("remove purge journal: %w", err)
		}
	}
	rep.Residuals = append(rep.Residuals, "backups and sync copies outside Be Me are not erased; restored stores stay filtered by the tombstone ledger")
	return rep, nil
}

// planPurge locates every record matching key across projections and
// resolves everything the purge must remove, while the content is still
// available for matching. Only identifiers are kept.
func (rt *Runtime) planPurge(key string) (*purgeJournal, error) {
	plan := &purgeJournal{SchemaVersion: "1", Records: []journalRecord{}, Traces: []string{}, Observations: []string{}}
	byID := map[string]*journalRecord{}
	texts := map[string][]string{}
	for _, p := range allProfiles {
		path := rt.ProjectionPath(p)
		exists, err := durable.Exists(path)
		if err != nil {
			return nil, fmt.Errorf("inspect %s projection: %w", p, err)
		}
		if !exists {
			continue
		}
		store, err := storage.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s projection: %w", p, err)
		}
		recs, err := store.AllRecords()
		if err != nil {
			store.Close()
			return nil, fmt.Errorf("read %s projection: %w", p, err)
		}
		for _, rec := range recs {
			match := rec.RecordID == key
			if src, ok := strings.CutPrefix(key, "source:"); ok {
				match = rec.SourceID == src
			}
			if !match {
				continue
			}
			jr := byID[rec.RecordID]
			if jr == nil {
				jr = &journalRecord{RecordID: rec.RecordID, SourceID: rec.SourceID, SourceRecordID: rec.SourceRecordID,
					ProvenanceRefs: []string{}, Locators: []string{}, Profiles: []string{}}
				byID[rec.RecordID] = jr
			}
			jr.Profiles = addUnique(jr.Profiles, string(p))
			for _, ref := range rec.ProvenanceRefs {
				jr.ProvenanceRefs = addUnique(jr.ProvenanceRefs, ref)
				if prov, ok := store.Provenance(ref); ok && prov.Locator != "" {
					jr.Locators = addUnique(jr.Locators, prov.Locator)
				}
			}
			for _, s := range []string{rec.Title, rec.Statement, rec.CompactText} {
				if len(normalizeText(s)) >= 12 {
					texts[rec.RecordID] = append(texts[rec.RecordID], s)
				}
			}
		}
		store.Close()
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		plan.Records = append(plan.Records, *byID[id])
	}
	if len(ids) == 0 {
		return plan, nil
	}
	traces, err := rt.tracesNaming(ids)
	if err != nil {
		return nil, fmt.Errorf("inspect traces: %w", err)
	}
	plan.Traces = traces
	obsDir := filepath.Join(rt.Config.DataDir, "observations")
	exists, err := durable.Exists(obsDir)
	if err != nil {
		return nil, fmt.Errorf("inspect observations: %w", err)
	}
	if exists {
		obsStore, err := learning.Open(rt.Config.DataDir)
		if err != nil {
			return nil, fmt.Errorf("inspect observations: %w", err)
		}
		all, err := obsStore.ListAll()
		if err != nil {
			return nil, fmt.Errorf("inspect observations: %w", err)
		}
		for _, obs := range all {
			hay := strings.Join([]string{obs.Hypothesis, obs.SupportingEvidence, obs.Counterevidence, obs.Task}, "\n")
			if mentionsAny(hay, ids, texts) {
				plan.Observations = append(plan.Observations, obs.ObservationID)
			}
		}
	}
	return plan, nil
}

// tracesNaming lists persisted trace files that reference any record ID.
// An unreadable trace directory or file is an error, never "no traces".
func (rt *Runtime) tracesNaming(ids []string) ([]string, error) {
	out := []string{}
	traceDir := filepath.Join(rt.Config.CacheDir, "traces")
	entries, err := os.ReadDir(traceDir)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(traceDir, e.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if strings.Contains(string(data), `"`+id+`"`) {
				out = append(out, e.Name())
				break
			}
		}
	}
	return out, nil
}

func mentionsAny(hay string, ids []string, texts map[string][]string) bool {
	h := normalizeText(hay)
	for _, id := range ids {
		if strings.Contains(hay, id) {
			return true
		}
		for _, s := range texts[id] {
			if n := normalizeText(s); len(n) >= 12 && strings.Contains(h, n) {
				return true
			}
		}
	}
	// The observation may restate a record in a shorter form.
	for _, line := range strings.Split(hay, "\n") {
		n := normalizeText(line)
		if len(n) < 12 {
			continue
		}
		for _, id := range ids {
			for _, s := range texts[id] {
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

func addUnique(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func unionSorted(a, b []string) []string {
	set := map[string]bool{}
	for _, v := range append(append([]string{}, a...), b...) {
		set[v] = true
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

var (
	errLocatorEscapes = errors.New("locator escapes source root")
	errSourceRootGone = errors.New("source root no longer exists")
)

// containedPath joins a provenance locator to a source root and refuses any
// result that escapes the root (including via symlinks). Inspection failures
// (e.g. permission denied) are returned as errors, never as "absent".
func containedPath(root, locator string) (string, error) {
	if filepath.IsAbs(locator) || strings.Contains(filepath.ToSlash(locator), "../") {
		return "", errLocatorEscapes
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if errors.Is(err, os.ErrNotExist) {
		return "", errSourceRootGone
	}
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	path := filepath.Join(root, filepath.FromSlash(locator))
	realPath, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errLocatorEscapes
	}
	return realPath, nil
}
