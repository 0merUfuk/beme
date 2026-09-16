package app

// Evaluation inputs (evals/EVALUATION_CONTRACT.md §4). One serving session
// produces every arm's raw material for a case, so all baselines and
// ablations share the same capability, the same Stage-A gate, and the same
// durable revocation/purge state (EffectiveRevoked). No arm reads the store
// around that gate.

import (
	"fmt"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
	"github.com/0merUfuk/beme/internal/resolver"
	"github.com/0merUfuk/beme/internal/storage"
)

// SyntheticSourcePrefix marks a registered source as synthetic evaluation
// data. It is one half of the no-scope eligibility rule (see
// NoScopeRefusal); the sensitivity checks are the other half.
const SyntheticSourcePrefix = "synthetic-"

// EvalInputs opens a session under (profile, capabilityID,
// experimentalLearned) and computes the evaluation inputs for one task.
func (rt *Runtime) EvalInputs(profile contracts.Profile, capabilityID string, experimentalLearned bool, task, workspaceHint string) (resolver.EvalInputs, error) {
	sess, err := rt.Serve(profile, capabilityID, experimentalLearned)
	if err != nil {
		return resolver.EvalInputs{}, err
	}
	defer sess.Store.Close()
	return sess.EvalInputs(task, workspaceHint)
}

// EvalInputs computes the evaluation inputs for one task in this session:
//   - Pack: Session.Resolve, unchanged (B4);
//   - Eligible: the same Stage-A gate and revocation set, every eligible
//     record relevance-ranked before precedence (B2, B3);
//   - NoScopePack: the same pipeline with scope filtering disabled, computed
//     only when NoScopeRefusal finds the deployment wholly synthetic.
func (s *Session) EvalInputs(task, workspaceHint string) (resolver.EvalInputs, error) {
	req := contracts.ResolutionRequest{
		SchemaVersion: contracts.SchemaVersion,
		Task:          task,
		WorkspaceHint: workspaceHint,
	}
	pack, _, err := s.Resolve(req)
	if err != nil {
		return resolver.EvalInputs{}, err
	}

	// Mirror Session.Resolve's trusted task context and options exactly.
	tc := policy.TaskContext{Task: req.Task, TaskKinds: clampKinds(req.TaskKindHints)}
	if req.WorkspaceHint != "" {
		m := s.Runtime.Registry.Resolve(req.WorkspaceHint)
		if m.Ambiguous {
			return resolver.EvalInputs{}, fmt.Errorf("workspace ambiguous: %s", m.Reason)
		}
		tc.WorkspaceID = m.WorkspaceID
	}
	revoked, _, err := s.Runtime.EffectiveRevoked(s.Capability.Profile, s.Store)
	if err != nil {
		return resolver.EvalInputs{}, err
	}
	now := timeNowUTC()
	opts := resolver.Options{
		Policy:       policy.NewEngine(now),
		Revoked:      revoked,
		Now:          now,
		Degradations: s.degradations,
	}

	in := resolver.EvalInputs{
		Profile:        s.Capability.Profile,
		CapabilityID:   s.Capability.CapabilityID,
		LearnedEnabled: s.Capability.ExperimentalLearnedGuidance,
		WorkspaceID:    tc.WorkspaceID,
		Pack:           pack,
		Eligible:       resolver.EligibleCandidates(s.View, s.Capability, tc, req, opts),
		Roles:          map[string]contracts.SourceRole{},
	}
	in.BuildGeneration, _ = s.Store.GetMeta("build_generation")
	for _, c := range in.Eligible.Candidates {
		in.Roles[c.Item.RecordID] = c.SourceRole
	}

	refusal, err := s.Runtime.NoScopeRefusal(s.Store)
	if err != nil {
		return resolver.EvalInputs{}, err
	}
	if refusal != "" {
		in.NoScopeRefusal = refusal
		return in, nil
	}
	noScope, _ := resolver.ResolveWithoutScope(s.View, s.Capability, tc, req, opts)
	in.NoScopePack = &noScope
	noScopeSet := resolver.EligibleCandidatesWithoutScope(s.View, s.Capability, tc, req, opts)
	in.NoScopeEligible = len(noScopeSet.Candidates)
	for _, c := range noScopeSet.Candidates {
		in.Roles[c.Item.RecordID] = c.SourceRole
	}
	return in, nil
}

// NoScopeRefusal returns "" only when the deployment may run the no-scope
// ablation, which disables workspace and task scope filtering and therefore
// must never touch personal_private or work_restricted data (§18.6). The rule:
//
//  1. at least one source is registered;
//  2. every registered source's source_id starts with SyntheticSourcePrefix
//     and its descriptor sensitivity is public_general;
//  3. every record in the projection store — including revoked and purged
//     ones, so the check is strictly broader than any arm's view — has
//     sensitivity public_general and belongs to a registered synthetic
//     source.
//
// Anything else returns a non-empty reason. The record scan reads metadata
// for this refusal decision only; no record content flows from it into an
// arm. The runner additionally requires network_policy=disabled.
func (rt *Runtime) NoScopeRefusal(store *storage.Store) (string, error) {
	if len(rt.Sources) == 0 {
		return "no-scope requires a synthetic deployment: no registered sources", nil
	}
	synthetic := map[string]bool{}
	for _, sd := range rt.Sources {
		if !strings.HasPrefix(sd.SourceID, SyntheticSourcePrefix) {
			return fmt.Sprintf("no-scope requires a synthetic deployment: source %q is not marked %s*", sd.SourceID, SyntheticSourcePrefix), nil
		}
		if sd.Sensitivity != contracts.SensPublicGeneral {
			return fmt.Sprintf("no-scope requires a synthetic deployment: source %q has sensitivity %q (public_general required)", sd.SourceID, sd.Sensitivity), nil
		}
		synthetic[sd.SourceID] = true
	}
	recs, err := store.AllRecords()
	if err != nil {
		return "", err
	}
	for _, rec := range recs {
		if rec.Sensitivity != contracts.SensPublicGeneral {
			// The record's identity is deliberately not named: a refusal
			// must not reveal which non-public record exists.
			return "no-scope requires a synthetic deployment: the projection holds a record whose sensitivity is not public_general", nil
		}
		if !synthetic[rec.SourceID] {
			return "no-scope requires a synthetic deployment: the projection holds a record from a source that is not a registered synthetic source", nil
		}
	}
	return "", nil
}
