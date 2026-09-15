package evalrunner

// Arm construction (evals/EVALUATION_CONTRACT.md §4). Every arm is built from
// one resolver.EvalInputs, which a single serving session produced under one
// capability with the durable revocation/purge state applied — so every arm
// shares the same hard capability boundary. Arms never share memory: each
// build deep-copies what it uses, and the runner rebuilds per repeat.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/0merUfuk/beme/internal/bootstrap"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
)

// Context shapes.
const (
	ShapeNone    = "none"    // B0, B1: no personal context
	ShapeRecords = "records" // B2, B3: a flat record list
	ShapePack    = "pack"    // B4 and the ablations: a ContextPack
)

// ArmContext is the personal context one arm gives the model.
type ArmContext struct {
	Shape   string                 `json:"shape"`
	Records []resolver.ContextItem `json:"records,omitempty"`
	Pack    *resolver.Pack         `json:"pack,omitempty"`
}

// Items returns every context item in rendered order.
func (c ArmContext) Items() []resolver.ContextItem {
	if c.Pack == nil {
		return c.Records
	}
	out := []resolver.ContextItem{}
	for _, sec := range [][]resolver.ContextItem{c.Pack.Constraints, c.Pack.Guidance, c.Pack.Precedents, c.Pack.LearnedExperimental} {
		out = append(out, sec...)
	}
	return out
}

// ArmConstruction describes how an arm's context was built. It is recorded
// in the owner-side evidence for every generation.
type ArmConstruction struct {
	Arm Arm `json:"arm"`
	// Source: none | bootstrap_only | stage_a_eligible_dump |
	// stage_a_ranked_retrieval | session_resolve |
	// session_resolve_without_scope.
	Source    string `json:"source"`
	Transform string `json:"transform,omitempty"`
	Ordering  string `json:"ordering,omitempty"`
	// Eligible: Stage-A-eligible records available to the arm.
	Eligible int `json:"eligible"`
	// Selected: items that entered the context.
	Selected int `json:"selected"`
	// Truncated: items dropped by a token budget.
	Truncated int `json:"truncated"`
	// Removed: items or provenance references removed by an ablation
	// transform.
	Removed      int `json:"removed"`
	BudgetTokens int `json:"budget_tokens,omitempty"`
}

// CanonicalRoles defines the canonical-only ablation: items whose source role
// is canonical foundation, canonical reusable knowledge, trusted project
// policy, or declassified-safe (the user-approved safe form of canonical
// content that work-safe projections are built from). Learned observations,
// episodic evidence, and trusted references are excluded, and so is every
// precedent-kind item regardless of role.
var CanonicalRoles = map[contracts.SourceRole]bool{
	contracts.RoleCanonicalFoundation:  true,
	contracts.RoleCanonicalKnowledge:   true,
	contracts.RoleTrustedProjectPolicy: true,
	contracts.RoleDeclassifiedSafe:     true,
}

// armBuild is one arm's constructed input. NotRun is non-empty when the arm
// cannot honestly run on these inputs.
type armBuild struct {
	Context      ArmContext
	Bootstrap    string
	Construction ArmConstruction
	NotRun       string
}

