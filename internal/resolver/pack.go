package resolver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	budget := req.BudgetHintTokens
	if budget <= 0 {
		budget = opts.BudgetTokens
	}
	if budget <= 0 {
		budget = 3000
	}
	sort.SliceStable(winners, func(i, j int) bool {
		if winners[i].score != winners[j].score {
			return winners[i].score > winners[j].score
		}
		return winners[i].rec.RecordID < winners[j].rec.RecordID
	})
	items := map[string]ContextItem{}
	selected := []scoredRecord{}
	advisory := []scoredRecord{}
	for _, sr := range winners {
		items[sr.rec.RecordID] = toContextItem(sr.rec, sr.score, facets, tc, cap)
		if sr.rec.SourceRole != contracts.RoleLearnedObservation && isMandatory(sr.rec) {
			selected = append(selected, sr)
		} else {
			advisory = append(advisory, sr)
		}
	}
	base := Pack{SchemaVersion: contracts.SchemaVersion, PackID: newOpaqueID("ctx"), GeneratedAt: now.Format(time.RFC3339),
		Request:    PackRequest{TaskSummary: summarize(req.Task), TaskKinds: facets, Harness: req.Harness, Risk: strPtrOrNil(tc.Risk)},
		Resolution: PackResolution{CapabilityID: cap.CapabilityID, Profile: string(cap.Profile), ProfileCeiling: string(cap.Profile), Completeness: "complete", PolicyDigest: store.PolicyDigest(), SourceRevisionDigest: strPtrOrNil(store.SourceRevisionDigest()), IndexRevision: store.IndexRevision()},
		Knowledge:  []KnowledgeRef{}, Conflicts: conflictsSection(conflicts), Assumptions: []string{}, TraceRef: newOpaqueID("trace"),
	}
	if opts.PolicyDigest != "" {
		base.Resolution.PolicyDigest = opts.PolicyDigest
	}
	if cap.Profile == contracts.ProfileWorkSafe {
		base.Resolution.PolicyDigest = "withheld"
		base.Resolution.IndexRevision = "withheld"
		base.Resolution.SourceRevisionDigest = nil
	}
	// Admission costs the actual compact JSON envelope, including explanations,
	// unknowns, provenance, section labels and the omission notice. Stable opaque
	// IDs are allocated once, so probing admission cannot change size or ranking.
	assemble := func(delivered []scoredRecord, omitted int) Pack {
		p := base
		p.Constraints = []ContextItem{}
		p.Guidance = []ContextItem{}
		p.Precedents = []ContextItem{}
		evidence := make([]scoredRecord, 0, len(delivered))
		for _, sr := range delivered {
			item := items[sr.rec.RecordID]
			switch {
			case sr.rec.SourceRole == contracts.RoleLearnedObservation:
				p.LearnedExperimental = append(p.LearnedExperimental, item)
			case isMandatory(sr.rec):
				p.Constraints = append(p.Constraints, item)
			case sr.rec.Kind == contracts.KindPrecedent:
				p.Precedents = append(p.Precedents, item)
			default:
				p.Guidance = append(p.Guidance, item)
			}
			// Only delivered text can answer a material unknown; not an omitted full
			// statement, title or semantic key living solely in the index.
			sr.rec.Statement = item.Text
			sr.rec.Title = ""
			sr.rec.Key = ""
			evidence = append(evidence, sr)
		}
		sortContextItems(p.Constraints)
		sortContextItems(p.Guidance)
		sortContextItems(p.Precedents)
		sortContextItems(p.LearnedExperimental)
		p.Unknowns = deriveUnknowns(evidence, req, tc, facets)
		p.Provenance = provenanceManifest(delivered, cap, store)
		p.Degradations = []PackDegradation{}
		for _, d := range opts.Degradations {
			p.Degradations = append(p.Degradations, PackDegradation{Kind: d.Kind, Detail: d.Detail})
		}
		if len(opts.Degradations) > 0 {
			p.Resolution.Completeness = "incomplete"
		}
		p.Budget = PackBudget{RequestedTokens: budget, OmittedAdvisoryCount: omitted, Truncated: omitted > 0}
		if omitted > 0 {
			p.Degradations = append(p.Degradations, PackDegradation{Kind: "reduced_assurance", Detail: fmt.Sprintf("token budget %d: %d advisory items omitted; mandatory content preserved", budget, omitted)})
			for i := range p.Unknowns {
				p.Unknowns[i].WhyMaterial += " Relevant evidence may have been omitted by the token budget; absence from this pack is not absence from the sources."
			}
		}
		p.RecountTokens()
		return p
	}
	p := assemble(selected, len(advisory))
	// Mandatory content (including honest unknowns/degradations) is never
	// dropped. It cannot license an unlimited advisory tail when it overflows.
	// Try complete candidate packs even if the empty pack's unknowns would
	// overflow: delivering evidence can replace an unknown and cost less.
	omitted := len(advisory)
	for _, sr := range advisory {
		trial := append(append([]scoredRecord{}, selected...), sr)
		candidate := assemble(trial, omitted-1)
		if candidate.Budget.EstimatedTokens <= budget {
			selected = trial
			p = candidate
			omitted--
		}
	}
	if p.Budget.EstimatedTokens > budget {
		p.Resolution.Completeness = "incomplete"
		p.Budget.Truncated = true
		p.Degradations = append(p.Degradations, PackDegradation{Kind: "reduced_assurance", Detail: "mandatory content and output metadata exceed the token budget; no advisory evidence admitted"})
		p.RecountTokens()
	}
	deliveredIDs := map[string]bool{}
	for _, item := range p.DeliveredItems() {
		deliveredIDs[item.RecordID] = true
	}
	for _, sr := range advisory {
		if !deliveredIDs[sr.rec.RecordID] {
			*trace = append(*trace, TraceStep{Step: "budget", RecordID: sr.rec.RecordID, Outcome: "omitted by token budget"})
		}
	}
	*trace = append(*trace, TraceStep{Step: "pack", Outcome: fmt.Sprintf("pack %s: constraints=%d guidance=%d precedents=%d learned=%d unknowns=%d conflicts=%d", p.PackID, len(p.Constraints), len(p.Guidance), len(p.Precedents), len(p.LearnedExperimental), len(p.Unknowns), len(p.Conflicts))})
	return p
}

