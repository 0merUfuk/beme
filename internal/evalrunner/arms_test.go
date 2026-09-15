package evalrunner_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/bootstrap"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"github.com/0merUfuk/beme/internal/policy"
	"github.com/0merUfuk/beme/internal/resolver"
	"github.com/0merUfuk/beme/internal/storage"
)

// buildFixture builds and seeds the synthetic deployment. The store is
// opened here, in test code, per the ADR-027 read-surface guard.
func buildFixture(t *testing.T, opts fx.Options) *fx.Deployment {
	t.Helper()
	d, err := fx.Build(t.TempDir(), opts)
	if err != nil {
		t.Fatal(err)
	}
	st, err := storage.Open(d.Runtime.ProjectionPath(fx.Profile))
	if err != nil {
		t.Fatal(err)
	}
	err = d.Seed(st)
	if cerr := st.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// recordingProvider keeps every request it receives, per arm.
type recordingProvider struct {
	evalrunner.MockProvider
	reqs map[evalrunner.Arm][]evalrunner.GenerationRequest
}

func newRecorder() *recordingProvider {
	return &recordingProvider{MockProvider: evalrunner.NewMockProvider(""), reqs: map[evalrunner.Arm][]evalrunner.GenerationRequest{}}
}

func (r *recordingProvider) Generate(req evalrunner.GenerationRequest) (evalrunner.GenerationResult, error) {
	r.reqs[req.Arm] = append(r.reqs[req.Arm], req)
	return r.MockProvider.Generate(req)
}

type armRun struct {
	d   *fx.Deployment
	in  resolver.EvalInputs
	sum *evalrunner.Summary
	rec *recordingProvider
}

func runSyntheticArms(t *testing.T, opts fx.Options, arms []evalrunner.Arm, cfg evalrunner.RunConfig) armRun {
	t.Helper()
	d := buildFixture(t, opts)
	corpus := syntheticCorpus(t, d, 1)
	rec := newRecorder()
	cfg.Arms = arms
	cfg.SkipRetrieval = true
	sum, err := evalrunner.Run(corpus, cfg, rec, evalrunner.DeterministicGrader{}, d.Inputs(true))
	if err != nil {
		t.Fatal(err)
	}
	in, err := d.Inputs(true)(fx.Profile, fx.Task, d.AlphaPath)
	if err != nil {
		t.Fatal(err)
	}
	return armRun{d: d, in: in, sum: sum, rec: rec}
}

func (r armRun) request(t *testing.T, arm evalrunner.Arm) evalrunner.GenerationRequest {
	t.Helper()
	reqs := r.rec.reqs[arm]
	if len(reqs) != 1 {
		t.Fatalf("%s: expected exactly one generation request, got %d", arm, len(reqs))
	}
	return reqs[0]
}

func (r armRun) result(arm evalrunner.Arm) evalrunner.CaseResult {
	for _, cr := range r.sum.CaseResults {
		if cr.Arm == arm {
			return cr
		}
	}
	return evalrunner.CaseResult{}
}

func ids(items []resolver.ContextItem) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.RecordID)
	}
	return out
}

func packIDs(p resolver.Pack) []string {
	return ids(evalrunner.ArmContext{Shape: evalrunner.ShapePack, Pack: &p}.Items())
}

