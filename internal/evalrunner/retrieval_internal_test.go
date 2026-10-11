package evalrunner

import (
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
	"testing"
)

func TestRetrievalThresholdNotRounded(t *testing.T) {
	if ratio(1799, 2000) >= RecallThreshold {
		t.Fatal("rounded sub-threshold recall became a pass")
	}
}

func TestRetrievalOptionalAliasesForbiddenAndDegraded(t *testing.T) {
	cs := Case{ID: "decision.synthetic.v1"}
	cs.Gold.RequiredEvidenceRefs = []string{"alias", "required", "alias"}
	cs.Gold.OptionalEvidenceRefs = []string{"optional"}
	cs.Gold.ForbiddenEvidenceRefs = []string{"forbidden"}
	cs.Gold.RelevanceLabelsComplete = true
	in := resolver.EvalInputs{Pack: resolver.Pack{Constraints: []resolver.ContextItem{{RecordID: "required"}}, Guidance: []resolver.ContextItem{{RecordID: "required"}, {RecordID: "optional"}}}}
	refs := func(_ contracts.Profile, id string) []string {
		if id == "required" {
			return []string{"alias"}
		}
		return nil
	}
	r := measureRetrieval(cs, refs, in, nil)
	if r.Required != 1 || r.Retrieved != 1 || r.Selected != 2 || r.Relevant != 2 || r.Outcome != OutcomePassed || r.PrecisionMeaning != "labeled_context_precision" {
		t.Fatalf("optional/alias denominator: %+v", r)
	}
	in.Pack.LearnedExperimental = []resolver.ContextItem{{RecordID: "forbidden"}}
	r = measureRetrieval(cs, refs, in, nil)
	if r.Forbidden != 1 || r.FalsePositives != 1 || r.Selected != 3 || r.Outcome != OutcomeFailed {
		t.Fatalf("forbidden delivery: %+v", r)
	}
	in.Pack.LearnedExperimental = nil
	in.Pack.Degradations = []resolver.PackDegradation{{Kind: "partial_source", Detail: "synthetic"}}
	r = measureRetrieval(cs, refs, in, nil)
	if r.Outcome != OutcomeFailed || r.Recall != 1 || r.Precision != 1 {
		t.Fatalf("degradation must fail without dropping measured evidence: %+v", r)
	}
	summary := RetrievalSummary{Cases: []RetrievalCase{r}}
	summary.aggregate()
	if summary.Measured != 1 || summary.Excluded != 0 || summary.ThresholdsMet {
		t.Fatalf("degraded denominator: %+v", summary)
	}
}

func TestDeliveredLearnedAndNegativeControls(t *testing.T) {
	cs := Case{ID: "decision.synthetic.v1"}
	cs.Gold.RequiredEvidenceRefs = []string{"learned", "learned"}
	in := resolver.EvalInputs{Pack: resolver.Pack{LearnedExperimental: []resolver.ContextItem{{RecordID: "learned"}, {RecordID: "learned"}}}}
	refs := func(contracts.Profile, string) []string { return nil }
	r := measureRetrieval(cs, refs, in, nil)
	if r.Required != 1 || r.Retrieved != 1 || r.Selected != 1 || r.Recall != 1 {
		t.Fatalf("delivered learned/dedup: %+v", r)
	}
	cs.Gold.RequiredEvidenceRefs = nil
	r = measureRetrieval(cs, refs, in, nil)
	if r.Outcome != OutcomeFailed || r.Selected != 1 {
		t.Fatalf("negative false positive skipped: %+v", r)
	}
	in.Pack.LearnedExperimental = nil
	r = measureRetrieval(cs, refs, in, nil)
	if r.Outcome != OutcomePassed {
		t.Fatalf("empty negative control: %+v", r)
	}
}
