// Package resolver implements Stage B: facet classification, candidate
// retrieval, lexicographic precedence (§9.2), conflict preservation,
// budgeting, unknowns, and ContextPack emission.
package resolver

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
)

// Store is the read interface the resolver needs from a projection store.
type Store interface {
	Records() []contracts.Record
	Provenance(id string) (contracts.Provenance, bool)
	IndexRevision() string
	PolicyDigest() string
	SourceRevisionDigest() string
}

// TraceStep records one resolver decision for inspectability (NFR-005,
// FR-038: explain uses the actual trace).
type TraceStep struct {
	Step     string   `json:"step"`
	RecordID string   `json:"record_id,omitempty"`
	Outcome  string   `json:"outcome"`
	Reasons  []string `json:"reasons,omitempty"`
}

// Resolve produces a ContextPack. Deterministic: identical inputs produce
// structurally identical packs (NFR-002).
func Resolve(store Store, cap policy.Capability, tc policy.TaskContext, req contracts.ResolutionRequest, opts Options) (Pack, []TraceStep) {
	trace := []TraceStep{}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	trace = append(trace, TraceStep{Step: "capability", Outcome: fmt.Sprintf("bound profile=%s capability=%s", cap.Profile, cap.CapabilityID)})

	// Stage A over every record.
	eligible := []contracts.Record{}
	excluded := map[string]string{}
	for _, rec := range store.Records() {
		res := opts.Policy.Evaluate(rec, cap, tc, opts.Revoked, cap.ExperimentalLearnedGuidance)
		if res.Eligible {
			eligible = append(eligible, rec)
			trace = append(trace, TraceStep{Step: "stage_a", RecordID: rec.RecordID, Outcome: "eligible"})
		} else {
			excluded[rec.RecordID] = strings.Join(res.Reasons, "; ")
			trace = append(trace, TraceStep{Step: "stage_a", RecordID: rec.RecordID, Outcome: "excluded", Reasons: res.Reasons})
		}
	}

	// Stage B: facet classification (deterministic lexical).
	facets := classifyFacets(req.Task, tc.TaskKinds)
	trace = append(trace, TraceStep{Step: "facets", Outcome: strings.Join(facets, ",")})

	// Retrieval scoring: relevance to task text + facet + decision_key.
	scored := scoreCandidates(eligible, req, facets, tc)

	// Precedence resolution per decision_key group.
	groups := groupByDecisionKey(scored)
	winners, conflicts := resolvePrecedence(groups, tc, trace)

	// Budgeting (reserve mandatory first; only advisory truncates).
	pack := buildPack(winners, conflicts, req, cap, tc, store, opts, facets, now, &trace)
	return pack, trace
}

// Options carries trusted resolver inputs.
type Options struct {
	Policy       *policy.Engine
	Revoked      map[string]bool
	Now          time.Time
	BudgetTokens int
	Degradations []DegradationInput
}

// DegradationInput is resolver-visible degradation (blueprint §13.7).
type DegradationInput struct {
	Kind   string
	Detail string
}