// buildArm constructs one arm from the shared inputs without mutating them.
func buildArm(in resolver.EvalInputs, arm Arm, networkPolicy string) (armBuild, error) {
	b := armBuild{Bootstrap: bootstrap.Text(), Construction: ArmConstruction{Arm: arm}}
	switch arm {
	case ArmB0:
		b.Bootstrap = ""
		b.Context = ArmContext{Shape: ShapeNone}
		b.Construction.Source = "none"
		return b, nil
	case ArmB1:
		b.Context = ArmContext{Shape: ShapeNone}
		b.Construction.Source = "bootstrap_only"
		return b, nil
	case ArmB2:
		return rawDump(b, in)
	case ArmB3:
		return rankedRetrieval(b, in)
	case ArmB4:
		return fullPack(b, in, in.Pack, "session_resolve")
	case ArmAblNoScope:
		if networkPolicy != "disabled" {
			b.NotRun = fmt.Sprintf("no-scope runs only with network_policy=disabled (§18.6); got %q", networkPolicy)
			return b, nil
		}
		if in.NoScopePack == nil {
			reason := in.NoScopeRefusal
			if reason == "" {
				reason = "no scope-disabled pack was produced"
			}
			b.NotRun = reason
			return b, nil
		}
		b, err := fullPack(b, in, *in.NoScopePack, "session_resolve_without_scope")
		b.Construction.Eligible = in.NoScopeEligible
		return b, err
	case ArmAblNoProv:
		b, err := fullPack(b, in, in.Pack, "session_resolve")
		if err != nil {
			return b, err
		}
		b.Construction.Removed = stripProvenance(b.Context.Pack)
		b.Construction.Transform = "all provenance removed: item provenance refs, expand refs, provenance-citing selection reasons, provenance manifest, knowledge refs and locators, learned refs, source revision digest"
		return b, nil
	case ArmAblNoUnknowns:
		b, err := fullPack(b, in, in.Pack, "session_resolve")
		if err != nil {
			return b, err
		}
		b.Construction.Removed = len(b.Context.Pack.Unknowns)
		b.Context.Pack.Unknowns = []resolver.Unknown{}
		b.Construction.Transform = "unknowns removed"
		return b, nil
	case ArmAblCanonOnly:
		return canonicalOnly(b, in)
	case ArmAblLearnedOnly:
		return learnedOnly(b, in)
	}
	b.NotRun = fmt.Sprintf("unknown arm %q", arm)
	return b, nil
}

// rawDump (B2): every Stage-A-eligible record, ordered by record ID, with no
// task-based selection, no precedence reduction, and no budget.
func rawDump(b armBuild, in resolver.EvalInputs) (armBuild, error) {
	cands, err := deepCopy(in.Eligible.Candidates)
	if err != nil {
		return b, err
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Item.RecordID < cands[j].Item.RecordID })
	items := make([]resolver.ContextItem, 0, len(cands))
	for _, c := range cands {
		it := c.Item
		it.ExpandRef = ""
		it.SelectionReason = nil // nothing was selected
		items = append(items, it)
	}
	b.Context = ArmContext{Shape: ShapeRecords, Records: items}
	b.Construction.Source = "stage_a_eligible_dump"
	b.Construction.Ordering = "record_id_asc"
	b.Construction.Transform = "no task selection, no precedence reduction, no budget"
	b.Construction.Eligible = len(cands)
	b.Construction.Selected = len(items)
	return b, nil
}

// rankedRetrieval (B3): Stage-A-eligible records in relevance order, before
// precedence (competing decisions for one decision_key all remain), with no
// provenance, truncated to the B4 pack's token budget by walking the ranked
// list and skipping any record that no longer fits — the resolver's own
// budgetSection rule, applied to one score-ordered list with nothing
// reserved.
func rankedRetrieval(b armBuild, in resolver.EvalInputs) (armBuild, error) {
	budget := in.Pack.Budget.RequestedTokens
	if budget <= 0 {
		b.NotRun = "B3 needs the B4 pack's token budget; the pack reports none"
		return b, nil
	}
	cands, err := deepCopy(in.Eligible.Candidates)
	if err != nil {
		return b, err
	}
	items := []resolver.ContextItem{}
	used, truncated := 0, 0
	for _, c := range cands {
		if used+c.Tokens > budget {
			truncated++
			continue
		}
		used += c.Tokens
		it := c.Item
		it.ProvenanceRefs = nil
		it.ExpandRef = ""
		it.SelectionReason = retrievalReasons(it.SelectionReason)
		items = append(items, it)
	}
	b.Context = ArmContext{Shape: ShapeRecords, Records: items}
	b.Construction.Source = "stage_a_ranked_retrieval"
	b.Construction.Ordering = "relevance_score_desc_record_id_asc"
	b.Construction.Transform = "no precedence, no provenance; plain score-ordered budget truncation"
	b.Construction.Eligible = len(cands)
	b.Construction.Selected = len(items)
	b.Construction.Truncated = truncated
	b.Construction.BudgetTokens = budget
	return b, nil
}

