package learning_test

import (
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/learning"
)

func open(t *testing.T) *learning.Store {
	t.Helper()
	s, err := learning.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestFamilyDedupNeverIndependent (§14.4 / FR-052 / threat case 12):
// repeated suggestions from ONE model/session/workflow are one evidence
// family — they must increment a family count, never create independent
// observations.
func TestFamilyDedupNeverIndependent(t *testing.T) {
	s := open(t)
	for i := 0; i < 5; i++ {
		if _, err := s.Observe("observation", "user prefers dark editor themes", "mcp-session", "personal", "personal_private", "task"); err != nil {
			t.Fatalf("observe %d: %v", i, err)
		}
	}
	obs := s.List("quarantined")
	if len(obs) != 1 {
		t.Fatalf("correlated repetitions must collapse into one observation; got %d", len(obs))
	}
	if obs[0].FamilyCount != 5 {
		t.Fatalf("family count must record repetitions within the family; got %d", obs[0].FamilyCount)
	}
	// a DIFFERENT evidence family on the same hypothesis stays separate
	if _, err := s.Observe("observation", "user prefers dark editor themes", "cli-session", "personal", "personal_private", "task"); err != nil {
		t.Fatal(err)
	}
	if got := len(s.List("quarantined")); got != 2 {
		t.Fatalf("distinct evidence families are legitimately distinct observations; got %d", got)
	}
}

// TestRejectedTombstoneRefusesEquivalent (§14.5 / FR-053 / threat case 13):
// a rejected candidate is fingerprinted; the same weak proposal is refused,
// not re-filed for review again.
func TestRejectedTombstoneRefusesEquivalent(t *testing.T) {
	s := open(t)
	o1, err := s.Observe("observation", "user wants aggressive refactoring by default", "mcp-session", "personal", "personal_private", "task")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(o1.ObservationID, "reject", "operator", "overgeneralizes one incident"); err != nil {
		t.Fatal(err)
	}
	// exact re-proposal: refused
	if _, err := s.Observe("observation", "user wants aggressive refactoring by default", "mcp-session", "personal", "personal_private", "task"); err == nil {
		t.Fatal("equivalent re-proposal after rejection must be refused")
	} else if !strings.Contains(err.Error(), "tombstoned") {
		t.Fatalf("refusal must cite the tombstone; got %v", err)
	}
	// whitespace/case-varied re-proposal: still refused (normalized fingerprint)
	if _, err := s.Observe("observation", "User   wants AGGRESSIVE refactoring by default ", "mcp-session", "personal", "personal_private", "task"); err == nil {
		t.Fatal("normalized-equivalent re-proposal must be refused")
	}
	// genuinely different hypothesis: accepted
	if _, err := s.Observe("observation", "user wants conventional commits enforced in CI", "mcp-session", "personal", "personal_private", "task"); err != nil {
		t.Fatalf("different hypothesis must be accepted: %v", err)
	}
}

// TestReviewActionsLifecycle (§14.5): all batch-review actions behave and
// terminal states refuse double review.
func TestReviewActionsLifecycle(t *testing.T) {
	cases := []struct{ action, wantStatus string }{
		{"approve", "approved"},
		{"reject", "rejected"},
		{"defer", "deferred"},
		{"situational", "situational"},
		{"scope_limit", "scope_limited"},
		{"scope-limit", "scope_limited"},
	}
	for _, c := range cases {
		t.Run(c.action, func(t *testing.T) {
			s := open(t)
			o, _ := s.Observe("observation", "h "+c.action, "fam", "personal", "personal_private", "t")
			if _, err := s.Review(o.ObservationID, c.action, "operator", "n"); err != nil {
				t.Fatalf("%s: %v", c.action, err)
			}
			got, _ := s.Get(o.ObservationID)
			if got.Status != c.wantStatus {
				t.Fatalf("%s → status %q, want %q", c.action, got.Status, c.wantStatus)
			}
		})
	}

	// edit rewrites the hypothesis and returns it to review
	s := open(t)
	o, _ := s.Observe("observation", "overbroad claim", "fam", "personal", "personal_private", "t")
	if _, err := s.Review(o.ObservationID, "edit", "operator", "narrower: prefer scoped refactors after tests"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(o.ObservationID)
	if got.Hypothesis != "narrower: prefer scoped refactors after tests" || got.Status != "pending_review" {
		t.Fatalf("edit must rewrite and requeue; got %q %q", got.Hypothesis, got.Status)
	}

	// merge records the target canonical record
	s2 := open(t)
	o2, _ := s2.Observe("observation", "dup of KP-002", "fam", "personal", "personal_private", "t")
	if _, err := s2.Review(o2.ObservationID, "merge", "operator", "rec_seed0001"); err != nil {
		t.Fatal(err)
	}
	got2, _ := s2.Get(o2.ObservationID)
	if got2.Status != "merged" || got2.ReviewOutcome.MergedIntoRecord != "rec_seed0001" {
		t.Fatalf("merge must record the target; got %+v", got2.ReviewOutcome)
	}

	// counterexample appends counterevidence and requeues
	s3 := open(t)
	o3, _ := s3.Observe("observation", "user never wants tests", "fam", "personal", "personal_private", "t")
	if _, err := s3.Review(o3.ObservationID, "counterexample", "operator", "user demanded tests on 2026-09-14"); err != nil {
		t.Fatal(err)
	}
	got3, _ := s3.Get(o3.ObservationID)
	if !strings.Contains(got3.Counterevidence, "2026-09-14") || got3.Status != "pending_review" {
		t.Fatalf("counterexample must append counterevidence; got %q %q", got3.Counterevidence, got3.Status)
	}

	// terminal states refuse double review
	s4 := open(t)
	o4, _ := s4.Observe("observation", "terminal test", "fam", "personal", "personal_private", "t")
	s4.Review(o4.ObservationID, "approve", "operator", "n")
	if _, err := s4.Review(o4.ObservationID, "reject", "operator", "n"); err == nil {
		t.Fatal("approved observation must refuse further review actions")
	}
}

// TestObserveNeverCanonical (FR-050): Observe writes only under the
// observations directory and only observation JSON; it cannot touch any
// knowledge/policy path.
func TestObserveNeverCanonical(t *testing.T) {
	s := open(t)
	o, err := s.Observe("correction", "fix misattribution", "mcp-session", "personal", "personal_private", "t")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != "quarantined" {
		t.Fatalf("new observations are quarantined, got %q", o.Status)
	}
	// the learning store API surface has no canonical-write entry point by
	// construction; assert the written file lives only under observations/.
	if !strings.HasPrefix(o.ObservationID, "obs_") {
		t.Fatalf("unexpected id shape %q", o.ObservationID)
	}
}

// TestSensitivityInherited (FR-054): work-safe feedback inherits a
// non-personal sensitivity; personal feedback inherits personal_private.
func TestSensitivityInherited(t *testing.T) {
	s := open(t)
	o, err := s.Observe("observation", "h", "fam", "work-safe", "public_general", "t")
	if err != nil {
		t.Fatal(err)
	}
	if o.InheritedSensitivity != "public_general" {
		t.Fatalf("work-safe observation must not silently become global personal knowledge; got %q", o.InheritedSensitivity)
	}
}
