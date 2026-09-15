package app

// Read surfaces (ADR-027 "read surfaces"). Every surface that can reveal
// projection content, provenance, counts, or metadata — resolve/preview,
// export, explain, MCP status and get_context_item, doctor — reads through
// these functions. They apply store tombstones and the durable ledger at read
// time, so a restored backup containing purged or revoked records cannot
// surface them, and they fail closed (ErrLedgerUnusable) when the ledger or
// the purge key cannot be read.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
	"github.com/0merUfuk/beme/internal/storage"
)

// ErrTraceUnavailable: the trace does not exist, is unreadable, or was
// rejected. Callers cannot tell these apart.
var ErrTraceUnavailable = errors.New("trace not available")

type visibility struct {
	revoked map[string]bool
	ledger  *Ledger
}

func (rt *Runtime) loadVisibility(profile contracts.Profile, store *storage.Store) (*visibility, error) {
	ledger, err := rt.LoadLedger()
	if err != nil {
		return nil, err
	}
	revoked, err := store.RevokedKeys()
	if err != nil {
		return nil, fmt.Errorf("read tombstones: %w", err)
	}
	for _, r := range ledger.Revocations {
		if r.Profile == "" || r.Profile == string(profile) {
			revoked[r.Key] = true
		}
	}
	return &visibility{revoked: revoked, ledger: ledger}, nil
}

func (v *visibility) visible(rec contracts.Record) bool {
	if v.revoked[rec.RecordID] || v.revoked["source:"+rec.SourceID] {
		return false
	}
	return !v.ledger.Purged(rec)
}

// EffectiveRevoked merges store tombstones with the durable ledger and marks
// any record whose identity matches a purge fingerprint. restoredPurged
// reports whether the store still holds purged records (e.g. restored from a
// backup).
func (rt *Runtime) EffectiveRevoked(profile contracts.Profile, store *storage.Store) (revoked map[string]bool, restoredPurged bool, err error) {
	vis, err := rt.loadVisibility(profile, store)
	if err != nil {
		return nil, false, err
	}
	revoked = make(map[string]bool, len(vis.revoked))
	for k := range vis.revoked {
		revoked[k] = true
	}
	if len(vis.ledger.Purges) > 0 {
		recs, err := store.AllRecords()
		if err != nil {
			return nil, false, err
		}
		for _, rec := range recs {
			if vis.ledger.Purged(rec) {
				revoked[rec.RecordID] = true
				restoredPurged = true
			}
		}
	}
	return revoked, restoredPurged, nil
}

// VisibleRecords returns the session's records minus revoked and purged ones.
func (s *Session) VisibleRecords() ([]contracts.Record, error) {
	vis, err := s.Runtime.loadVisibility(s.Capability.Profile, s.Store)
	if err != nil {
		return nil, err
	}
	recs, err := s.Store.AllRecords()
	if err != nil {
		return nil, err
	}
	out := []contracts.Record{}
	for _, rec := range recs {
		if vis.visible(rec) {
			out = append(out, rec)
		}
	}
	return out, nil
}

// VisibleCount is the record count safe to report (MCP status).
func (s *Session) VisibleCount() (int, error) {
	recs, err := s.VisibleRecords()
	if err != nil {
		return 0, err
	}
	return len(recs), nil
}

// ExportRecord is one exported record with the provenance it owns.
type ExportRecord struct {
	Record     contracts.Record       `json:"record"`
	Provenance []contracts.Provenance `json:"provenance"`
}

// ProjectionExport is the filtered export of one projection.
type ProjectionExport struct {
	Profile string         `json:"profile"`
	Records []ExportRecord `json:"records"`
}