// DeliveredItems is the complete delivered evidence view, shared by metrics,
// pack authorization and callers. It includes experimental learned evidence.
func (p Pack) DeliveredItems() []ContextItem {
	out := make([]ContextItem, 0, len(p.Constraints)+len(p.Guidance)+len(p.Precedents)+len(p.LearnedExperimental))
	out = append(out, p.Constraints...)
	out = append(out, p.Guidance...)
	out = append(out, p.Precedents...)
	out = append(out, p.LearnedExperimental...)
	return out
}

// RecountTokens accounts for the entire compact serialized pack, not just
// item text. This remains a deterministic byte/4 estimate, not a model tokenizer.
// Call again after adding a serving-boundary degradation.
func (p *Pack) RecountTokens() {
	for {
		b, _ := json.Marshal(p)
		estimate := (len(b) + 3) / 4
		if estimate == p.Budget.EstimatedTokens {
			return
		}
		p.Budget.EstimatedTokens = estimate
	}
}

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
		{"dialect", "Preferred SQL dialect beyond documented needs", "No approved evidence is delivered in this pack for dialect preference"},
		{"vendor", "Preferred vendor for this component class", "No approved evidence is delivered in this pack for vendor preference"},
		{"library", "Preferred library for this component class", "No approved evidence is delivered in this pack for library preference"},
		{"framework", "Preferred framework for this component class", "No approved evidence is delivered in this pack for framework preference"},
	}
	for _, pr := range probes {
		if strings.Contains(t, pr.pattern) && !evidenceCovers(winners, pr.pattern) {
			esc := "proceed_with_assumption"
			unknowns = append(unknowns, Unknown{Question: pr.question, WhyMaterial: pr.why, SuggestedEscalation: &esc})
		}
	}
	// No retained record is task evidence: say so, so an agent does not read
	// always-applicable principles or an empty pack as the owner's choice.
	if !hasAdvisoryEvidence(winners) {
		esc := "proceed_with_assumption"
		unknowns = append(unknowns, Unknown{
			Question:            NoEvidenceQuestion,
			WhyMaterial:         "No approved personal preference, precedent or guidance is delivered for this task; any unsupported preference would be an assumption, not the owner's recorded choice",
			SuggestedEscalation: &esc,
		})
	}
	return unknowns
}

// NoEvidenceQuestion is the unknown a pack carries when no advisory record
// supports the task.
const NoEvidenceQuestion = "No recorded preference, precedent or guidance is available in this pack for this task"

func evidenceCovers(winners []scoredRecord, pattern string) bool {
	for _, w := range winners {
		text := strings.ToLower(w.rec.Title + " " + w.rec.Statement + " " + w.rec.Key)
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

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
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SourceRole != out[j].SourceRole {
			return out[i].SourceRole < out[j].SourceRole
		}
		// Tie-break only on the already-redacted public tuple. Hidden source
		// identifiers must never influence work-safe array order.
		a, _ := json.Marshal(out[i])
		b, _ := json.Marshal(out[j])
		return string(a) < string(b)
	})
	return out
}