func candidateIDs(in resolver.EvalInputs) []string {
	out := []string{}
	for _, c := range in.Eligible.Candidates {
		out = append(out, c.Item.RecordID)
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

func has(xs []string, x string) bool { return indexOf(xs, x) >= 0 }

// contextBlock extracts the rendered CONTEXT block of a prompt.
func contextBlock(t *testing.T, prompt string) string {
	t.Helper()
	start := strings.Index(prompt, "=== CONTEXT ===\n")
	end := strings.Index(prompt, "\n=== END CONTEXT ===")
	if start < 0 || end < start {
		t.Fatalf("prompt has no CONTEXT block:\n%s", prompt)
	}
	return prompt[start+len("=== CONTEXT ===\n") : end]
}

var (
	wantEligible = []string{fx.RecDecisionLoser, fx.RecDecisionWinner, fx.RecFoundation, fx.RecIrrelevantLong1, fx.RecIrrelevantLong2, fx.RecIrrelevantShort, fx.RecLearned, fx.RecPrecedent, fx.RecScopedAlpha}
	wantB4       = []string{fx.RecDecisionWinner, fx.RecFoundation, fx.RecIrrelevantShort, fx.RecScopedAlpha, fx.RecPrecedent, fx.RecLearned}
	stageADenied = []string{fx.RecScopedBeta, fx.RecTaskScoped, fx.RecRevoked, fx.RecDeprecated}
)

// TestSyntheticArmFixturePositiveControls proves every condition the arm
// tests depend on is actually present in the synthetic deployment, so no
// arm assertion can pass vacuously.
func TestSyntheticArmFixturePositiveControls(t *testing.T) {
	r := runSyntheticArms(t, fx.Options{}, []evalrunner.Arm{evalrunner.ArmB4}, evalrunner.RunConfig{})
	in := r.in
	if in.WorkspaceID != fx.WorkspaceAlpha || !in.LearnedEnabled || in.BuildGeneration == "" {
		t.Fatalf("control: workspace=%q learned=%v generation=%q", in.WorkspaceID, in.LearnedEnabled, in.BuildGeneration)
	}
	if got := candidateIDs(in); !sameSet(got, wantEligible) {
		t.Fatalf("control: Stage-A-eligible set %v, want %v", got, wantEligible)
	}
	stored := map[string]contracts.Record{}
	for _, rec := range r.d.Records {
		stored[rec.RecordID] = rec
	}
	for _, id := range stageADenied {
		if _, ok := stored[id]; !ok {
			t.Fatalf("control: denied record %s must exist in the projection", id)
		}
		if has(candidateIDs(in), id) {
			t.Fatalf("control: %s must be Stage-A-denied", id)
		}
	}
	ledger, err := r.d.Runtime.LoadLedger()
	if err != nil {
		t.Fatal(err)
	}
	revoked := false
	for _, rv := range ledger.Revocations {
		revoked = revoked || rv.Key == fx.RecRevoked
	}
	if !revoked {
		t.Fatal("control: the revoked record must be revoked through the durable ledger")
	}

	scores, keys := map[string]float64{}, map[string]string{}
	for _, c := range in.Eligible.Candidates {
		scores[c.Item.RecordID], keys[c.Item.RecordID] = c.Score, c.DecisionKey
	}
	if keys[fx.RecDecisionWinner] != fx.DecisionKey || keys[fx.RecDecisionLoser] != fx.DecisionKey {
		t.Fatalf("control: competing decisions must share %s: %v", fx.DecisionKey, keys)
	}
	if scores[fx.RecDecisionLoser] <= scores[fx.RecDecisionWinner] {
		t.Fatalf("control: the precedence loser must be the more relevant record (%v)", scores)
	}

	b4 := packIDs(in.Pack)
	if !sameSet(b4, wantB4) {
		t.Fatalf("control: B4 selection %v, want %v", b4, wantB4)
	}
	shadowed := false
	for _, c := range in.Pack.Conflicts {
		shadowed = shadowed || (has(c.RecordIDs, fx.RecDecisionWinner) && has(c.RecordIDs, fx.RecDecisionLoser))
	}
	if !shadowed {
		t.Fatalf("control: B4 must record the shadowed competing decision: %+v", in.Pack.Conflicts)
	}
	if !in.Pack.Budget.Truncated || in.Pack.Budget.OmittedAdvisoryCount != 2 {
		t.Fatalf("control: B4 must truncate the two long records: %+v", in.Pack.Budget)
	}
	if !has(ids(in.Pack.Precedents), fx.RecPrecedent) || !has(ids(in.Pack.LearnedExperimental), fx.RecLearned) {
		t.Fatal("control: B4 must carry precedent and learned-observation evidence")
	}
	if in.Roles[fx.RecPrecedent] != contracts.RoleEpisodicEvidence || in.Roles[fx.RecLearned] != contracts.RoleLearnedObservation || in.Roles[fx.RecFoundation] != contracts.RoleCanonicalFoundation {
		t.Fatalf("control: roles %v", in.Roles)
	}
	if len(in.Pack.Unknowns) == 0 || len(in.Pack.Provenance) == 0 || in.Pack.Resolution.SourceRevisionDigest == nil {
		t.Fatal("control: B4 must carry unknowns, a provenance manifest, and a source revision digest")
	}
	for _, it := range (evalrunner.ArmContext{Pack: &in.Pack}).Items() {
		if len(it.ProvenanceRefs) == 0 {
			t.Fatalf("control: B4 item %s must carry provenance refs", it.RecordID)
		}
	}
	if in.NoScopePack == nil {
		t.Fatalf("control: the synthetic deployment must be no-scope eligible: %s", in.NoScopeRefusal)
	}
	ns := packIDs(*in.NoScopePack)
	if !has(ns, fx.RecScopedBeta) || !has(ns, fx.RecTaskScoped) || has(ns, fx.RecRevoked) || has(ns, fx.RecDeprecated) {
		t.Fatalf("control: no-scope pack %v", ns)
	}
}

var armLabel = regexp.MustCompile(`\bB[0-4]\b|no-scope|no-provenance|no-unknowns|canonical-only|learned-only`)

// TestArmsReceiveExactlyTheirConstruction asserts, per arm, exactly what
// enters the generation request.
func TestArmsReceiveExactlyTheirConstruction(t *testing.T) {
	r := runSyntheticArms(t, fx.Options{}, evalrunner.AllArms, evalrunner.RunConfig{})
	for _, cr := range r.sum.CaseResults {
		if cr.Outcome != evalrunner.OutcomePassed {
			t.Fatalf("%s: %s (%s)", cr.Arm, cr.Outcome, cr.Reason)
		}
	}
	texts := map[string]string{}
	for _, rec := range r.d.Records {
		texts[rec.RecordID] = rec.CompactText
	}

	for _, arm := range evalrunner.AllArms {
		req := r.request(t, arm)
		again, err := evalrunner.RenderPrompt(req.Task, req.Bootstrap, req.Context)
		if err != nil || again != req.Prompt {
			t.Fatalf("%s: the request prompt must be the deterministic rendering of its inputs", arm)
		}
		if !strings.Contains(req.Prompt, fx.Task) {
			t.Fatalf("%s: prompt lacks the task", arm)
		}
		if m := armLabel.FindString(req.Prompt); m != "" {
			t.Fatalf("%s: prompt reveals an arm label %q", arm, m)
		}
		for _, id := range []string{fx.RecRevoked, fx.RecDeprecated} {
			if strings.Contains(req.Prompt, fx.Marker(id)) {
				t.Fatalf("%s: Stage-A-denied record %s reached the prompt", arm, id)
			}
		}
		if arm != evalrunner.ArmAblNoScope {
			for _, id := range []string{fx.RecScopedBeta, fx.RecTaskScoped} {
				if strings.Contains(req.Prompt, fx.Marker(id)) {
					t.Fatalf("%s: out-of-scope record %s reached the prompt", arm, id)
				}
			}
		}
		if arm != evalrunner.ArmB0 && req.Bootstrap != bootstrap.Text() {
			t.Fatalf("%s: bootstrap must be the real canonical text", arm)
		}
	}

	t.Run("B0", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmB0)
		if req.Bootstrap != "" || req.Context.Shape != evalrunner.ShapeNone || len(req.Context.Items()) != 0 {
			t.Fatalf("B0 must carry no bootstrap and no context: %+v", req.Context)
		}
		if want := "=== TASK ===\n" + fx.Task + "\n=== END TASK ===\n"; req.Prompt != want {
			t.Fatalf("B0 prompt must be the task only:\n%s", req.Prompt)
		}
	})

	t.Run("B1", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmB1)
		if req.Bootstrap != bootstrap.Text() || req.Context.Shape != evalrunner.ShapeNone || len(req.Context.Items()) != 0 {
			t.Fatal("B1 must carry the real bootstrap and no context")
		}
		if !strings.Contains(req.Prompt, strings.TrimRight(bootstrap.Text(), "\n")) || strings.Contains(req.Prompt, "=== CONTEXT ===") || strings.Contains(req.Prompt, "SYNMARK") {
			t.Fatalf("B1 prompt must be bootstrap + task only:\n%s", req.Prompt)
		}
	})

	t.Run("B2", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmB2)
		got := ids(req.Context.Records)
		if req.Context.Shape != evalrunner.ShapeRecords || !sameSet(got, wantEligible) {
			t.Fatalf("B2 must be every Stage-A-eligible record: %v", got)
		}
		if !sort.StringsAreSorted(got) {
			t.Fatalf("B2 must not be task-ordered: %v", got)
		}
		for _, id := range []string{fx.RecDecisionWinner, fx.RecDecisionLoser, fx.RecIrrelevantLong1, fx.RecIrrelevantLong2, fx.RecIrrelevantShort} {
			if !strings.Contains(req.Prompt, fx.Marker(id)) {
				t.Fatalf("B2 prompt lacks %s", id)
			}
		}
		for _, it := range req.Context.Records {
			if it.Text != texts[it.RecordID] {
				t.Fatalf("B2 item %s must be untruncated (%d vs %d bytes)", it.RecordID, len(it.Text), len(texts[it.RecordID]))
			}
		}
		if len(texts[fx.RecIrrelevantLong1]) < 12000 {
			t.Fatal("control: long record must exceed the B4 budget")
		}
	})

	t.Run("B3", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmB3)
		got := ids(req.Context.Records)
		want := []string{}
		for _, id := range candidateIDs(r.in) {
			if id != fx.RecIrrelevantLong1 && id != fx.RecIrrelevantLong2 {
				want = append(want, id)
			}
		}
		if req.Context.Shape != evalrunner.ShapeRecords || strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("B3 must be the relevance-ranked eligible records within the budget:\n got %v\nwant %v", got, want)
		}
		if !has(got, fx.RecDecisionWinner) || !has(got, fx.RecDecisionLoser) || indexOf(got, fx.RecDecisionLoser) > indexOf(got, fx.RecDecisionWinner) {
			t.Fatalf("B3 must hold both competing decisions ranked by relevance: %v", got)
		}
		for _, it := range req.Context.Records {
			if len(it.ProvenanceRefs) != 0 || it.ExpandRef != "" {
				t.Fatalf("B3 item %s carries provenance", it.RecordID)
			}
			for _, reason := range it.SelectionReason {
				if strings.Contains(reason, "provenance") || strings.Contains(reason, "decision_key") {
					t.Fatalf("B3 selection reason %q cites provenance or precedence", reason)
				}
			}
		}
		// the shared bootstrap text itself mentions provenance; the arm's
		// context is what must carry no provenance mechanism
		ctx := contextBlock(t, req.Prompt)
		for _, s := range []string{"prov_", "provenance", "shadowed", "decision_key"} {
			if strings.Contains(ctx, s) {
				t.Fatalf("B3 context contains %q", s)
			}
		}
	})

	t.Run("B4", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmB4)
		if req.Context.Shape != evalrunner.ShapePack || !sameSet(ids(req.Context.Items()), wantB4) {
			t.Fatalf("B4 must be the Session.Resolve pack: %v", ids(req.Context.Items()))
		}
		if !req.Context.Pack.Budget.Truncated || req.Context.Pack.Budget.OmittedAdvisoryCount != 2 {
			t.Fatalf("B4 must truncate: %+v", req.Context.Pack.Budget)
		}
		if strings.Contains(req.Prompt, fx.Marker(fx.RecDecisionLoser)) || !strings.Contains(req.Prompt, "shadowed") || !strings.Contains(req.Prompt, "prov_") {
			t.Fatal("B4 prompt must select one winner, disclose the conflict, and carry provenance")
		}
	})

	t.Run("no-scope", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmAblNoScope)
		want := append(append([]string{}, wantB4...), fx.RecScopedBeta, fx.RecTaskScoped)
		if !sameSet(ids(req.Context.Items()), want) || !strings.Contains(req.Prompt, fx.Marker(fx.RecScopedBeta)) {
			t.Fatalf("no-scope must add the out-of-scope synthetic records: %v", ids(req.Context.Items()))
		}
	})

	t.Run("no-provenance", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmAblNoProv)
		p := req.Context.Pack
		if !sameSet(ids(req.Context.Items()), wantB4) {
			t.Fatalf("no-provenance must keep B4's items: %v", ids(req.Context.Items()))
		}
		for _, it := range req.Context.Items() {
			if len(it.ProvenanceRefs) != 0 || it.ExpandRef != "" {
				t.Fatalf("item %s keeps provenance", it.RecordID)
			}
		}
		for _, k := range p.Knowledge {
			if len(k.ProvenanceRefs) != 0 || k.Locator != nil {
				t.Fatal("knowledge ref keeps provenance")
			}
		}
		if len(p.Provenance) != 0 || p.Resolution.SourceRevisionDigest != nil {
			t.Fatal("provenance manifest or source revision digest kept")
		}
		b4 := contextBlock(t, r.request(t, evalrunner.ArmB4).Prompt)
		ctx := contextBlock(t, req.Prompt)
		for _, s := range []string{"prov_", "provenance", "source_revision_digest", "synthetic-knowledge"} {
			if !strings.Contains(b4, s) {
				t.Fatalf("control: B4 context must contain %q", s)
			}
			if strings.Contains(ctx, s) {
				t.Fatalf("no-provenance context still contains %q", s)
			}
		}
	})

	t.Run("no-unknowns", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmAblNoUnknowns)
		if !sameSet(ids(req.Context.Items()), wantB4) || len(req.Context.Pack.Unknowns) != 0 {
			t.Fatal("no-unknowns must be B4 without unknowns")
		}
		if b4 := r.request(t, evalrunner.ArmB4); len(b4.Context.Pack.Unknowns) == 0 || strings.Contains(req.Prompt, "SQL dialect beyond") || !strings.Contains(b4.Prompt, "SQL dialect beyond") {
			t.Fatal("control: B4 must carry the unknown that no-unknowns removes")
		}
	})

	t.Run("canonical-only", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmAblCanonOnly)
		want := []string{fx.RecDecisionWinner, fx.RecFoundation, fx.RecIrrelevantShort, fx.RecScopedAlpha}
		if !sameSet(ids(req.Context.Items()), want) || len(req.Context.Pack.Precedents) != 0 || req.Context.Pack.LearnedExperimental != nil {
			t.Fatalf("canonical-only must drop learned and precedent items: %v", ids(req.Context.Items()))
		}
		for _, e := range req.Context.Pack.Provenance {
			if !evalrunner.CanonicalRoles[contracts.SourceRole(e.SourceRole)] {
				t.Fatalf("canonical-only keeps provenance for role %s", e.SourceRole)
			}
		}
	})

	t.Run("learned-only", func(t *testing.T) {
		req := r.request(t, evalrunner.ArmAblLearnedOnly)
		if got := ids(req.Context.Items()); len(got) != 1 || got[0] != fx.RecLearned {
			t.Fatalf("learned-only must hold only learned observations: %v", got)
		}
		if len(req.Context.Pack.Unknowns) != 0 || len(req.Context.Pack.Conflicts) != 0 {
			t.Fatal("learned-only must not carry resolver unknowns or conflicts")
		}
	})
}

