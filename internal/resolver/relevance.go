package resolver

// Stage B retrieval (ARCHITECTURE §2): eligible records are retrieved for the
// task, not all emitted. Mandatory constraints and globally applicable
// principles always apply; advisory records (preferences, heuristics,
// patterns, workflows, failure modes, facts, precedents, observations) enter
// a pack only when the task gives evidence they apply. When no advisory
// evidence applies, the pack says so through an unknown (FR-034) instead of
// presenting unrelated guidance as if it were context for the task.

import (
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
)

// isMandatory reports records that are constraints by source role or by
// authority: they are never filtered by relevance (FR-035 reserves them).
func isMandatory(rec contracts.Record) bool {
	if rec.SourceRole == contracts.RoleTrustedProjectPolicy {
		return true
	}
	return rec.Authority == contracts.AuthorityDefault &&
		(rec.Kind == contracts.KindDirective || rec.Kind == contracts.KindPrinciple) &&
		rec.Criticality == "high"
}

// isGlobalPrinciple reports records that apply to every task in their
// eligible scope: foundation-role records, and principles that carry no task,
// workspace, path or technology restriction.
func isGlobalPrinciple(rec contracts.Record) bool {
	if rec.SourceRole == contracts.RoleCanonicalFoundation {
		return true
	}
	return rec.Kind == contracts.KindPrinciple &&
		len(rec.Scope.TaskKinds) == 0 && len(rec.Scope.WorkspaceIDs) == 0 &&
		len(rec.Scope.PathGlobs) == 0 && len(rec.Scope.Technologies) == 0
}

// alwaysApplies reports records retrieval never drops.
func alwaysApplies(rec contracts.Record) bool {
	return isMandatory(rec) || isGlobalPrinciple(rec)
}

// relevantToTask reports whether the task gives evidence an advisory record
// applies: a task or workspace scope (Stage A's task_scope and
// workspace_scope checks already proved it matches this task), a matching
// facet, a named technology, or shared content vocabulary.
func relevantToTask(rec contracts.Record, taskTokens map[string]bool, facets []string, tc policy.TaskContext) bool {
	if len(rec.Scope.TaskKinds) > 0 || len(rec.Scope.WorkspaceIDs) > 0 {
		return true
	}
	for _, tech := range rec.Scope.Technologies {
		for tok := range contentTokens(tech) {
			if taskTokens[tok] {
				return true
			}
		}
	}
	return contentOverlap(taskTokens, recordText(rec)) > 0
}

// retainRelevant drops advisory records the task gives no evidence for.
// Records sharing a decision_key with a relevant record are kept: they are
// alternatives for the same decision, so precedence and conflict reporting
// must see them.
func retainRelevant(scored []scoredRecord, req contracts.ResolutionRequest, facets []string, tc policy.TaskContext) (kept []scoredRecord, excluded []string) {
	taskTokens := contentTokens(req.Task)
	relevant := make([]bool, len(scored))
	relevantKeys := map[string]bool{}
	for i, sr := range scored {
		if alwaysApplies(sr.rec) || relevantToTask(sr.rec, taskTokens, facets, tc) {
			relevant[i] = true
			if sr.rec.DecisionKey != "" {
				relevantKeys[sr.rec.DecisionKey] = true
			}
		}
	}
	for i, sr := range scored {
		if relevant[i] || (sr.rec.DecisionKey != "" && relevantKeys[sr.rec.DecisionKey]) {
			kept = append(kept, sr)
			continue
		}
		excluded = append(excluded, sr.rec.RecordID)
	}
	return kept, excluded
}

// hasAdvisoryEvidence reports whether any retained record is task evidence
// rather than an always-applicable constraint or principle.
func hasAdvisoryEvidence(winners []scoredRecord) bool {
	for _, w := range winners {
		if !alwaysApplies(w.rec) {
			return true
		}
	}
	return false
}

func recordText(rec contracts.Record) string {
	return rec.Title + " " + rec.Statement + " " + rec.CompactText + " " + rec.Key + " " + rec.DecisionKey
}

// contentOverlap counts the distinct content tokens a text shares with the
// task.
func contentOverlap(taskTokens map[string]bool, text string) int {
	n := 0
	for tok := range contentTokens(text) {
		if taskTokens[tok] {
			n++
		}
	}
	return n
}

// contentTokens splits text into normalized content words: lower-cased,
// function words and generic request words removed, and a light stem so
// inflections of one word match (tests/testing/tested, image/images,
// consistent/consistency). Matching is on whole normalized words, never on
// substrings, so short or common words cannot create relevance.
func contentTokens(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicodeIsWord(r) }) {
		if len([]rune(w)) < 3 || stopwords[w] {
			continue
		}
		out[stem(w)] = true
	}
	return out
}

func stem(w string) string {
	for _, suffix := range []string{"ing", "ed", "es", "s"} {
		if len(w) > len(suffix)+3 && strings.HasSuffix(w, suffix) {
			w = strings.TrimSuffix(w, suffix)
			break
		}
	}
	if r := []rune(w); len(r) > 6 {
		return string(r[:6])
	}
	return w
}

// stopwords are English function words and generic request vocabulary that
// appear in almost any task and carry no topic.
var stopwords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`
		a an and are as at be been being but by can could did do does doing done for from had has have having
		he her here hers him his how i if in into is it its itself just let lets me more most my no nor not of off
		on once only or other our ours out over own same she should so some such than that the their theirs them
		then there these they this those through to too under until up very was we were what when where which while
		who whom why will with would you your yours about above after again against all also am any because before
		below between both down during each few further via per etc
		use uses using used make makes making made need needs want wants choose choosing pick decide deciding decision
		best better good way ways approach option options help please new thing things something get gets set
		one two able like just now currently today question task work working next best
	`) {
		m[w] = true
	}
	return m
}()
