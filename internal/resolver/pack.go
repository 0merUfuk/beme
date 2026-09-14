package resolver

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
)

// Pack is the canonical versioned ContextPack (schemas/context-pack).
type Pack struct {
	SchemaVersion       string            `json:"schema_version"`
	PackID              string            `json:"pack_id"`
	GeneratedAt         string            `json:"generated_at"`
	Request             PackRequest       `json:"request"`
	Resolution          PackResolution    `json:"resolution"`
	Constraints         []ContextItem     `json:"constraints"`
	Guidance            []ContextItem     `json:"guidance"`
	Precedents          []ContextItem     `json:"precedents"`
	Knowledge           []KnowledgeRef    `json:"knowledge"`
	Unknowns            []Unknown         `json:"unknowns"`
	Conflicts           []PackConflict    `json:"conflicts"`
	Assumptions         []string          `json:"assumptions"`
	Degradations        []PackDegradation `json:"degradations"`
	LearnedExperimental []ContextItem     `json:"learned_experimental,omitempty"`
	Provenance          []PackProvenance  `json:"provenance"`
	Budget              PackBudget        `json:"budget"`
	TraceRef            string            `json:"trace_ref"`
}

type PackRequest struct {
	TaskSummary  string   `json:"task_summary"`
	TaskKinds    []string `json:"task_kinds"`
	RepositoryID *string  `json:"repository_id"`
	Harness      *string  `json:"harness"`
	Risk         *string  `json:"risk"`
}

type PackResolution struct {
	CapabilityID         string  `json:"capability_id"`
	Profile              string  `json:"profile"`
	ProfileCeiling       string  `json:"profile_ceiling"`
	Completeness         string  `json:"completeness"`
	PolicyDigest         string  `json:"policy_digest"`
	SourceRevisionDigest *string `json:"source_revision_digest,omitempty"`
	IndexRevision        string  `json:"index_revision"`
}

type ContextItem struct {
	RecordID        string   `json:"record_id"`
	Kind            string   `json:"kind"`
	Text            string   `json:"text"`
	Force           string   `json:"force"`
	Confidence      string   `json:"confidence"`
	Authority       string   `json:"authority"`
	Sensitivity     string   `json:"sensitivity"`
	SelectionReason []string `json:"selection_reason"`
	ProvenanceRefs  []string `json:"provenance_refs"`
	ExpandRef       string   `json:"expand_ref,omitempty"`
}

type KnowledgeRef struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Locator        *string  `json:"locator,omitempty"`
	ProvenanceRefs []string `json:"provenance_refs,omitempty"`
}

type Unknown struct {
	Question            string  `json:"question"`
	WhyMaterial         string  `json:"why_material"`
	SuggestedEscalation *string `json:"suggested_escalation,omitempty"`
}

type PackConflict struct {
	State       string   `json:"state"`
	Description string   `json:"description"`
	RecordIDs   []string `json:"record_ids,omitempty"`
}

type PackDegradation struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

type PackProvenance struct {
	SourceID       *string `json:"source_id"`
	SourceRole     string  `json:"source_role"`
	RecordCount    int     `json:"record_count"`
	SourceRevision *string `json:"source_revision,omitempty"`
	ContentHash    *string `json:"content_hash,omitempty"`
}

type PackBudget struct {
	RequestedTokens      int  `json:"requested_tokens"`
	EstimatedTokens      int  `json:"estimated_tokens"`
	Truncated            bool `json:"truncated"`
	OmittedAdvisoryCount int  `json:"omitted_advisory_count"`
}

// estimateTokens: deterministic token estimate (~4 chars/token).
func estimateTokens(items []ContextItem) int {
	n := 0
	for _, it := range items {
		n += len(it.Text) / 4
	}
	return n
}

func newOpaqueID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure should never happen; fallback keeps determinism
		return fmt.Sprintf("%s_fallback0000", prefix)
	}
	return prefix + "_" + hex.EncodeToString(b)
}