// mutatingProvider mutates everything reachable from each request and
// records whether any request arrived already mutated.
type mutatingProvider struct {
	evalrunner.MockProvider
	seen    []string
	mutated int
}

func (m *mutatingProvider) Generate(req evalrunner.GenerationRequest) (evalrunner.GenerationResult, error) {
	data, _ := json.Marshal(req.Context)
	if strings.Contains(req.Prompt, "MUTATED") || strings.Contains(string(data), "MUTATED") {
		m.seen = append(m.seen, fmt.Sprintf("%s rep%d", req.Arm, req.Repeat))
	}
	gen, err := m.MockProvider.Generate(req)
	mutateContext(&req.Context)
	if after, _ := json.Marshal(req.Context); strings.Contains(string(after), "MUTATED") {
		m.mutated++
	}
	return gen, err
}

func mutateItems(items []resolver.ContextItem) {
	for i := range items {
		items[i].Text, items[i].RecordID = "MUTATED", "MUTATED"
		for j := range items[i].ProvenanceRefs {
			items[i].ProvenanceRefs[j] = "MUTATED"
		}
		for j := range items[i].SelectionReason {
			items[i].SelectionReason[j] = "MUTATED"
		}
	}
}

func mutateContext(c *evalrunner.ArmContext) {
	mutateItems(c.Records)
	p := c.Pack
	if p == nil {
		return
	}
	mutateItems(p.Constraints)
	mutateItems(p.Guidance)
	mutateItems(p.Precedents)
	mutateItems(p.LearnedExperimental)
	for i := range p.Unknowns {
		p.Unknowns[i].Question = "MUTATED"
	}
	for i := range p.Conflicts {
		for j := range p.Conflicts[i].RecordIDs {
			p.Conflicts[i].RecordIDs[j] = "MUTATED"
		}
	}
	for i := range p.Provenance {
		p.Provenance[i].SourceRole = "MUTATED"
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

// TestArmsCannotMutateEachOthersInput: a provider that mutates everything
// reachable from its request (item texts, provenance ref and reason slice
// elements, pointers) never affects a later arm or repeat.
func TestArmsCannotMutateEachOthersInput(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	corpus := syntheticCorpus(t, d, 1)
	p := &mutatingProvider{MockProvider: evalrunner.NewMockProvider("")}
	sum, err := evalrunner.Run(corpus, evalrunner.RunConfig{Arms: evalrunner.AllArms, Repeats: 2, SkipRetrieval: true}, p, evalrunner.DeterministicGrader{}, d.Inputs(true))
	if err != nil {
		t.Fatal(err)
	}
	if p.mutated == 0 {
		t.Fatal("control: the provider must actually mutate its requests")
	}
	if len(p.seen) != 0 {
		t.Fatalf("requests arrived carrying another request's mutation: %v", p.seen)
	}
	for _, cr := range sum.CaseResults {
		if cr.Outcome != evalrunner.OutcomePassed {
			t.Fatalf("%s: %s (%s)", cr.Arm, cr.Outcome, cr.Reason)
		}
	}
}

// TestNoScopeRefusedOutsideSyntheticDeployments: no-scope is not_run — and
// never generated — unless every source and record is public synthetic data
// and the network policy is disabled.
func TestNoScopeRefusedOutsideSyntheticDeployments(t *testing.T) {
	arms := []evalrunner.Arm{evalrunner.ArmB4, evalrunner.ArmAblNoScope}

	r := runSyntheticArms(t, fx.Options{PersonalPrivateRecord: true}, arms, evalrunner.RunConfig{})
	present := false
	for _, rec := range r.d.Records {
		present = present || (rec.RecordID == fx.RecPersonalPrivate && rec.Sensitivity == contracts.SensPersonalPrivate)
	}
	if !present {
		t.Fatal("control: the personal_private record must exist")
	}
	if cr := r.result(evalrunner.ArmAblNoScope); cr.Outcome != evalrunner.OutcomeNotRun || !strings.Contains(cr.Reason, "synthetic deployment") || strings.Contains(cr.Reason, fx.RecPersonalPrivate) {
		t.Fatalf("no-scope over personal_private data must be not_run without naming the record: %+v", cr)
	}
	if len(r.rec.reqs[evalrunner.ArmAblNoScope]) != 0 {
		t.Fatal("a refused no-scope arm must never reach the provider")
	}
	if cr := r.result(evalrunner.ArmB4); cr.Outcome != evalrunner.OutcomePassed {
		t.Fatalf("B4 must still run: %+v", cr)
	}

	rt, _ := fixtureDeployment(t)
	in, err := rt.EvalInputs(contracts.ProfileWorkSafe, "cap_eval_nonsynthetic", false, "Choose the storage engine", "")
	if err != nil {
		t.Fatal(err)
	}
	if in.NoScopePack != nil || !strings.Contains(in.NoScopeRefusal, `source "synth-src" is not marked synthetic-*`) {
		t.Fatalf("a source without the synthetic marker must refuse no-scope: %q", in.NoScopeRefusal)
	}

	r3 := runSyntheticArms(t, fx.Options{}, arms, evalrunner.RunConfig{NetworkPolicy: "integration_only"})
	if cr := r3.result(evalrunner.ArmAblNoScope); cr.Outcome != evalrunner.OutcomeNotRun || !strings.Contains(cr.Reason, "network_policy=disabled") || len(r3.rec.reqs[evalrunner.ArmAblNoScope]) != 0 {
		t.Fatalf("no-scope with network access must be not_run: %+v", cr)
	}
}

// TestEvalInputsPackMatchesSessionResolve is the drift guard for
// app.Session.EvalInputs mirroring Session.Resolve: identical selection,
// budget, unknowns, conflicts, and Stage-A eligible set.
func TestEvalInputsPackMatchesSessionResolve(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	in, err := d.Inputs(true)(fx.Profile, fx.Task, d.AlphaPath)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := d.Runtime.Serve(fx.Profile, fx.CapabilityID, true)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()
	pack, trace, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: fx.Task, WorkspaceHint: d.AlphaPath})
	if err != nil {
		t.Fatal(err)
	}
	eligible := []string{}
	for _, st := range trace {
		if st.Step == "stage_a" && st.Outcome == "eligible" {
			eligible = append(eligible, st.RecordID)
		}
	}
	if !sameSet(eligible, candidateIDs(in)) {
		t.Fatalf("Eligible %v differs from Session.Resolve Stage A %v", candidateIDs(in), eligible)
	}
	if !sameSet(packIDs(pack), packIDs(in.Pack)) || pack.Budget != in.Pack.Budget || len(pack.Unknowns) != len(in.Pack.Unknowns) || len(pack.Conflicts) != len(in.Pack.Conflicts) {
		t.Fatalf("EvalInputs pack differs from Session.Resolve: %v vs %v", packIDs(in.Pack), packIDs(pack))
	}
}