// ExportProjection exports visible records and their provenance only.
func (rt *Runtime) ExportProjection(profile contracts.Profile) (*ProjectionExport, error) {
	if profile != contracts.ProfilePersonal && profile != contracts.ProfileWorkSafe {
		return nil, fmt.Errorf("invalid profile %q", profile)
	}
	out := &ProjectionExport{Profile: string(profile), Records: []ExportRecord{}}
	// The ledger gates every export, even of an unbuilt projection.
	if _, err := rt.LoadLedger(); err != nil {
		return nil, err
	}
	exists, err := statExists(rt.ProjectionPath(profile))
	if err != nil {
		return nil, err
	}
	if !exists {
		return out, nil
	}
	store, err := storage.Open(rt.ProjectionPath(profile))
	if err != nil {
		return nil, err
	}
	defer store.Close()
	vis, err := rt.loadVisibility(profile, store)
	if err != nil {
		return nil, err
	}
	recs, err := store.AllRecords()
	if err != nil {
		return nil, err
	}
	for _, rec := range recs {
		if !vis.visible(rec) {
			continue
		}
		er := ExportRecord{Record: rec, Provenance: []contracts.Provenance{}}
		for _, ref := range rec.ProvenanceRefs {
			if p, ok := store.Provenance(ref); ok {
				er.Provenance = append(er.Provenance, p)
			}
		}
		out.Records = append(out.Records, er)
	}
	return out, nil
}

// LoadTrace loads a persisted resolver trace and removes every step naming a
// record that is not visible in the profile's current projection. When any
// step is removed, pack-level counts are withheld too.
func (rt *Runtime) LoadTrace(profile contracts.Profile, traceID string) ([]resolver.TraceStep, error) {
	if profile != contracts.ProfilePersonal && profile != contracts.ProfileWorkSafe {
		return nil, fmt.Errorf("invalid profile %q", profile)
	}
	if _, err := rt.LoadLedger(); err != nil {
		return nil, err
	}
	id := strings.TrimPrefix(traceID, "trace_")
	if id == "" || strings.ContainsAny(id, `/\.:`) {
		return nil, ErrTraceUnavailable
	}
	data, err := os.ReadFile(filepath.Join(rt.Config.CacheDir, "traces", id+".json"))
	if err != nil {
		return nil, ErrTraceUnavailable
	}
	var steps []resolver.TraceStep
	if err := jsonUnmarshal(data, &steps); err != nil {
		return nil, ErrTraceUnavailable
	}

	visibleIDs := map[string]bool{}
	exists, err := statExists(rt.ProjectionPath(profile))
	if err != nil {
		return nil, err
	}
	if exists {
		store, err := storage.Open(rt.ProjectionPath(profile))
		if err != nil {
			return nil, err
		}
		defer store.Close()
		vis, err := rt.loadVisibility(profile, store)
		if err != nil {
			return nil, err
		}
		recs, err := store.AllRecords()
		if err != nil {
			return nil, err
		}
		for _, rec := range recs {
			if vis.visible(rec) {
				visibleIDs[rec.RecordID] = true
			}
		}
	}
	out := []resolver.TraceStep{}
	withheld := false
	for _, st := range steps {
		if st.RecordID != "" && !visibleIDs[st.RecordID] {
			withheld = true
			continue
		}
		out = append(out, st)
	}
	if withheld {
		for i := range out {
			if out[i].Step == "pack" || out[i].Step == "budget" {
				out[i].Outcome = "withheld: this trace references records no longer visible"
			}
		}
	}
	return out, nil
}

// ProjectionFindings reports administrative health for doctor without
// revealing record identities, counts, or content.
func (rt *Runtime) ProjectionFindings(profile contracts.Profile) ([]string, error) {
	if _, err := rt.LoadLedger(); err != nil {
		return nil, err
	}
	exists, err := statExists(rt.ProjectionPath(profile))
	if err != nil || !exists {
		return nil, err
	}
	store, err := storage.Open(rt.ProjectionPath(profile))
	if err != nil {
		return nil, err
	}
	defer store.Close()
	_, restored, err := rt.EffectiveRevoked(profile, store)
	if err != nil {
		return nil, err
	}
	if restored {
		return []string{fmt.Sprintf("projection %s is out of date with the tombstone ledger: rebuild required (run: beme build --profile %s)", profile, profile)}, nil
	}
	return nil, nil
}