// retrievalReasons keeps only retrieval-derived selection reasons; reasons
// that come from decision keys, scope, or authority are dropped.
func retrievalReasons(reasons []string) []string {
	out := []string{}
	for _, r := range reasons {
		if strings.HasPrefix(r, "retrieval score: ") || strings.HasPrefix(r, "task facet matched: ") {
			out = append(out, r)
		}
	}
	return out
}

func fullPack(b armBuild, in resolver.EvalInputs, src resolver.Pack, source string) (armBuild, error) {
	p, err := deepCopy(src)
	if err != nil {
		return b, err
	}
	b.Context = ArmContext{Shape: ShapePack, Pack: &p}
	b.Construction.Source = source
	b.Construction.Ordering = "precedence winners; sections ordered by record_id"
	b.Construction.Eligible = len(in.Eligible.Candidates)
	b.Construction.Selected = len(b.Context.Items())
	b.Construction.Truncated = p.Budget.OmittedAdvisoryCount
	b.Construction.BudgetTokens = p.Budget.RequestedTokens
	return b, nil
}

// stripProvenance removes every provenance mechanism from p and returns how
// many references or entries were removed.
func stripProvenance(p *resolver.Pack) int {
	n := 0
	for _, sec := range []*[]resolver.ContextItem{&p.Constraints, &p.Guidance, &p.Precedents, &p.LearnedExperimental} {
		for i := range *sec {
			it := &(*sec)[i]
			n += len(it.ProvenanceRefs)
			it.ProvenanceRefs = nil
			it.ExpandRef = ""
			kept := []string{}
			for _, r := range it.SelectionReason {
				if strings.Contains(strings.ToLower(r), "provenance") {
					n++
					continue
				}
				kept = append(kept, r)
			}
			it.SelectionReason = kept
		}
	}
	for i := range p.Knowledge {
		n += len(p.Knowledge[i].ProvenanceRefs)
		p.Knowledge[i].ProvenanceRefs = nil
		if p.Knowledge[i].Locator != nil {
			n++
			p.Knowledge[i].Locator = nil
		}
	}
	n += len(p.Provenance)
	p.Provenance = []resolver.PackProvenance{}
	if p.Resolution.SourceRevisionDigest != nil {
		n++
		p.Resolution.SourceRevisionDigest = nil
	}
	return n
}

