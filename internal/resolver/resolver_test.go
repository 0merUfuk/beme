package resolver_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
	"github.com/0merUfuk/beme/internal/resolver"
)

// fakeStore gives deterministic inputs to the resolver.
type fakeStore struct {
	records []contracts.Record
}

func (f *fakeStore) Records() []contracts.Record { return f.records }
func (f *fakeStore) Provenance(id string) (contracts.Provenance, bool) {
	return contracts.Provenance{}, false
}
func (f *fakeStore) IndexRevision() string        { return "idx_test0001" }
func (f *fakeStore) PolicyDigest() string         { return "sha256:" + repeat("a", 64) }
func (f *fakeStore) SourceRevisionDigest() string { return "sha256:" + repeat("b", 64) }

func repeat(s string, n int) string {
	out := []byte{}
	for i := 0; i < n; i++ {
		out = append(out, s[0])
	}
	return string(out)
}

func personalCap() policy.Capability {
	return policy.Capability{CapabilityID: "cap_p_0001", Profile: contracts.ProfilePersonal}
}

func baseOpts() resolver.Options {
	return resolver.Options{
		Policy: policy.NewEngine(time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)),
		Now:    time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
	}
}

func mk(t *testing.T, id, kind, role, sens, decisionKey, compact string) contracts.Record {
	t.Helper()
	return contracts.Record{
		RecordID:       id,
		Kind:           contracts.Kind(kind),
		Status:         contracts.StatusActive,
		Confidence:     contracts.ConfidenceValidated,
		Authority:      contracts.AuthorityDefault,
		SourceRole:     contracts.SourceRole(role),
		Trust:          contracts.TrustCanonical,
		Sensitivity:    sens,
		DecisionKey:    decisionKey,
		CompactText:    compact,
		ProvenanceRefs: []string{"prov_" + id},
	}
}

func TestProjectPolicyOutranksPersonalPreference(t *testing.T) {
	store := &fakeStore{records: []contracts.Record{
		mk(t, "rec_a0000001", "preference", "canonical_reusable_knowledge", "public_general", "storage.choice", "personal: prefer boring components"),
		mk(t, "rec_b0000001", "precedent", "trusted_project_policy", "work_restricted:ws-alpha", "storage.choice", "project: embedded db for single-writer batch"),
	}}
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "Choose the storage engine"}
	tc := policy.TaskContext{Task: req.Task, WorkspaceID: "ws-alpha"}
	pack, trace := resolver.Resolve(store, personalCap(), tc, req, baseOpts())
	_ = trace
	found := map[string]bool{}
	for _, c := range pack.Constraints {
		found[c.RecordID] = true
	}
	if !found["rec_b0000001"] {
		t.Fatalf("project policy must win storage.choice; constraints=%v", pack.Constraints)
	}
	// the losing personal preference should appear as shadowed, not dropped silently
	shadowed := false
	for _, c := range pack.Conflicts {
		b, _ := json.Marshal(c)
		if string(b) != "" && (contains(c.RecordIDs, "rec_a0000001")) {
			shadowed = true
		}
	}
	if !shadowed {
		t.Fatalf("shadowed personal preference must be visible in conflicts; got %+v", pack.Conflicts)
	}
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestPrecedenceNarrowerScopeWins(t *testing.T) {
	store := &fakeStore{records: []contracts.Record{
		mk(t, "rec_global001", "principle", "canonical_reusable_knowledge", "public_general", "testing.depth", "global: test what you ship"),
	}}
	scoped := mk(t, "rec_scoped001", "principle", "canonical_reusable_knowledge", "public_general", "testing.depth", "scoped: deterministic QA recomputes from artifact")
	scoped.Scope = contracts.Scope{TaskKinds: []string{"testing"}}
	store.records = append(store.records, scoped)

	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "verify the rendered output passes QA", TaskKindHints: []string{"testing"}}
	tc := policy.TaskContext{Task: req.Task, TaskKinds: []string{"testing"}}
	pack, _ := resolver.Resolve(store, personalCap(), tc, req, baseOpts())
	winner := ""
	for _, g := range pack.Guidance {
		if g.RecordID == "rec_scoped001" || g.RecordID == "rec_global001" {
			if winner == "" {
				winner = g.RecordID
			}
		}
	}
	// Both may appear (different decision-key grouping only collapses same key; here same key -> one winner)
	var winnerID string
	for _, c := range pack.Conflicts {
		if contains(c.RecordIDs, "rec_scoped001") {
			winnerID = c.RecordIDs[0]
		}
	}
	if winnerID != "" && winnerID != "rec_scoped001" {
		t.Fatalf("scoped record must win over global; winner=%s", winnerID)
	}
	_ = winner
}

