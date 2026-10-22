package brain

import (
	"strings"
	"testing"
	"time"
)

func TestExactDuplicateEquivalencePreservesMaterialChanges(t *testing.T) {
	base := &Note{Type: TypeDecision, Project: "demo", Title: "Gateway policy", Topic: "gateway", Statement: "Service A sends API v3 requests to B at most 3 times.", Why: "The latency budget is 50 ms.", Next: "Verify region A."}
	for name, change := range map[string]func(*Note){
		"scalar":                    func(n *Note) { n.Statement = strings.ReplaceAll(n.Statement, "3 times", "5 times") },
		"identifier":                func(n *Note) { n.Statement = strings.ReplaceAll(n.Statement, "v3", "v4") },
		"case-sensitive-identifier": func(n *Note) { n.Statement = strings.ReplaceAll(n.Statement, "API", "api") },
		"subject-object":            func(n *Note) { n.Statement = "Service B sends API v3 requests to A at most 3 times." },
		"rationale":                 func(n *Note) { n.Why = "The latency budget is 80 ms." },
		"next":                      func(n *Note) { n.Next = "Verify region B." },
		"topic":                     func(n *Note) { n.Topic = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			other := *base
			change(&other)
			if sameClaim(base, &other) {
				t.Fatal("material change was considered an exact recapture")
			}
		})
	}
	other := *base
	other.Statement = "  " + base.Statement + "\n"
	if !sameClaim(base, &other) {
		t.Fatal("surrounding whitespace does not change the claim")
	}
}

func TestDelayedOlderDecisionNeverReplacesNewerLiveDecision(t *testing.T) {
	v, _ := newVault(t)
	in := sqliteDecision
	newest := mustCapture(t, v, in, t0.Add(time.Hour))
	mustDoctor(t, v, t0.Add(time.Hour))
	in.Statement = "Shared state lives in Postgres for concurrent writers."
	older := mustCapture(t, v, in, t0)
	mustDoctor(t, v, t0.Add(2*time.Hour))
	live, _ := v.List("demo", TypeDecision, false)
	found := false
	for _, n := range live {
		if n.ID == newest.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("late processing of %s displaced newer %s", older.ID, newest.ID)
	}
}

func TestFractionalAndOffsetChronologyUsesInstants(t *testing.T) {
	a := &Note{Created: "2026-09-19T11:00:00+01:00"}
	b := &Note{Created: "2026-09-19T10:00:00.2Z"}
	v, root := newVault(t)
	for i, n := range []*Note{a, b} {
		n.ID = []string{"sum-20260919-older-001", "sum-20260919-newer-001"}[i]
		n.Type = TypeSummary
		n.Project = "demo"
		n.Title = "Checkpoint"
		n.Statement = []string{"The old checkpoint still has failing checks.", "The latest checkpoint has all checks passing."}[i]
		n.Status = StatusActive
		putLive(t, v, n)
	}
	s, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0.Add(time.Hour)})
	if !strings.Contains(s, b.Statement) {
		t.Fatal("offset lexicographic ordering is not time ordering")
	}
}