// buildPack assembles sections from winners, applying budget policy (§11.3):
// mandatory content is reserved first; only advisory sections truncate.
func buildPack(winners []scoredRecord, conflicts []ConflictInfo, req contracts.ResolutionRequest,
	cap policy.Capability, tc policy.TaskContext, store Store, opts Options,
	facets []string, now time.Time, trace *[]TraceStep) Pack {

	packID := newOpaqueID("ctx")
	traceRef := newOpaqueID("trace")

	constraints := []ContextItem{}
	guidance := []ContextItem{}
	precedents := []ContextItem{}
	learned := []ContextItem{}

	// learned observations go to the dedicated non-normative section only
	// (never constraints), and only when explicitly enabled.
	for _, sr := range winners {
		rec := sr.rec
		item := toContextItem(rec, sr.score, facets, tc, cap)
		switch {
		case rec.SourceRole == contracts.RoleLearnedObservation:
			learned = append(learned, item)
		case rec.SourceRole == contracts.RoleTrustedProjectPolicy:
			constraints = append(constraints, item)
		case rec.Kind == contracts.KindPrecedent:
			precedents = append(precedents, item)
		case rec.Authority == contracts.AuthorityDefault && (rec.Kind == contracts.KindDirective || rec.Kind == contracts.KindPrinciple) && rec.Criticality == "high":
			constraints = append(constraints, item)
		default:
			guidance = append(guidance, item)
		}
	}

	// Deterministic section ordering (NFR-002).
	sortContextItems(constraints)
	sortContextItems(guidance)
	sortContextItems(precedents)
	sortContextItems(learned)

	// Budget: mandatory = constraints + conflicts + unknowns + degradations.
	budget := req.BudgetHintTokens
	if budget <= 0 {
		budget = opts.BudgetTokens
	}
	if budget <= 0 {
		budget = 3000
	}
	mandatoryTokens := estimateTokens(constraints)
	for _, c := range conflicts {
		mandatoryTokens += tokenLen(c.DecisionKey) + tokenLen(c.WinnerID) + tokenLen(c.LoserID)
	}
	unknowns := deriveUnknowns(winners, req, tc, facets)
	mandatoryTokens += tokenLen(unknownsText(unknowns))

	remaining := budget - mandatoryTokens
	omitted := 0
	truncated := false
	if remaining < 0 {
		// Mandatory content does not fit: completeness incomplete + explicit
		// degradation (never silently dropped, FR-035).
		truncated = true
		*trace = append(*trace, TraceStep{Step: "budget", Outcome: fmt.Sprintf("mandatory %d > budget %d", mandatoryTokens, budget)})
	} else {
		// Advisory sections fill the remainder in priority order:
		// guidance -> precedents -> learned (explicitly enabled only).
		used := 0
		g2, gUsed, gOmit := budgetSection(guidance, remaining-used)
		used += gUsed
		omitted += gOmit
		p2, pUsed, pOmit := budgetSection(precedents, remaining-used)
		used += pUsed
		omitted += pOmit
		l2, lUsed, lOmit := budgetSection(learned, remaining-used)
		used += lUsed
		omitted += lOmit
		guidance, precedents, learned = g2, p2, l2
		if omitted > 0 {
			truncated = true
		}
	}

	degradations := []PackDegradation{}
	for _, d := range opts.Degradations {
		degradations = append(degradations, PackDegradation{Kind: d.Kind, Detail: d.Detail})
	}
	if truncated {
		degradations = append(degradations, PackDegradation{Kind: "reduced_assurance", Detail: fmt.Sprintf("token budget %d: advisory truncation=%d omissions, mandatory content preserved", budget, omitted)})
	}

	// provenance manifest (safe view under work-safe: omit private IDs)
	prov := provenanceManifest(winners, cap, store)

	var harnessP *string
	if req.Harness != nil {
		h := *req.Harness
		harnessP = &h
	}
	var riskP *string
	if tc.Risk != "" {
		r := tc.Risk
		riskP = &r
	}

	completeness := "complete"
	if truncated && omitted > 0 {
		// advisory truncation is budgeted, not incomplete
		completeness = "complete"
	}
	if truncated && mandatoryTokens > budget {
		completeness = "incomplete"
	}

	p := Pack{
		SchemaVersion: contracts.SchemaVersion,
		PackID:        packID,
		GeneratedAt:   now.Format(time.RFC3339),
		Request: PackRequest{
			TaskSummary: summarize(req.Task),
			TaskKinds:   facets,
			Harness:     harnessP,
			Risk:        riskP,
		},
		Resolution: PackResolution{
			CapabilityID:         cap.CapabilityID,
			Profile:              string(cap.Profile),
			ProfileCeiling:       string(cap.Profile),
			Completeness:         completeness,
			PolicyDigest:         store.PolicyDigest(),
			SourceRevisionDigest: strPtrOrNil(store.SourceRevisionDigest()),
			IndexRevision:        store.IndexRevision(),
		},
		Constraints:  constraints,
		Guidance:     guidance,
		Precedents:   precedents,
		Knowledge:    []KnowledgeRef{},
		Unknowns:     unknowns,
		Conflicts:    conflictsSection(conflicts),
		Assumptions:  []string{},
		Degradations: degradations,
		Provenance:   prov,
		Budget: PackBudget{
			RequestedTokens:      budget,
			EstimatedTokens:      mandatoryTokens + estimateTokens(guidance) + estimateTokens(precedents) + estimateTokens(learned),
			Truncated:            truncated,
			OmittedAdvisoryCount: omitted,
		},
		TraceRef: traceRef,
	}
	if len(learned) > 0 {
		p.LearnedExperimental = learned
	}
	*trace = append(*trace, TraceStep{Step: "pack", Outcome: fmt.Sprintf("pack %s: constraints=%d guidance=%d precedents=%d unknowns=%d conflicts=%d", packID, len(constraints), len(guidance), len(precedents), len(unknowns), len(p.Conflicts))})
	return p
}

// (degradationsHolder removed — DegradationInput lives in Options.)