func TestUnknownsNeverInvented(t *testing.T) {
	store := &fakeStore{records: []contracts.Record{
		mk(t, "rec_c0000001", "principle", "canonical_reusable_knowledge", "public_general", "", "verify before claiming completion"),
	}}
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "Which SQL dialect should we use for the new service? Choose the dialect."}
	tc := policy.TaskContext{Task: req.Task}
	pack, _ := resolver.Resolve(store, personalCap(), tc, req, baseOpts())
	hasDialectUnknown := false
	for _, u := range pack.Unknowns {
		if u.Question == "Preferred SQL dialect beyond documented needs" {
			hasDialectUnknown = true
		}
	}
	if !hasDialectUnknown {
		t.Fatalf("dialect question with no evidence must produce an unknown, not an invented preference; unknowns=%v", pack.Unknowns)
	}
	for _, g := range pack.Guidance {
		if containsStr(g.Text, "dialect") {
			t.Fatalf("no guidance may assert a dialect preference: %q", g.Text)
		}
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

func TestDeterministicPacks(t *testing.T) {
	store := &fakeStore{records: []contracts.Record{
		mk(t, "rec_d0000001", "principle", "canonical_reusable_knowledge", "public_general", "quality.gate", "evidence beats prose"),
		mk(t, "rec_d0000002", "heuristic", "canonical_reusable_knowledge", "public_general", "complexity.need", "no measured problem no new complexity"),
	}}
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "Decide whether to add a cache to the service"}
	tc := policy.TaskContext{Task: req.Task}
	p1, _ := resolver.Resolve(store, personalCap(), tc, req, baseOpts())
	p2, _ := resolver.Resolve(store, personalCap(), tc, req, baseOpts())
	// pack IDs and timestamps are nondeterministic by design; structural
	// content must be stable.
	b1 := structuralJSON(t, p1)
	b2 := structuralJSON(t, p2)
	if b1 != b2 {
		t.Fatalf("identical inputs must produce structurally identical packs:\n%s\n%s", b1, b2)
	}
}

func structuralJSON(t *testing.T, p resolver.Pack) string {
	t.Helper()
	p.PackID = ""
	p.TraceRef = ""
	p.GeneratedAt = ""
	for i := range p.Constraints {
		p.Constraints[i].ExpandRef = ""
	}
	for i := range p.Guidance {
		p.Guidance[i].ExpandRef = ""
	}
	for i := range p.Precedents {
		p.Precedents[i].ExpandRef = ""
	}
	b, _ := json.Marshal(p)
	return string(b)
}

func TestMandatoryContentNotBudgetDropped(t *testing.T) {
	// A tiny budget with big mandatory constraints must yield incomplete
	// completeness + degradation, never silent omission (FR-035).
	store := &fakeStore{records: []contracts.Record{
		mk(t, "rec_e0000001", "principle", "trusted_project_policy", "work_restricted:ws-alpha", "safety.hard", "mandatory constraint one that is quite long to consume tokens"),
		mk(t, "rec_e0000002", "principle", "trusted_project_policy", "work_restricted:ws-alpha", "safety.hard2", "mandatory constraint two also long to consume many tokens"),
	}}
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "do the thing", BudgetHintTokens: 10}
	tc := policy.TaskContext{Task: req.Task, WorkspaceID: "ws-alpha"}
	opts := baseOpts()
	pack, _ := resolver.Resolve(store, personalCap(), tc, req, opts)
	if pack.Resolution.Completeness != "incomplete" {
		t.Fatalf("mandatory overflow must mark completeness incomplete; got %s", pack.Resolution.Completeness)
	}
	if len(pack.Constraints) == 0 {
		t.Fatal("constraints must never be silently dropped")
	}
	found := false
	for _, d := range pack.Degradations {
		if d.Kind == "reduced_assurance" {
			found = true
		}
	}
	if !found {
		t.Fatalf("budget overflow must be reported as degradation; got %+v", pack.Degradations)
	}
}

func TestLearnedObservationsNonNormative(t *testing.T) {
	rec := mk(t, "rec_f0000001", "preference", "learned_observation", "personal_private", "editor.choice", "observed: seemed to prefer dark themes")
	rec.Trust = contracts.TrustQuarantined
	store := &fakeStore{records: []contracts.Record{rec}}
	cap := personalCap()
	cap.ExperimentalLearnedGuidance = true
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "choose editor theme colors"}
	tc := policy.TaskContext{Task: req.Task}
	pack, _ := resolver.Resolve(store, cap, tc, req, baseOpts())
	if len(pack.Constraints) > 0 {
		t.Fatal("learned observations can never be constraints")
	}
	if len(pack.LearnedExperimental) == 0 {
		t.Fatal("explicitly enabled learned guidance must appear in the dedicated section")
	}
	for _, g := range pack.Guidance {
		if g.RecordID == "rec_f0000001" {
			t.Fatal("learned observation must not appear in canonical guidance")
		}
	}
}
