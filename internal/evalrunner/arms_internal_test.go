package evalrunner

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
)

// richInputs is a hand-built EvalInputs that exercises every field an arm
// transform touches, including knowledge refs with locators (which the live
// resolver does not emit yet).
func richInputs() resolver.EvalInputs {
	item := func(id, kind, prov string) resolver.ContextItem {
		return resolver.ContextItem{
			RecordID: id, Kind: kind, Text: "text of " + id,
			SelectionReason: []string{"retrieval score: 1.0", "decision_key matched: k", "provenance: cited " + prov},
			ProvenanceRefs:  []string{prov}, ExpandRef: "expand_" + id,
		}
	}
	loc, srd, sid := "entries/k.md", "sha256:source-revision", "src-a"
	pack := resolver.Pack{
		Request:             resolver.PackRequest{TaskKinds: []string{"architecture"}},
		Resolution:          resolver.PackResolution{Profile: "personal", PolicyDigest: "sha256:policy", IndexRevision: "idx_1", SourceRevisionDigest: &srd},
		Constraints:         []resolver.ContextItem{item("rec_policy", "directive", "prov_policy")},
		Guidance:            []resolver.ContextItem{item("rec_found", "principle", "prov_found"), item("rec_know", "heuristic", "prov_know"), item("rec_decl", "principle", "prov_decl"), item("rec_ref", "fact", "prov_ref")},
		Precedents:          []resolver.ContextItem{item("rec_prec", "precedent", "prov_prec"), item("rec_canon_prec", "precedent", "prov_canon_prec")},
		LearnedExperimental: []resolver.ContextItem{item("rec_learned", "pattern", "prov_learned")},
		Knowledge:           []resolver.KnowledgeRef{{Title: "k", Description: "d", Locator: &loc, ProvenanceRefs: []string{"prov_k"}}},
		Unknowns:            []resolver.Unknown{{Question: "q", WhyMaterial: "w"}},
		Conflicts: []resolver.PackConflict{
			{State: "shadowed", Description: "know vs learned", RecordIDs: []string{"rec_know", "rec_learned"}},
			{State: "shadowed", Description: "found vs know", RecordIDs: []string{"rec_found", "rec_know"}},
		},
		Provenance: []resolver.PackProvenance{
			{SourceID: &sid, SourceRole: string(contracts.RoleCanonicalFoundation), RecordCount: 1},
			{SourceRole: string(contracts.RoleLearnedObservation), RecordCount: 1},
			{SourceRole: string(contracts.RoleEpisodicEvidence), RecordCount: 1},
		},
		Budget: resolver.PackBudget{RequestedTokens: 20, Truncated: true, OmittedAdvisoryCount: 1},
	}
	noScope := resolver.Pack{Guidance: []resolver.ContextItem{item("rec_scoped", "heuristic", "prov_scoped")}, Budget: resolver.PackBudget{RequestedTokens: 20}}
	return resolver.EvalInputs{
		Profile: contracts.ProfilePersonal, CapabilityID: "cap", LearnedEnabled: true,
		Pack: pack, NoScopePack: &noScope, NoScopeEligible: 4, BuildGeneration: "gen",
		Eligible: resolver.EligibleSet{Candidates: []resolver.Candidate{
			{Item: item("rec_know", "heuristic", "prov_know"), SourceRole: contracts.RoleCanonicalKnowledge, Score: 3, Tokens: 10},
			{Item: item("rec_learned", "pattern", "prov_learned"), SourceRole: contracts.RoleLearnedObservation, Score: 2, Tokens: 50},
			{Item: item("rec_found", "principle", "prov_found"), SourceRole: contracts.RoleCanonicalFoundation, Score: 1, Tokens: 5},
		}},
		Roles: map[string]contracts.SourceRole{
			"rec_policy": contracts.RoleTrustedProjectPolicy, "rec_found": contracts.RoleCanonicalFoundation,
			"rec_know": contracts.RoleCanonicalKnowledge, "rec_decl": contracts.RoleDeclassifiedSafe,
			"rec_ref": contracts.RoleTrustedReference, "rec_prec": contracts.RoleEpisodicEvidence,
			"rec_canon_prec": contracts.RoleCanonicalKnowledge, "rec_learned": contracts.RoleLearnedObservation,
		},
	}
}