func classifyFacets(task string, hints []string) []string {
	facets := map[string]bool{}
	t := strings.ToLower(task)
	rules := [][2]string{
		{"test", "testing"}, {"verify", "testing"}, {"verification", "testing"}, {"qa", "testing"},
		{"cache", "architecture"}, {"database", "architecture"}, {"storage", "architecture"}, {"schema", "architecture"},
		{"design", "architecture"}, {"architect", "architecture"}, {"structure", "architecture"},
		{"deploy", "deployment"}, {"docker", "deployment"}, {"railway", "deployment"}, {"server", "deployment"},
		{"release", "release"}, {"publish", "release"}, {"ship", "release"},
		{"security", "security"}, {"credential", "security"}, {"secret", "security"}, {"auth", "security"},
		{"document", "documentation"}, {"readme", "documentation"}, {"docs", "documentation"},
		{"research", "research"}, {"compare", "research"},
		{"workflow", "workflow"}, {"process", "workflow"},
		{"implement", "implementation"}, {"build", "implementation"}, {"write", "implementation"}, {"code", "implementation"},
		{"reliab", "reliability"}, {"retry", "reliability"}, {"idempoten", "reliability"}, {"failure", "reliability"},
		{"cost", "reliability"}, {"budget", "reliability"}, {"performance", "reliability"},
	}
	for _, r := range rules {
		if strings.Contains(t, r[0]) {
			facets[r[1]] = true
		}
	}
	for _, h := range hints {
		if h != "" {
			facets[h] = true
		}
	}
	if len(facets) == 0 {
		facets["implementation"] = true
	}
	out := make([]string, 0, len(facets))
	for f := range facets {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

type scoredRecord struct {
	rec   contracts.Record
	score float64
}

func scoreCandidates(recs []contracts.Record, req contracts.ResolutionRequest, facets []string, tc policy.TaskContext) []scoredRecord {
	taskLower := strings.ToLower(req.Task)
	out := make([]scoredRecord, 0, len(recs))
	for _, rec := range recs {
		s := 0.0
		// facet match via task_kinds scope
		if len(rec.Scope.TaskKinds) > 0 {
			for _, k := range rec.Scope.TaskKinds {
				for _, f := range facets {
					if k == f {
						s += 3
					}
				}
			}
		}
		// lexical overlap
		text := strings.ToLower(rec.Title + " " + rec.Statement + " " + rec.CompactText + " " + rec.Key)
		for _, w := range strings.FieldsFunc(taskLower, func(r rune) bool { return !unicodeIsWord(r) }) {
			if len(w) > 3 && strings.Contains(text, w) {
				s += 0.5
			}
		}
		// workspace specificity
		if len(rec.Scope.WorkspaceIDs) > 0 {
			s += 1
		}
		// criticality
		switch rec.Criticality {
		case "high":
			s += 1.5
		case "medium":
			s += 0.5
		}
		out = append(out, scoredRecord{rec: rec, score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].rec.RecordID < out[j].rec.RecordID // stable tie-break
	})
	return out
}

func unicodeIsWord(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r > 127
}

func groupByDecisionKey(recs []scoredRecord) map[string][]scoredRecord {
	groups := map[string][]scoredRecord{}
	for _, sr := range recs {
		k := sr.rec.DecisionKey
		if k == "" {
			k = "record:" + sr.rec.RecordID
		}
		groups[k] = append(groups[k], sr)
	}
	return groups
}

// precedenceTier implements §9.2 lexicographic order from strongest (1) to
// weakest (11).
func precedenceTier(rec contracts.Record) int {
	// tier 1: non-bypassable runtime policy — enforced elsewhere (always present)
	if rec.SourceRole == contracts.RoleTrustedProjectPolicy {
		return 3 // trusted project decision/constraint
	}
	if rec.SourceRole == contracts.RoleCanonicalFoundation {
		if len(rec.Scope.WorkspaceIDs) > 0 {
			return 4 // profile-scoped canonical directive
		}
		return 5 // global canonical directive
	}
	if rec.SourceRole == contracts.RoleCanonicalKnowledge {
		if len(rec.Scope.WorkspaceIDs) > 0 || len(rec.Scope.TaskKinds) > 0 {
			return 6 // scoped approved principle/preference
		}
		return 7 // global approved principle/preference
	}
	if rec.SourceRole == contracts.RoleDeclassifiedSafe {
		return 6 // scoped approved principle (safe form)
	}
	switch rec.Kind {
	case contracts.KindPrecedent, contracts.KindPattern, contracts.KindWorkflow, contracts.KindHeuristic, contracts.KindFailureMode:
		return 8 // validated precedent/pattern/workflow/heuristic/failure mode
	case contracts.KindFact:
		return 8
	}
	if rec.Trust == contracts.TrustUntrustedData {
		return 10 // untrusted content: data only
	}
	if rec.SourceRole == contracts.RoleLearnedObservation {
		return 9 // observed pattern
	}
	return 7
}

func resolvePrecedence(groups map[string][]scoredRecord, tc policy.TaskContext, trace []TraceStep) (winners []scoredRecord, conflicts []ConflictInfo) {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		g := groups[k]
		// sort by tier, then narrower scope, then explicit supersession, then
		// stronger evidence, then retrieval relevance, then record ID.
		sort.SliceStable(g, func(i, j int) bool {
			a, b := g[i].rec, g[j].rec
			ti, tj := precedenceTier(a), precedenceTier(b)
			if ti != tj {
				return ti < tj
			}
			si, sj := len(a.Scope.WorkspaceIDs)+len(a.Scope.TaskKinds), len(b.Scope.WorkspaceIDs)+len(b.Scope.TaskKinds)
			if si != sj {
				return si > sj // narrower applicable scope wins
			}
			if len(a.Relationships.Supersedes) > 0 != (len(b.Relationships.Supersedes) > 0) {
				return len(a.Relationships.Supersedes) > 0
			}
			if a.Confidence != b.Confidence {
				return a.Confidence == contracts.ConfidenceValidated // stronger evidence
			}
			if g[i].score != g[j].score {
				return g[i].score > g[j].score
			}
			return a.RecordID < b.RecordID
		})
		winner := g[0]
		winners = append(winners, winner)
		// conflict detection: same decision_key, different acceptable answers.
		if len(g) > 1 {
			for _, loser := range g[1:] {
				if loser.rec.Confidence == winner.rec.Confidence && precedenceTier(loser.rec) == precedenceTier(winner.rec) {
					conflicts = append(conflicts, ConflictInfo{
						State:       contracts.ConflictShadowed,
						DecisionKey: k,
						WinnerID:    winner.rec.RecordID,
						LoserID:     loser.rec.RecordID,
					})
				} else {
					conflicts = append(conflicts, ConflictInfo{
						State:       contracts.ConflictShadowed,
						DecisionKey: k,
						WinnerID:    winner.rec.RecordID,
						LoserID:     loser.rec.RecordID,
					})
				}
			}
		}
	}
	return winners, conflicts
}

type ConflictInfo struct {
	State       contracts.ConflictState
	DecisionKey string
	WinnerID    string
	LoserID     string
}