// canonicalOnly: the B4 pack restricted to CanonicalRoles, minus
// precedent-kind items. A selected record whose role is unknown makes the
// arm not_run rather than guessing.
func canonicalOnly(b armBuild, in resolver.EvalInputs) (armBuild, error) {
	b, err := fullPack(b, in, in.Pack, "session_resolve")
	if err != nil {
		return b, err
	}
	p := b.Context.Pack
	keep := func(it resolver.ContextItem) (bool, bool) {
		role, ok := in.Roles[it.RecordID]
		return ok && CanonicalRoles[role] && it.Kind != string(contracts.KindPrecedent), ok
	}
	removed, unknown := 0, false
	filter := func(items []resolver.ContextItem) []resolver.ContextItem {
		out := []resolver.ContextItem{}
		for _, it := range items {
			k, known := keep(it)
			if !known {
				unknown = true
			}
			if k {
				out = append(out, it)
			} else {
				removed++
			}
		}
		return out
	}
	p.Constraints, p.Guidance, p.Precedents = filter(p.Constraints), filter(p.Guidance), filter(p.Precedents)
	p.LearnedExperimental = nilIfEmpty(filter(p.LearnedExperimental))
	if unknown {
		b.NotRun = "canonical-only: a selected record has no known source role"
		return b, nil
	}
	conflicts := []resolver.PackConflict{}
	for _, c := range p.Conflicts {
		all := len(c.RecordIDs) > 0
		for _, id := range c.RecordIDs {
			if !CanonicalRoles[in.Roles[id]] {
				all = false
			}
		}
		if all {
			conflicts = append(conflicts, c)
		}
	}
	p.Conflicts = conflicts
	// Knowledge refs carry no source role, so none can be shown canonical.
	removed += len(p.Knowledge)
	p.Knowledge = []resolver.KnowledgeRef{}
	prov := []resolver.PackProvenance{}
	for _, e := range p.Provenance {
		if CanonicalRoles[contracts.SourceRole(e.SourceRole)] {
			prov = append(prov, e)
		}
	}
	p.Provenance = prov
	b.Construction.Selected = len(b.Context.Items())
	b.Construction.Removed = removed
	b.Construction.Transform = "restricted to canonical source roles (canonical_foundation, canonical_reusable_knowledge, trusted_project_policy, declassified_safe); precedent-kind items, learned observations, episodic evidence, and trusted references removed"
	return b, nil
}

// learnedOnly: only learned-observation items from the B4 pack. It needs a
// capability with experimental learned guidance enabled; otherwise the
// capability boundary already excludes every learned observation.
func learnedOnly(b armBuild, in resolver.EvalInputs) (armBuild, error) {
	if !in.LearnedEnabled {
		b.NotRun = "learned-only needs experimental learned guidance (personal profile only); this capability excludes learned observations"
		return b, nil
	}
	b, err := fullPack(b, in, in.Pack, "session_resolve")
	if err != nil {
		return b, err
	}
	p := b.Context.Pack
	removed, unknown := 0, false
	filter := func(items []resolver.ContextItem) []resolver.ContextItem {
		out := []resolver.ContextItem{}
		for _, it := range items {
			role, ok := in.Roles[it.RecordID]
			if !ok {
				unknown = true
			}
			if ok && role == contracts.RoleLearnedObservation {
				out = append(out, it)
			} else {
				removed++
			}
		}
		return out
	}
	p.Constraints, p.Guidance, p.Precedents = filter(p.Constraints), filter(p.Guidance), filter(p.Precedents)
	p.LearnedExperimental = nilIfEmpty(filter(p.LearnedExperimental))
	if unknown {
		b.NotRun = "learned-only: a selected record has no known source role"
		return b, nil
	}
	removed += len(p.Unknowns) + len(p.Conflicts) + len(p.Knowledge)
	p.Unknowns, p.Conflicts, p.Knowledge = []resolver.Unknown{}, []resolver.PackConflict{}, []resolver.KnowledgeRef{}
	prov := []resolver.PackProvenance{}
	for _, e := range p.Provenance {
		if contracts.SourceRole(e.SourceRole) == contracts.RoleLearnedObservation {
			prov = append(prov, e)
		}
	}
	p.Provenance = prov
	b.Construction.Selected = len(b.Context.Items())
	b.Construction.Removed = removed
	b.Construction.Transform = "only learned_observation items; unknowns, conflicts, knowledge refs, and non-learned provenance removed"
	return b, nil
}

func nilIfEmpty(items []resolver.ContextItem) []resolver.ContextItem {
	if len(items) == 0 {
		return nil
	}
	return items
}

// deepCopy returns a structurally independent copy of v (every slice, map,
// and pointer is fresh), so no arm can alias another arm's input.
func deepCopy[T any](v T) (T, error) {
	var out T
	data, err := json.Marshal(v)
	if err != nil {
		return out, fmt.Errorf("deep copy: %w", err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("deep copy: %w", err)
	}
	return out, nil
}