func mutateAll(c *ArmContext) {
	touch := func(items []resolver.ContextItem) {
		for i := range items {
			items[i].Text = "MUTATED"
			for j := range items[i].ProvenanceRefs {
				items[i].ProvenanceRefs[j] = "MUTATED"
			}
			for j := range items[i].SelectionReason {
				items[i].SelectionReason[j] = "MUTATED"
			}
		}
	}
	touch(c.Records)
	if p := c.Pack; p != nil {
		touch(p.Constraints)
		touch(p.Guidance)
		touch(p.Precedents)
		touch(p.LearnedExperimental)
		for i := range p.Knowledge {
			if p.Knowledge[i].Locator != nil {
				*p.Knowledge[i].Locator = "MUTATED"
			}
			for j := range p.Knowledge[i].ProvenanceRefs {
				p.Knowledge[i].ProvenanceRefs[j] = "MUTATED"
			}
		}
		for i := range p.Unknowns {
			p.Unknowns[i].Question = "MUTATED"
		}
		for i := range p.Conflicts {
			for j := range p.Conflicts[i].RecordIDs {
				p.Conflicts[i].RecordIDs[j] = "MUTATED"
			}
		}
		for i := range p.Provenance {
			if p.Provenance[i].SourceID != nil {
				*p.Provenance[i].SourceID = "MUTATED"
			}
		}
		if p.Resolution.SourceRevisionDigest != nil {
			*p.Resolution.SourceRevisionDigest = "MUTATED"
		}
		for i := range p.Request.TaskKinds {
			p.Request.TaskKinds[i] = "MUTATED"
		}
	}
}

// TestBuildArmNeverAliasesInputs: mutating every arm's output through slice
// elements and pointers leaves the shared inputs byte-identical.
func TestBuildArmNeverAliasesInputs(t *testing.T) {
	in := richInputs()
	before, _ := json.Marshal(in)
	for _, arm := range AllArms {
		b, err := buildArm(in, arm, "disabled")
		if err != nil || b.NotRun != "" {
			t.Fatalf("%s: err=%v not_run=%q", arm, err, b.NotRun)
		}
		mutateAll(&b.Context)
		if arm != ArmB0 && arm != ArmB1 {
			if out, _ := json.Marshal(b.Context); !strings.Contains(string(out), "MUTATED") {
				t.Fatalf("control: %s output was not mutated", arm)
			}
		}
	}
	if after, _ := json.Marshal(in); !bytes.Equal(before, after) {
		t.Fatalf("an arm mutated the shared inputs:\nbefore %s\nafter  %s", before, after)
	}
}

// TestNoProvenanceRemovesEveryProvenanceMechanism covers the fields the live
// fixture cannot: knowledge refs and locators and provenance-citing reasons.
func TestNoProvenanceRemovesEveryProvenanceMechanism(t *testing.T) {
	in := richInputs()
	full, err := buildArm(in, ArmB4, "disabled")
	if err != nil {
		t.Fatal(err)
	}
	fullView, _ := RenderContext(full.Context)
	for _, s := range []string{"prov_", "entries/k.md", "sha256:source-revision", "provenance"} {
		if !strings.Contains(fullView, s) {
			t.Fatalf("control: B4 view must contain %q", s)
		}
	}
	b, err := buildArm(in, ArmAblNoProv, "disabled")
	if err != nil {
		t.Fatal(err)
	}
	view, _ := RenderContext(b.Context)
	for _, s := range []string{"prov_", "entries/k.md", "sha256:source-revision", "provenance", "expand_", "src-a"} {
		if strings.Contains(view, s) {
			t.Fatalf("no-provenance view still contains %q:\n%s", s, view)
		}
	}
	p := b.Context.Pack
	if len(p.Provenance) != 0 || p.Resolution.SourceRevisionDigest != nil || p.Knowledge[0].Locator != nil || p.Knowledge[0].ProvenanceRefs != nil || p.LearnedExperimental[0].ProvenanceRefs != nil {
		t.Fatalf("no-provenance kept provenance: %+v", p)
	}
	// 8 item refs + 8 provenance-citing reasons + knowledge ref + locator + 3 manifest entries + source revision digest
	if b.Construction.Removed != 22 {
		t.Fatalf("removed %d provenance references, want 22", b.Construction.Removed)
	}
	if got := len(b.Context.Items()); got != len(full.Context.Items()) {
		t.Fatalf("no-provenance must keep every item: %d vs %d", got, len(full.Context.Items()))
	}
}

