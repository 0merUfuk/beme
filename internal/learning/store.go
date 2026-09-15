// Package learning implements the observation → candidate → batch-review
// pipeline (blueprint §14, FR-050–055, ADR-010).
//
// Invariants encoded here:
//   - Agent feedback creates quarantined observations ONLY (FR-050). Nothing
//     in this package writes canonical knowledge.
//   - Repeated suggestions from one model/session/workflow are ONE evidence
//     family, never independent confirmations (§14.4, FR-052).
//   - Silence is never approval; promotion is user-owned (FR-051, RED).
//   - Rejected candidates carry a fingerprint tombstone so equivalent
//     re-proposals are not re-shown (FR-053, threat case 13).
//   - Work-restricted observations never become global personal knowledge
//     without de-identification + approval (FR-054).
//   - Review actions are transactional and reversible: approve/defers write
//     new state; nothing canonical is mutated (FR-055).
package learning

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func readRand(b []byte) (int, error) { return rand.Read(b) }

// Observation is a quarantined agent-feedback record (§14.3). All
// evidence-family fields are present from creation so repetition can never
// masquerade as independent confirmation.
type Observation struct {
	SchemaVersion string `json:"schema_version"`
	ObservationID string `json:"observation_id"`
	Kind          string `json:"kind"` // observation | correction
	// Hypothesis is the preference/lesson the feedback implies.
	Hypothesis string `json:"hypothesis"`
	// SupportingEvidence / Counterevidence keep §14.3 shape.
	SupportingEvidence string `json:"supporting_evidence,omitempty"`
	Counterevidence    string `json:"counterevidence,omitempty"`
	// EvidenceFamily deduplicates correlated sources: the same
	// model/session/workflow's repeated outputs are one family (§14.4).
	EvidenceFamily string `json:"evidence_family"`
	// FamilyCount counts repetitions WITHIN the family; it never promotes.
	FamilyCount int `json:"family_count"`
	// SourceProfile / InheritedSensitivity pin scope (FR-054).
	SourceProfile        string `json:"source_profile"`
	InheritedSensitivity string `json:"inherited_sensitivity"`
	// CandidateScopes are the scopes the observation might apply to.
	CandidateScopes []string `json:"candidate_scopes,omitempty"`
	Task            string   `json:"task,omitempty"`
	FirstObservedAt string   `json:"first_observed_at"`
	LastObservedAt  string   `json:"last_observed_at"`
	// Status lifecycle: quarantined -> pending_review | rejected | approved
	// | deferred | situational | scope_limited | superseded.
	Status string `json:"status"`
	// ReviewOutcome records the batch-review action + tombstone when rejected.
	ReviewOutcome *ReviewOutcome `json:"review_outcome,omitempty"`
}

// ReviewOutcome is the recorded batch-review decision (§14.5).
type ReviewOutcome struct {
	Action     string `json:"action"` // approve|edit|merge|reject|defer|situational|scope_limit|counterexample
	ReviewedAt string `json:"reviewed_at"`
	Reviewer   string `json:"reviewer"` // operator identity marker
	Note       string `json:"note,omitempty"`
	// Fingerprint tombstones equivalent re-proposals (FR-053).
	Fingerprint string `json:"fingerprint,omitempty"`
	// MergedIntoRecord names the canonical record a merge targeted (never
	// written here — canonical writes are a separate trusted path).
	MergedIntoRecord string `json:"merged_into_record,omitempty"`
}

// Store is the durable observation/candidate store (data dir, not cache).
// It is derived operational state: safe to review, never canonical.
type Store struct {
	dir   string
	index map[string]bool // tombstone fingerprints (reject/scope-limited)
}

// Open prepares the durable observations directory.
func Open(dataDir string) (*Store, error) {
	dir := filepath.Join(dataDir, "observations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, index: map[string]bool{}}
	// load existing tombstones; an unreadable store is an error, never an
	// empty one (tombstones and purge inspection depend on it)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read observations: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var obs Observation
		if data, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			if json.Unmarshal(data, &obs) == nil && obs.ReviewOutcome != nil && obs.ReviewOutcome.Fingerprint != "" {
				if isTerminalReject(obs.Status) {
					s.index[obs.ReviewOutcome.Fingerprint] = true
				}
			}
		}
	}
	return s, nil
}