// TestNoScopeGateKeepsCapabilityChecks: the no-scope gate drops only the
// workspace and task scope checks.
func TestNoScopeGateKeepsCapabilityChecks(t *testing.T) {
	e := policy.NewEngine(time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	cap := policy.Capability{CapabilityID: "cap", Profile: contracts.ProfileWorkSafe}
	tc := policy.TaskContext{Task: "x", WorkspaceID: "ws-a", TaskKinds: []string{"architecture"}}
	base := contracts.Record{RecordID: "rec_x", SourceID: "synthetic-a", Kind: contracts.KindHeuristic, Status: contracts.StatusActive,
		SourceRole: contracts.RoleCanonicalKnowledge, Trust: contracts.TrustCanonical, Sensitivity: contracts.SensPublicGeneral}

	scoped := base
	scoped.Scope = contracts.Scope{WorkspaceIDs: []string{"ws-b"}, TaskKinds: []string{"deployment"}}
	if e.Evaluate(scoped, cap, tc, nil, false).Eligible || !resolver.EvaluateWithoutScope(e, scoped, cap, tc, nil, false).Eligible {
		t.Fatal("scope-only exclusion must be lifted by the no-scope gate and only by it")
	}
	denied := map[string]contracts.Record{}
	private := scoped
	private.Sensitivity = contracts.SensPersonalPrivate
	denied["personal_private under work-safe"] = private
	deprecated := scoped
	deprecated.Status = contracts.StatusDeprecated
	denied["deprecated"] = deprecated
	learned := scoped
	learned.SourceRole = contracts.RoleLearnedObservation
	denied["learned without enablement"] = learned
	for name, rec := range denied {
		if resolver.EvaluateWithoutScope(e, rec, cap, tc, nil, false).Eligible {
			t.Fatalf("%s must stay denied without scope", name)
		}
	}
	if resolver.EvaluateWithoutScope(e, scoped, cap, tc, map[string]bool{"rec_x": true}, false).Eligible {
		t.Fatal("revocation must stay applied without scope")
	}
}