// TestCanonicalAndLearnedOnlyRoleDefinitions pins the role rules.
func TestCanonicalAndLearnedOnlyRoleDefinitions(t *testing.T) {
	in := richInputs()
	c, err := buildArm(in, ArmAblCanonOnly, "disabled")
	if err != nil || c.NotRun != "" {
		t.Fatalf("canonical-only: %v %q", err, c.NotRun)
	}
	p := c.Context.Pack
	got := func(items []resolver.ContextItem) string {
		out := []string{}
		for _, it := range items {
			out = append(out, it.RecordID)
		}
		return strings.Join(out, ",")
	}
	if got(p.Constraints) != "rec_policy" || got(p.Guidance) != "rec_found,rec_know,rec_decl" || len(p.Precedents) != 0 || p.LearnedExperimental != nil {
		t.Fatalf("canonical-only sections: c=%s g=%s p=%s l=%s", got(p.Constraints), got(p.Guidance), got(p.Precedents), got(p.LearnedExperimental))
	}
	if len(p.Conflicts) != 1 || p.Conflicts[0].Description != "found vs know" || len(p.Knowledge) != 0 || len(p.Provenance) != 1 || p.Provenance[0].SourceRole != string(contracts.RoleCanonicalFoundation) {
		t.Fatalf("canonical-only conflicts/knowledge/provenance: %+v %+v %+v", p.Conflicts, p.Knowledge, p.Provenance)
	}

	l, err := buildArm(in, ArmAblLearnedOnly, "disabled")
	if err != nil || l.NotRun != "" {
		t.Fatalf("learned-only: %v %q", err, l.NotRun)
	}
	lp := l.Context.Pack
	if got(l.Context.Items()) != "rec_learned" || len(lp.Unknowns) != 0 || len(lp.Conflicts) != 0 || len(lp.Provenance) != 1 || lp.Provenance[0].SourceRole != string(contracts.RoleLearnedObservation) {
		t.Fatalf("learned-only: items=%s %+v", got(l.Context.Items()), lp)
	}

	in.Roles = map[string]contracts.SourceRole{}
	if b, _ := buildArm(in, ArmAblCanonOnly, "disabled"); b.NotRun == "" {
		t.Fatal("canonical-only with unknown roles must be not_run, never guessed")
	}
}

// TestUnsupportedConditionsAreNotRun: arms that cannot honestly run say why.
func TestUnsupportedConditionsAreNotRun(t *testing.T) {
	cases := map[string]func() (armBuild, error){
		"learned-only without learned capability": func() (armBuild, error) {
			in := richInputs()
			in.LearnedEnabled = false
			return buildArm(in, ArmAblLearnedOnly, "disabled")
		},
		"no-scope refused deployment": func() (armBuild, error) {
			in := richInputs()
			in.NoScopePack, in.NoScopeRefusal = nil, "no-scope requires a synthetic deployment: test"
			return buildArm(in, ArmAblNoScope, "disabled")
		},
		"no-scope with network": func() (armBuild, error) { return buildArm(richInputs(), ArmAblNoScope, "integration_only") },
		"unknown arm":           func() (armBuild, error) { return buildArm(richInputs(), Arm("B9"), "disabled") },
		"B3 without a budget": func() (armBuild, error) {
			in := richInputs()
			in.Pack.Budget.RequestedTokens = 0
			return buildArm(in, ArmB3, "disabled")
		},
	}
	for name, fn := range cases {
		b, err := fn()
		if err != nil || b.NotRun == "" {
			t.Errorf("%s: must be not_run with a reason (err=%v)", name, err)
		}
	}
}

// TestRawDumpAndRankedRetrievalConstruction pins B2 ordering and B3's
// score-ordered budget rule.
func TestRawDumpAndRankedRetrievalConstruction(t *testing.T) {
	in := richInputs()
	b2, _ := buildArm(in, ArmB2, "disabled")
	if strings.Join(ids(b2.Context.Records), ",") != "rec_found,rec_know,rec_learned" || b2.Construction.Truncated != 0 || b2.Construction.Selected != 3 {
		t.Fatalf("B2: %v %+v", ids(b2.Context.Records), b2.Construction)
	}
	for _, it := range b2.Context.Records {
		if it.SelectionReason != nil || it.ExpandRef != "" {
			t.Fatalf("B2 item %s must carry no selection reasons or expansion refs", it.RecordID)
		}
	}

	// budget 20: 10 fits, 50 does not (skipped), 5 fits
	b3, _ := buildArm(in, ArmB3, "disabled")
	if strings.Join(ids(b3.Context.Records), ",") != "rec_know,rec_found" || b3.Construction.Truncated != 1 || b3.Construction.BudgetTokens != 20 {
		t.Fatalf("B3: %v %+v", ids(b3.Context.Records), b3.Construction)
	}
	for _, it := range b3.Context.Records {
		if it.ProvenanceRefs != nil || len(it.SelectionReason) != 1 || it.SelectionReason[0] != "retrieval score: 1.0" {
			t.Fatalf("B3 item %s: refs=%v reasons=%v", it.RecordID, it.ProvenanceRefs, it.SelectionReason)
		}
	}
}

func ids(items []resolver.ContextItem) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.RecordID)
	}
	return out
}
