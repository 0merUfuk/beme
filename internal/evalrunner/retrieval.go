package evalrunner

import (
	"github.com/0merUfuk/beme/internal/resolver"
	"sort"
)

func measureRetrieval(cs Case, refs RefsFn, in resolver.EvalInputs, inErr error) RetrievalCase {
	r := RetrievalCase{CaseID: cs.ID, RefMappings: map[string][]string{}, PrecisionMeaning: "required_reference_precision"}
	if len(cs.Gold.OptionalEvidenceRefs) > 0 {
		r.PrecisionMeaning = "required_or_optional_reference_precision"
	}
	if cs.Gold.RelevanceLabelsComplete {
		r.PrecisionMeaning = "labeled_context_precision"
	}
	if refs == nil {
		r.Outcome, r.Reason = OutcomeNotRun, "no record reference lookup configured"
		return r
	}
	if inErr != nil {
		r.Outcome, r.Reason = OutcomeNotRun, "resolution failed: "+inErr.Error()
		return r
	}
	// Normalize aliases against the actual eligible/delivered record identities.
	// A shared ambiguous alias cannot silently credit two records.
	aliases, ambiguous := map[string]string{}, map[string]bool{}
	items := (ArmContext{Pack: &in.Pack}).Items()
	all := append([]resolver.ContextItem{}, items...)
	for _, c := range in.Eligible.Candidates {
		all = append(all, c.Item)
	}
	for _, it := range all {
		if it.RecordID == "" {
			continue
		}
		keys := append([]string{it.RecordID}, refs(in.Profile, it.RecordID)...)
		seen := map[string]bool{}
		for _, key := range keys {
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			if old := aliases[key]; old != "" && old != it.RecordID {
				ambiguous[key] = true
			}
			aliases[key] = it.RecordID
		}
		r.RefMappings[it.RecordID] = keys
	}
	canonical := func(key string) string {
		if id := aliases[key]; id != "" {
			return id
		}
		return "ref:" + key
	}
	required, relevant, forbidden := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, set := range [][]string{cs.Gold.RequiredEvidenceRefs, cs.Gold.OptionalEvidenceRefs, cs.Gold.ForbiddenEvidenceRefs} {
		for _, key := range set {
			if key == "" || ambiguous[key] {
				r.Outcome, r.Reason = OutcomeNotRun, "empty or ambiguous evidence alias"
				return r
			}
		}
	}
	for _, key := range cs.Gold.RequiredEvidenceRefs {
		required[canonical(key)] = true
		relevant[canonical(key)] = true
	}
	for _, key := range cs.Gold.OptionalEvidenceRefs {
		relevant[canonical(key)] = true
	}
	for _, key := range cs.Gold.ForbiddenEvidenceRefs {
		forbidden[canonical(key)] = true
		if relevant[canonical(key)] {
			r.Outcome, r.Reason = OutcomeNotRun, "contradictory evidence labels"
			return r
		}
	}
	r.Required = len(required)
	r.RecallApplicable = r.Required > 0
	r.NegativeControl = len(relevant) == 0
	selected := map[string]bool{}
	for _, it := range items {
		id := it.RecordID
		if id == "" || selected[id] {
			continue
		}
		selected[id] = true
		r.SelectedIDs = append(r.SelectedIDs, id)
		r.Selected++
		if required[id] {
			r.Retrieved++
		}
		if relevant[id] {
			r.Relevant++
		}
		if forbidden[id] {
			r.Forbidden++
		}
	}
	sort.Strings(r.SelectedIDs)
	r.FalsePositives = r.Selected - r.Relevant
	r.PrecisionApplicable = r.Selected > 0
	r.Recall = ratio(r.Retrieved, r.Required)
	r.Precision = ratio(r.Relevant, r.Selected)
	r.Outcome = OutcomePassed
	if (r.RecallApplicable && r.Recall < RecallThreshold) || (r.PrecisionApplicable && r.Precision < PrecisionThreshold) || r.Forbidden > 0 || (r.NegativeControl && r.Selected > 0) {
		r.Outcome = OutcomeFailed
	}
	if len(in.Pack.Degradations) > 0 || (in.Pack.Resolution.Completeness != "" && in.Pack.Resolution.Completeness != "complete") {
		r.Outcome = OutcomeFailed
		r.Reason = "degraded resolution; retained in denominator"
	}
	return r
}

func (r *RetrievalSummary) aggregate() {
	allPassed := true
	for _, c := range r.Cases {
		if c.Outcome == OutcomeNotRun {
			r.Excluded++
			allPassed = false
			continue
		}
		r.Measured++
		r.Required += c.Required
		r.Retrieved += c.Retrieved
		r.Selected += c.Selected
		r.Relevant += c.Relevant
		if c.NegativeControl {
			r.NegativeControls++
			if c.FalsePositives > 0 {
				r.FalsePositiveCases++
			}
		}
		if c.Outcome != OutcomePassed {
			allPassed = false
		}
	}
	r.Recall = ratio(r.Retrieved, r.Required)
	r.Precision = ratio(r.Relevant, r.Selected)
	r.FalsePositiveRate = ratio(r.FalsePositiveCases, r.NegativeControls)
	r.ThresholdsMet = r.Measured > 0 && allPassed
}
