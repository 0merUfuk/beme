package resolver_test

import (
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
	"github.com/0merUfuk/beme/internal/resolver"
)

// Retrieval (ARCHITECTURE §2, FR-034/035): constraints and globally
// applicable principles always reach the pack; advisory records need task
// evidence; with none, the pack carries an honest unknown.

func packItemIDs(p resolver.Pack) map[string]bool {
	ids := map[string]bool{}
	for _, sec := range [][]resolver.ContextItem{p.Constraints, p.Guidance, p.Precedents, p.LearnedExperimental} {
		for _, it := range sec {
			ids[it.RecordID] = true
		}
	}
	return ids
}

func hasNoEvidenceUnknown(p resolver.Pack) bool {
	for _, u := range p.Unknowns {
		if u.Question == resolver.NoEvidenceQuestion {
			return u.SuggestedEscalation != nil && *u.SuggestedEscalation == "proceed_with_assumption"
		}
	}
	return false
}

func mixedStore(t *testing.T) *fakeStore {
	constraint := mk(t, "rec_policy", "directive", "trusted_project_policy", "public_general", "", "never commit credentials to the repository")
	critical := mk(t, "rec_critical", "directive", "canonical_reusable_knowledge", "public_general", "", "confirm destructive operations explicitly")
	critical.Criticality = "high"
	principle := mk(t, "rec_principle", "principle", "canonical_reusable_knowledge", "public_general", "", "evidence beats prose when claiming completion")
	preference := mk(t, "rec_pref", "preference", "canonical_reusable_knowledge", "public_general", "", "prefer small reviewable changes over large rewrites")
	precedent := mk(t, "rec_prec", "precedent", "episodic_evidence", "public_general", "", "the standard library testing package was enough for the last command-line tool")
	return &fakeStore{records: []contracts.Record{constraint, critical, principle, preference, precedent}}
}

func resolveTask(t *testing.T, store *fakeStore, task string) (resolver.Pack, []resolver.TraceStep) {
	t.Helper()
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: task}
	return resolver.Resolve(store, personalCap(), policy.TaskContext{Task: task}, req, baseOpts())
}

func TestRetrievalDropsIrrelevantAdvisoryKeepsMandatoryAndPrinciples(t *testing.T) {
	store := mixedStore(t)
	pack, trace := resolveTask(t, store, "Which database should the billing service use?")
	ids := packItemIDs(pack)
	for _, want := range []string{"rec_policy", "rec_critical", "rec_principle"} {
		if !ids[want] {
			t.Errorf("%s must always reach the pack (constraint or global principle); got %v", want, ids)
		}
	}
	for _, drop := range []string{"rec_pref", "rec_prec"} {
		if ids[drop] {
			t.Errorf("advisory record %s has no evidence for this task and must be excluded; got %v", drop, ids)
		}
	}
	excluded := map[string]bool{}
	for _, st := range trace {
		if st.Step == "relevance" && st.Outcome == "not relevant to task" {
			excluded[st.RecordID] = true
		}
	}
	if !excluded["rec_pref"] || !excluded["rec_prec"] || excluded["rec_principle"] {
		t.Errorf("every relevance exclusion must be traced, and only those: %v", excluded)
	}
	if !hasNoEvidenceUnknown(pack) {
		t.Errorf("with no advisory evidence the pack must carry the no-evidence unknown; unknowns=%+v", pack.Unknowns)
	}
}

func TestRetrievalKeepsRelevantAdvisoryWithoutNoEvidenceUnknown(t *testing.T) {
	store := mixedStore(t)
	pack, _ := resolveTask(t, store, "How big should this reviewable change be before I split it?")
	ids := packItemIDs(pack)
	if !ids["rec_pref"] {
		t.Fatalf("the preference about reviewable changes is evidence for this task; got %v", ids)
	}
	if ids["rec_prec"] {
		t.Fatalf("the testing precedent is not evidence for this task; got %v", ids)
	}
	if hasNoEvidenceUnknown(pack) {
		t.Fatalf("a pack with task evidence must not claim there is none; unknowns=%+v", pack.Unknowns)
	}
	// inflections match: "tests" in the task, "testing" in the record
	pack, _ = resolveTask(t, store, "Which tests framework should I adopt?")
	if !packItemIDs(pack)["rec_prec"] {
		t.Fatalf("inflected content words must match (tests/testing); got %v", packItemIDs(pack))
	}
}

// TestGenericWordsDoNotCreateRelevance: function words and generic request
// vocabulary appear in almost every task and record, so they cannot make an
// advisory record relevant — otherwise every pack would still carry
// everything.
func TestGenericWordsDoNotCreateRelevance(t *testing.T) {
	generic := mk(t, "rec_generic", "preference", "canonical_reusable_knowledge", "public_general", "", "what you should do with this is choose the best approach when you need to decide")
	store := &fakeStore{records: []contracts.Record{generic}}
	pack, _ := resolveTask(t, store, "What should I do with this? Help me choose the best approach and decide.")
	if packItemIDs(pack)["rec_generic"] {
		t.Fatal("shared generic words must not make an advisory record relevant")
	}
	if !hasNoEvidenceUnknown(pack) {
		t.Fatal("the pack must say no evidence supports the task")
	}
}

// TestScopedAndDecisionAlternativesAreRetained: a task-scoped record that
// passed Stage A is in scope, and a record sharing a decision_key with a
// relevant record is an alternative for the same decision, so precedence and
// conflict reporting still see it.
func TestScopedAndDecisionAlternativesAreRetained(t *testing.T) {
	scoped := mk(t, "rec_scoped", "heuristic", "canonical_reusable_knowledge", "public_general", "", "keep deploy scripts idempotent")
	scoped.Scope.TaskKinds = []string{"deployment"}
	winner := mk(t, "rec_win", "preference", "canonical_reusable_knowledge", "public_general", "dk.0001", "an embedded file store for local batch tools")
	loser := mk(t, "rec_lose", "preference", "canonical_reusable_knowledge", "public_general", "dk.0001", "a managed server")
	loser.Confidence = contracts.ConfidenceObserved
	store := &fakeStore{records: []contracts.Record{scoped, winner, loser}}
	// control: the loser shares no content word with the task, so only the
	// decision_key rule can retain it
	if tokens := "a managed server"; strings.Contains(tokens, "storage") || strings.Contains(tokens, "batch") {
		t.Fatal("fixture: the loser must not be independently relevant")
	}

	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "Pick the storage for a local batch tool"}
	tc := policy.TaskContext{Task: req.Task, TaskKinds: []string{"deployment"}}
	pack, trace := resolver.Resolve(store, personalCap(), tc, req, baseOpts())
	ids := packItemIDs(pack)
	if !ids["rec_scoped"] {
		t.Errorf("a task-scoped record that passed Stage A must be retained; got %v", ids)
	}
	if !ids["rec_win"] {
		t.Errorf("the relevant decision must be retained; got %v", ids)
	}
	for _, st := range trace {
		if st.Step == "relevance" && st.RecordID == "rec_lose" {
			t.Errorf("an alternative sharing the decision_key must reach precedence, not be dropped as irrelevant")
		}
	}
	shadowed := false
	for _, c := range pack.Conflicts {
		shadowed = shadowed || strings.Contains(c.Description, "rec_lose")
	}
	if !shadowed {
		t.Errorf("the alternative must be reported as shadowed by the winner: %+v", pack.Conflicts)
	}
}
