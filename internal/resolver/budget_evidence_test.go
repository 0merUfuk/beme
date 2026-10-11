package resolver_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
	"github.com/0merUfuk/beme/internal/resolver"
)

func budgetResolve(t *testing.T, records []contracts.Record, task string, budget int) resolver.Pack {
	t.Helper()
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: task, BudgetHintTokens: budget}
	p, _ := resolver.Resolve(&fakeStore{records: records}, personalCap(), policy.TaskContext{Task: task}, req, baseOpts())
	return p
}

func TestBudgetCanAdmitEvidenceThatReplacesAnUnknown(t *testing.T) {
	rec := mk(t, "rec_short", "heuristic", "canonical_reusable_knowledge", "public_general", "", "Use an embedded database for local storage.")
	full := budgetResolve(t, []contracts.Record{rec}, "local database storage", 3000)
	if !packItemIDs(full)[rec.RecordID] {
		t.Fatal("positive control")
	}
	p := budgetResolve(t, []contracts.Record{rec}, "local database storage", full.Budget.EstimatedTokens)
	if !packItemIDs(p)[rec.RecordID] {
		t.Fatalf("an empty-pack unknown reserved more than the entire fitting answer: full=%+v empty=%+v", full.Budget, p.Budget)
	}
}

func TestBudgetAccountingDescribesDeliveredEvidence(t *testing.T) {
	rec := mk(t, "rec_dialect", "heuristic", "canonical_reusable_knowledge", "public_general", "", "SQL dialect choice "+strings.Repeat("x", 4000))
	rec.Statement = rec.CompactText
	p := budgetResolve(t, []contracts.Record{rec}, "preferred SQL dialect", 10)
	if len(packItemIDs(p)) != 0 {
		t.Fatal("budget omission control")
	}
	if len(p.Unknowns) == 0 || len(p.Provenance) != 0 {
		t.Errorf("undelivered evidence still claimed: unknowns=%+v provenance=%+v", p.Unknowns, p.Provenance)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if p.Budget.EstimatedTokens != (len(b)+3)/4 {
		t.Errorf("output overhead unaccounted: estimated=%d serialized=%d", p.Budget.EstimatedTokens, (len(b)+3)/4)
	}
	if len(p.Unknowns) > 0 && !strings.Contains(strings.ToLower(p.Unknowns[0].WhyMaterial), "budget") {
		t.Error("unknown hides loss to budget")
	}
	// Evidence only in the full statement is not evidence delivered in compact text.
	rec.CompactText = "Choose an embedded database."
	rec.Statement = "Use this SQL dialect for the storage layer."
	p = budgetResolve(t, []contracts.Record{rec}, "preferred SQL dialect storage database", 3000)
	dialectUnknown := false
	for _, u := range p.Unknowns {
		if strings.Contains(u.Question, "dialect") {
			dialectUnknown = true
		}
	}
	if !dialectUnknown {
		t.Error("undelivered statement suppressed material unknown")
	}
}

func TestBudgetRanksAcrossAdvisorySections(t *testing.T) {
	low := mk(t, "rec_a_low", "heuristic", "canonical_reusable_knowledge", "public_general", "", "storage "+strings.Repeat("lettuce ", 500))
	high := mk(t, "rec_z_high", "precedent", "canonical_reusable_knowledge", "public_general", "", "storage engine local batch single-writer "+strings.Repeat("x", 4000))
	p := budgetResolve(t, []contracts.Record{low, high}, "storage engine local batch single-writer", 1600)
	if !packItemIDs(p)[high.RecordID] || packItemIDs(p)[low.RecordID] {
		t.Fatalf("section order defeated relevance: %v", packItemIDs(p))
	}
}

func TestMandatoryOverflowRemovesAllAdvisory(t *testing.T) {
	mandatory := mk(t, "rec_mandatory", "directive", "trusted_project_policy", "public_general", "", strings.Repeat("mandatory safety ", 40))
	advisory := mk(t, "rec_advisory", "heuristic", "canonical_reusable_knowledge", "public_general", "", strings.Repeat("storage ", 100))
	p := budgetResolve(t, []contracts.Record{mandatory, advisory}, "storage", 10)
	if !packItemIDs(p)[mandatory.RecordID] {
		t.Fatal("mandatory constraint lost")
	}
	if packItemIDs(p)[advisory.RecordID] || p.Budget.OmittedAdvisoryCount != 1 {
		t.Fatalf("overflow bypassed budget: %v %+v", packItemIDs(p), p.Budget)
	}
}

func TestBudgetAdmissionKeepsRelevanceBeforePresentationOrder(t *testing.T) {
	low := mk(t, "rec_a_low", "heuristic", "canonical_reusable_knowledge", "public_general", "", "storage "+strings.Repeat("lettuce ", 500))
	high := mk(t, "rec_z_high", "heuristic", "canonical_reusable_knowledge", "public_general", "", "storage engine local batch single-writer "+strings.Repeat("x", 4000))
	for _, records := range [][]contracts.Record{{low, high}, {high, low}} {
		p := budgetResolve(t, records, "storage engine local batch single-writer", 1600)
		if !packItemIDs(p)[high.RecordID] || packItemIDs(p)[low.RecordID] {
			t.Fatalf("ID order defeated relevance: %v %+v", packItemIDs(p), p.Budget)
		}
	}
}