func isTerminalReject(status string) bool {
	switch status {
	case "rejected", "superseded":
		return true
	}
	return false
}

// Dir returns the store directory.
func (s *Store) Dir() string { return s.dir }

// Observe records feedback. If an equivalent hypothesis from the SAME
// evidence family already exists, the repetition increments that family's
// count instead of creating a new observation (FR-052: correlated
// repetitions never appear independent).
func (s *Store) Observe(kind, hypothesis, family, sourceProfile, inheritedSensitivity, task string) (*Observation, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	fingerprint := hypothesisFingerprint(kind, hypothesis)

	// Tombstone check first: equivalent rejected proposals are not re-filed.
	if s.index[fingerprint] {
		return nil, fmt.Errorf("equivalent proposal was previously rejected (tombstoned)")
	}

	// Family dedup: same kind + hypothesis + family => increment, not a new record.
	if existing, err := s.findByFingerprint(fingerprint, family); err == nil && existing != nil {
		existing.FamilyCount++
		existing.LastObservedAt = now
		if err := s.write(existing.ObservationID, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	obs := Observation{
		SchemaVersion:        "1",
		ObservationID:        newID("obs"),
		Kind:                 kind,
		Hypothesis:           hypothesis,
		EvidenceFamily:       family,
		FamilyCount:          1,
		SourceProfile:        sourceProfile,
		InheritedSensitivity: inheritedSensitivity,
		Task:                 task,
		FirstObservedAt:      now,
		LastObservedAt:       now,
		Status:               "quarantined",
	}
	if err := s.write(obs.ObservationID, &obs); err != nil {
		return nil, err
	}
	return &obs, nil
}

// findByFingerprint locates a live (non-terminal, non-approved) observation
// equivalent to the given hypothesis fingerprint AND evidence family.
// Family is part of the match key: repetitions within one family collapse;
// distinct families stay distinct observations (§14.4).
func (s *Store) findByFingerprint(fingerprint, family string) (*Observation, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	for _, e := range sortedEntries(entries) {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var obs Observation
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil || json.Unmarshal(data, &obs) != nil {
			continue
		}
		if hypothesisFingerprint(obs.Kind, obs.Hypothesis) == fingerprint &&
			obs.EvidenceFamily == family &&
			!isTerminalReject(obs.Status) && obs.Status != "approved" {
			var copy = obs
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *Store) write(id string, obs *Observation) error {
	data, err := json.MarshalIndent(obs, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: temp + rename (process-safe on the same host).
	tmp := filepath.Join(s.dir, id+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, id+".json"))
}

// List returns observations by status ("quarantined" = pending batch review).
func (s *Store) List(status string) []Observation {
	out := []Observation{}
	entries, _ := os.ReadDir(s.dir)
	for _, e := range sortedEntries(entries) {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var obs Observation
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil || json.Unmarshal(data, &obs) != nil {
			continue
		}
		if status == "" || obs.Status == status {
			out = append(out, obs)
		}
	}
	return out
}

// Get fetches one observation by ID.
// ErrObservationNotFound: no observation with that ID exists.
var ErrObservationNotFound = errors.New("observation not found")

func validObservationID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\`) && id != "." && id != ".."
}

func (s *Store) Get(id string) (*Observation, error) {
	if !validObservationID(id) {
		return nil, fmt.Errorf("%w: %s", ErrObservationNotFound, id)
	}
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrObservationNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("observation %s unreadable: %w", id, err)
	}
	var obs Observation
	if err := json.Unmarshal(data, &obs); err != nil {
		return nil, fmt.Errorf("observation %s corrupt: %w", id, err)
	}
	return &obs, nil
}

// ListAll returns every observation, failing on an unreadable directory or
// any unreadable or corrupt observation file. Purge planning uses it so a
// store that cannot be fully inspected never looks like a store with no
// matches.
func (s *Store) ListAll() ([]Observation, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read observations: %w", err)
	}
	out := []Observation{}
	for _, e := range sortedEntries(entries) {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("observation %s unreadable: %w", e.Name(), err)
		}
		var obs Observation
		if err := json.Unmarshal(data, &obs); err != nil {
			return nil, fmt.Errorf("observation %s corrupt: %w", e.Name(), err)
		}
		out = append(out, obs)
	}
	return out, nil
}

// Exists reports whether an observation file exists (read errors other than
// not-exist are returned).
func (s *Store) Exists(id string) (bool, error) {
	if !validObservationID(id) {
		return false, fmt.Errorf("invalid observation id %q", id)
	}
	_, err := os.Stat(filepath.Join(s.dir, id+".json"))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	}
	return false, err
}

// Review applies one §14.5 batch-review action. Canonical promotion is NOT
// implemented here by design: `approve` only marks the observation
// user-approved-for-promotion and emits a canonical REVISION PROPOSAL
// file for the trusted proposal path — it never writes knowledge entries.
func (s *Store) Review(id, action, reviewer, note string) (*Observation, error) {
	obs, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if obs.Status == "approved" || isTerminalReject(obs.Status) {
		return nil, fmt.Errorf("observation %s already terminal (%s); file a new observation instead", id, obs.Status)
	}
	outcome := ReviewOutcome{
		Action:     action,
		ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Reviewer:   reviewer,
		Note:       note,
	}
	switch action {
	case "approve":
		// User-owned promotion remains RED: an approve here records the
		// decision and creates a proposal; canonical write is separate.
		obs.Status = "approved"
		outcome.Fingerprint = hypothesisFingerprint(obs.Kind, obs.Hypothesis)
	case "reject":
		obs.Status = "rejected"
		outcome.Fingerprint = hypothesisFingerprint(obs.Kind, obs.Hypothesis)
		s.index[outcome.Fingerprint] = true
	case "defer":
		obs.Status = "deferred"
	case "situational":
		obs.Status = "situational"
	case "scope_limit", "scope-limit":
		obs.Status = "scope_limited"
		action = "scope_limit"
	case "edit":
		if note == "" {
			return nil, fmt.Errorf("edit requires the revised hypothesis in the note")
		}
		obs.Hypothesis = note
		obs.Status = "pending_review"
	case "merge":
		// merge target carried in note as record id
		if note == "" {
			return nil, fmt.Errorf("merge requires the target canonical record ID in the note")
		}
		obs.Status = "merged"
		outcome.MergedIntoRecord = note
		outcome.Fingerprint = hypothesisFingerprint(obs.Kind, obs.Hypothesis)
	case "counterexample":
		if note == "" {
			return nil, fmt.Errorf("counterexample requires the counterevidence in the note")
		}
		obs.Counterevidence += "\n" + note
		obs.Status = "pending_review"
	default:
		return nil, fmt.Errorf("unknown review action %q (§14.5: approve|edit|merge|reject|defer|situational|scope_limit|counterexample)", action)
	}
	obs.ReviewOutcome = &outcome
	if err := s.write(obs.ObservationID, obs); err != nil {
		return nil, err
	}
	return obs, nil
}

// Tombstoned reports whether an equivalent hypothesis was rejected.
func (s *Store) Tombstoned(hypothesis string) bool {
	return s.index[hypothesisFingerprint("observation", hypothesis)]
}

// FamilyCounts returns family → repetition count (for review displays).
func (s *Store) FamilyCounts() map[string]int {
	out := map[string]int{}
	for _, obs := range s.List("") {
		out[obs.EvidenceFamily] += obs.FamilyCount
	}
	return out
}

func hypothesisFingerprint(kind, hypothesis string) string {
	normalized := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(hypothesis))), " ")
	h := sha256.Sum256([]byte(kind + "\x1f" + normalized))
	return "fp_" + hex.EncodeToString(h[:12])
}

func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := readRand(b); err != nil {
		return fmt.Sprintf("%s_fallback0", prefix)
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func sortedEntries(entries []os.DirEntry) []os.DirEntry {
	out := append([]os.DirEntry{}, entries...)
	// stable enough: sort by name for determinism (NFR-002)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Name() < out[j-1].Name(); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Remove deletes one observation file and reports whether it existed. It
// exists only for the RED physical purge workflow (§7.8 derived purge of
// pending observations); ordinary review never deletes observations.
func (s *Store) Remove(id string) (bool, error) {
	if !validObservationID(id) {
		return false, fmt.Errorf("invalid observation id %q", id)
	}
	err := os.Remove(filepath.Join(s.dir, id+".json"))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	}
	return false, err
}