func toContextItem(rec contracts.Record, score float64, facets []string, tc policy.TaskContext, cap policy.Capability) ContextItem {
	force := "should"
	switch {
	case rec.SourceRole == contracts.RoleTrustedProjectPolicy:
		force = "must"
	case rec.Authority == contracts.AuthorityDefault && rec.Criticality == "high":
		force = "must"
	}
	text := rec.CompactText
	if text == "" {
		text = rec.Statement
	}
	reasons := []string{}
	if rec.DecisionKey != "" {
		reasons = append(reasons, "decision_key matched: "+rec.DecisionKey)
	}
	for _, k := range rec.Scope.TaskKinds {
		for _, f := range facets {
			if k == f {
				reasons = append(reasons, "task facet matched: "+k)
			}
		}
	}
	if len(rec.Scope.WorkspaceIDs) > 0 {
		reasons = append(reasons, "scope matched: workspace "+strings.Join(rec.Scope.WorkspaceIDs, ","))
	}
	reasons = append(reasons, fmt.Sprintf("retrieval score: %.1f", score))
	reasons = append(reasons, "authority: "+string(rec.Authority))
	return ContextItem{
		RecordID:        rec.RecordID,
		Kind:            string(rec.Kind),
		Text:            text,
		Force:           force,
		Confidence:      string(rec.Confidence),
		Authority:       string(rec.Authority),
		Sensitivity:     rec.Sensitivity,
		SelectionReason: reasons,
		ProvenanceRefs:  rec.ProvenanceRefs,
		ExpandRef:       newOpaqueID("expand"),
	}
}

func sortContextItems(items []ContextItem) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].RecordID < items[j].RecordID })
}

func budgetSection(items []ContextItem, budget int) ([]ContextItem, int, int) {
	out := []ContextItem{}
	used := 0
	omitted := 0
	for _, it := range items {
		cost := len(it.Text) / 4
		if used+cost > budget {
			omitted++
			continue
		}
		used += cost
		out = append(out, it)
	}
	return out, used, omitted
}

func conflictsSection(conflicts []ConflictInfo) []PackConflict {
	out := []PackConflict{}
	seen := map[string]bool{}
	for _, c := range conflicts {
		key := c.DecisionKey + ":" + c.WinnerID + ":" + c.LoserID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, PackConflict{
			State:       string(c.State),
			Description: fmt.Sprintf("decision_key %s: record %s shadowed by %s (deterministic precedence selected winner)", c.DecisionKey, c.LoserID, c.WinnerID),
			RecordIDs:   []string{c.WinnerID, c.LoserID},
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Description < out[j].Description })
	return out
}

// deriveUnknowns: questions material to the task with no reliable evidence
// in the pack (FR-034, P13 negative control: never invent).
func deriveUnknowns(winners []scoredRecord, req contracts.ResolutionRequest, tc policy.TaskContext, facets []string) []Unknown {
	unknowns := []Unknown{}
	t := strings.ToLower(req.Task)
	// Dialect/vendor preference probes — unknown unless evidence present.
	probes := []struct {
		pattern  string
		question string
		why      string
	}{
		{"dialect", "Preferred SQL dialect beyond documented needs", "No approved evidence exists for dialect preference"},
		{"vendor", "Preferred vendor for this component class", "No approved evidence exists for vendor preference"},
		{"library", "Preferred library for this component class", "No approved evidence exists for library preference"},
		{"framework", "Preferred framework for this component class", "No approved evidence exists for framework preference"},
	}
	for _, pr := range probes {
		if strings.Contains(t, pr.pattern) && !evidenceCovers(winners, pr.pattern) {
			esc := "proceed_with_assumption"
			unknowns = append(unknowns, Unknown{Question: pr.question, WhyMaterial: pr.why, SuggestedEscalation: &esc})
		}
	}
	return unknowns
}

func evidenceCovers(winners []scoredRecord, pattern string) bool {
	for _, w := range winners {
		text := strings.ToLower(w.rec.Title + " " + w.rec.Statement + " " + w.rec.Key)
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func unknownsText(unknowns []Unknown) string {
	n := 0
	for _, u := range unknowns {
		n += len(u.Question) / 4
	}
	return strings.Repeat("x", n)
}

func tokenLen(s string) int { return len(s) / 4 }

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func summarize(task string) string {
	task = strings.TrimSpace(task)
	if len(task) <= 120 {
		return task
	}
	return task[:117] + "..."
}

func provenanceManifest(winners []scoredRecord, cap policy.Capability, store Store) []PackProvenance {
	bySource := map[string]*PackProvenance{}
	for _, sr := range winners {
		sp, ok := bySource[sr.rec.SourceID]
		if !ok {
			sp = &PackProvenance{SourceID: &sr.rec.SourceID, SourceRole: string(sr.rec.SourceRole), RecordCount: 0}
			bySource[sr.rec.SourceID] = sp
		}
		sp.RecordCount++
	}
	// Work-safe provenance is a safe scoped view: private source IDs and
	// correlatable content hashes are omitted (FR-039, ADR-005).
	if cap.Profile == contracts.ProfileWorkSafe {
		for _, sp := range bySource {
			sp.SourceID = nil
			sp.SourceRevision = nil
			sp.ContentHash = nil
		}
	}
	out := make([]PackProvenance, 0, len(bySource))
	for _, sp := range bySource {
		out = append(out, *sp)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SourceRole < out[j].SourceRole })
	return out
}
