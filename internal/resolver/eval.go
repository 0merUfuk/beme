package resolver

// Evaluation helpers (evals/EVALUATION_CONTRACT.md §4). They expose the
// resolver's own Stage-A gate, relevance scoring, and pipeline to evaluation
// baselines, so no baseline re-implements a capability check or a ranking.
// Nothing in this file is on the serving path.

import (
	"fmt"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
)

// Candidate is one Stage-A-eligible record scored for relevance to the task,
// before precedence resolution or budgeting.
type Candidate struct {
	Item        ContextItem          `json:"item"`
	SourceRole  contracts.SourceRole `json:"source_role"`
	DecisionKey string               `json:"decision_key,omitempty"`
	Score       float64              `json:"score"`
	// Tokens is the deterministic token estimate the resolver budgets with.
	Tokens int `json:"tokens"`
}

// EligibleSet is Stage A plus relevance scoring with no precedence and no
// budget: every eligible record, in relevance order (score descending, then
// record ID ascending — the resolver's own ordering).
type EligibleSet struct {
	Facets     []string    `json:"facets"`
	Candidates []Candidate `json:"candidates"`
	Excluded   int         `json:"excluded"`
}

// EligibleCandidates applies the serving Stage-A gate (sensitivity, profile,
// status/validity, workspace scope, trust, revocation, task scope — exactly
// policy.Engine.Evaluate with opts.Revoked) and ranks every eligible record.
func EligibleCandidates(store Store, cap policy.Capability, tc policy.TaskContext, req contracts.ResolutionRequest, opts Options) EligibleSet {
	return eligibleWith(store, cap, tc, req, policyEvaluator(opts))
}

// EligibleCandidatesWithoutScope is EligibleCandidates with the
// workspace-scope and task-scope checks disabled (no-scope ablation). Every
// other Stage-A check still applies.
func EligibleCandidatesWithoutScope(store Store, cap policy.Capability, tc policy.TaskContext, req contracts.ResolutionRequest, opts Options) EligibleSet {
	return eligibleWith(store, cap, tc, req, withoutScopeEvaluator(opts))
}

// ResolveWithoutScope runs the full resolver pipeline — identical ranking,
// precedence, budgeting, unknowns, and pack assembly — with the
// workspace-scope and task-scope Stage-A checks disabled. Evaluation only:
// callers must restrict it to synthetic deployments (§18.6).
func ResolveWithoutScope(store Store, cap policy.Capability, tc policy.TaskContext, req contracts.ResolutionRequest, opts Options) (Pack, []TraceStep) {
	return resolveWith(store, cap, tc, req, opts, withoutScopeEvaluator(opts))
}

// EvaluateWithoutScope is the no-scope Stage-A gate for one record: the
// policy engine's checks in their serving order minus workspace_scope and
// task_scope.
func EvaluateWithoutScope(e *policy.Engine, rec contracts.Record, cap policy.Capability, tc policy.TaskContext, revoked map[string]bool, includeLearned bool) policy.EligibilityResult {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"sensitivity", func() error { return e.CheckSensitivity(rec, cap, tc.WorkspaceID) }},
		{"profile_scope", func() error { return e.CheckProfileScope(rec, cap) }},
		{"status_validity", func() error { return e.CheckStatusValid(rec) }},
		{"trust", func() error { return e.CheckTrust(rec, includeLearned) }},
		{"revocation", func() error { return e.CheckRevocation(rec, revoked) }},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			return policy.EligibilityResult{Eligible: false, Reasons: []string{fmt.Sprintf("%s: %s", s.name, err.Error())}}
		}
	}
	return policy.EligibilityResult{Eligible: true}
}

func withoutScopeEvaluator(opts Options) eligibilityFunc {
	return func(rec contracts.Record, cap policy.Capability, tc policy.TaskContext) policy.EligibilityResult {
		return EvaluateWithoutScope(opts.Policy, rec, cap, tc, opts.Revoked, cap.ExperimentalLearnedGuidance)
	}
}

func eligibleWith(store Store, cap policy.Capability, tc policy.TaskContext, req contracts.ResolutionRequest, evaluate eligibilityFunc) EligibleSet {
	total := len(store.Records())
	eligible, _ := stageA(store, cap, tc, evaluate, nil)
	facets := classifyFacets(req.Task, tc.TaskKinds)
	scored := scoreCandidates(eligible, req, facets, tc)
	set := EligibleSet{Facets: facets, Candidates: make([]Candidate, 0, len(scored)), Excluded: total - len(eligible)}
	for _, sr := range scored {
		item := toContextItem(sr.rec, sr.score, facets, tc, cap)
		item.ExpandRef = "" // no pack is issued, so nothing is expandable
		set.Candidates = append(set.Candidates, Candidate{
			Item:        item,
			SourceRole:  sr.rec.SourceRole,
			DecisionKey: sr.rec.DecisionKey,
			Score:       sr.score,
			Tokens:      estimateTokens([]ContextItem{item}),
		})
	}
	return set
}

// EvalInputs is everything an evaluation run needs for one case, computed
// from one serving session bound to one capability. Every field that carries
// record content passed the Stage-A gate with the durable tombstone ledger
// applied.
type EvalInputs struct {
	Profile        contracts.Profile `json:"profile"`
	CapabilityID   string            `json:"capability_id"`
	LearnedEnabled bool              `json:"learned_enabled"`
	WorkspaceID    string            `json:"workspace_id,omitempty"`

	// Pack is the full Be Me ContextPack (Session.Resolve), used by B4.
	Pack Pack `json:"pack"`
	// Eligible is every Stage-A-eligible record, relevance-ranked, before
	// precedence (B2, B3).
	Eligible EligibleSet `json:"eligible"`
	// NoScopePack is the scope-disabled pack; nil when the deployment is not
	// eligible for the no-scope ablation (NoScopeRefusal says why).
	NoScopePack    *Pack  `json:"no_scope_pack,omitempty"`
	NoScopeRefusal string `json:"no_scope_refusal,omitempty"`
	// NoScopeEligible counts the records eligible with scope filtering
	// disabled (0 when NoScopePack is nil).
	NoScopeEligible int `json:"no_scope_eligible,omitempty"`
	// Roles maps every record ID that can appear in Pack, Eligible, or
	// NoScopePack (including conflict losers) to its source role.
	Roles map[string]contracts.SourceRole `json:"roles"`

	BuildGeneration string `json:"build_generation"`
}
