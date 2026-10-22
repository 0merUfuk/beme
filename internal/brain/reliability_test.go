package brain

import (
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Regression coverage for independently reproduced V2 reliability defects.
// Fixtures are synthetic and local; no host deployment or network access.
func TestReliabilityScalarRevisionMustReplacePriorDecision(t *testing.T) {
	v, root := newVault(t)
	in := CaptureInput{Type: TypeDecision, Title: "Gateway retry budget", Topic: "retry-budget", Statement: "The gateway retries failed upstream requests at most 3 times.", Why: "Measured tail latency sets the retry budget."}
	old := mustCapture(t, v, in, t0)
	mustDoctor(t, v, t0)
	in.Statement = "The gateway retries failed upstream requests at most 5 times."
	newest := mustCapture(t, v, in, t0.Add(time.Hour))
	rep := mustDoctor(t, v, t0.Add(time.Hour))
	live, _ := v.List("demo", TypeDecision, false)
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0.Add(time.Hour)})
	t.Logf("old=%s new=%s actions=%+v live=%d text_contains_new_scalar=%t", old.ID, newest.ID, rep.Actions, len(live), strings.Contains(text, "most 5 times"))
	if len(live) != 1 || live[0].ID != newest.ID || !strings.Contains(text, "most 5 times") {
		t.Fatalf("new scalar decision was silently discarded; retained=%q confirmed=%d", live[0].Statement, live[0].Confirmed)
	}
}

func TestReliabilityBatchPromotionBriefsNewestSummary(t *testing.T) {
	v, root := newVault(t)
	old := mustCapture(t, v, CaptureInput{Type: TypeSummary, Title: "Alpha initial checkpoint", Statement: "The retry implementation is incomplete and staging still fails."}, t0)
	newest := mustCapture(t, v, CaptureInput{Type: TypeSummary, Title: "Zulu final checkpoint", Statement: "The retry implementation is complete and staging now passes."}, t0.Add(time.Hour))
	mustDoctor(t, v, t0.Add(2*time.Hour))
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0.Add(2 * time.Hour)})
	t.Logf("old=%s new=%s old_shown=%t new_shown=%t", old.ID, newest.ID, strings.Contains(text, old.Statement), strings.Contains(text, newest.Statement))
	if !strings.Contains(text, newest.Statement) || strings.Contains(text, old.Statement) {
		t.Fatal("one maintenance timestamp makes briefing select stale summary instead of latest Created")
	}
}

func TestReliabilitySameSecondTopicUpdatePreservesOrder(t *testing.T) {
	v, _ := newVault(t)
	old := mustCapture(t, v, CaptureInput{Type: TypeDecision, Title: "Zulu initial database", Topic: "storage", Statement: "Local state lives in SQLite for offline installations.", Why: "Offline use avoids operational complexity."}, t0.Add(100*time.Millisecond))
	newest := mustCapture(t, v, CaptureInput{Type: TypeDecision, Title: "Alpha replacement database", Topic: "storage", Statement: "Remote transactional workloads require a Postgres database.", Why: "Concurrent writers require network storage."}, t0.Add(200*time.Millisecond))
	rep := mustDoctor(t, v, t0.Add(time.Hour))
	live, _ := v.List("demo", TypeDecision, false)
	t.Logf("old_created=%s new_created=%s actions=%+v final=%s", old.Created, newest.Created, rep.Actions, live[0].Title)
	if len(live) != 1 || live[0].ID != newest.ID {
		t.Fatal("filename sorting reversed successive captures in the same second")
	}
}

func TestReliabilityRestoreOldestSnapshotDoesNotDeleteTarget(t *testing.T) {
	v, _ := newVault(t)
	n := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	for i := 1; i <= backupKeep; i++ {
		if _, err := v.Backup("retention", t0.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	snaps, err := v.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	target := snaps[len(snaps)-1].Path
	err = v.Restore(target, true, t0.Add(time.Hour))
	_, findErr := v.Find(n.ID)
	t.Logf("snapshots_before=%d target_exists_after=%t initialized_after=%t restore_error=%v find_error=%v", len(snaps), fileExists(target), v.Initialized(), err, findErr)
	if err != nil || findErr != nil {
		t.Fatal("restoring a listed valid snapshot pruned its own input then removed the live vault")
	}
}

func TestReliabilityRefusedArchiveLeavesLiveVaultIntact(t *testing.T) {
	v, _ := newVault(t)
	n := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	bad := craftArchive(t, tar.Header{Name: "../outside.md", Typeflag: tar.TypeReg, Size: 2, Mode: 0600}, "hi")
	err := v.Restore(bad, true, t0.Add(time.Hour))
	_, findErr := v.Find(n.ID)
	t.Logf("restore_error=%v initialized_after=%t find_error=%v", err, v.Initialized(), findErr)
	if err == nil || findErr != nil {
		t.Fatal("rejected archive destroyed live target before validation finished")
	}
}

func TestReliabilitySupersessionFailureIsReportedAndRetried(t *testing.T) {
	v, _ := newVault(t)
	old := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	// Deterministic disk-error fixture: destination is a directory, so rename fails.
	if err := os.MkdirAll(v.archivePath(old), 0700); err != nil {
		t.Fatal(err)
	}
	newer := mustCapture(t, v, CaptureInput{Type: TypeDecision, Title: "Move state to Postgres", Topic: "storage", Statement: "Shared deployments need concurrent writers, so state moves to Postgres.", Why: "SQLite single-writer limit hit in staging."}, t0.Add(time.Hour))
	rep := mustDoctor(t, v, t0.Add(time.Hour))
	live, _ := v.List("demo", TypeDecision, false)
	t.Logf("new=%s report_live=%d actual_live=%d actions=%+v flags=%+v", newer.ID, rep.Live["demo"], len(live), rep.Actions, rep.Flags)
	if has(rep, "superseded", old.ID) && len(live) == 2 && len(rep.Flags) == 0 {
		t.Error("doctor claims a successful retirement without doing it or reporting its error")
	}
	if err := os.Remove(v.archivePath(old)); err != nil {
		t.Fatal(err)
	}
	rep2 := mustDoctor(t, v, t0.Add(2*time.Hour))
	live2, _ := v.List("demo", TypeDecision, false)
	t.Logf("retry_live=%d retry_actions=%+v retry_flags=%+v", len(live2), rep2.Actions, rep2.Flags)
	if len(live2) != 1 {
		t.Error("removing transient failure does not recover the incomplete supersession")
	}
}

func TestReliabilityFakeWorktreeMustNotAcquireProjectMemory(t *testing.T) {
	v, root := newVault(t)
	mustCapture(t, v, CaptureInput{Type: TypeLearning, Title: "Project alpha sentinel", Statement: "SYNTHETIC_ALPHA_ONLY evidence belongs to the registered project."}, t0)
	mustDoctor(t, v, t0)
	foreign := t.TempDir()
	fake := filepath.Join(root, ".git", "worktrees", "never-registered")
	if err := os.WriteFile(filepath.Join(foreign, ".git"), []byte("gitdir: "+fake+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	match := v.ResolveProject(foreign)
	text, _ := v.Briefing(BriefingOptions{CWD: foreign, Now: t0})
	t.Logf("claimed_gitdir_exists=%t match_slug=%q leaked_synthetic_sentinel=%t", fileExists(fake), match.Slug, strings.Contains(text, "SYNTHETIC_ALPHA_ONLY"))
	if match.Slug != "" || strings.Contains(text, "SYNTHETIC_ALPHA_ONLY") {
		t.Fatal("unverified gitdir string grants registered-project memory to unrelated directory")
	}
}

func TestReliabilityIdentitySymlinkMustBeWithheld(t *testing.T) {
	v, root := newVault(t)
	outside := filepath.Join(t.TempDir(), "outside-role.md")
	if err := os.WriteFile(outside, []byte("Role: SYNTHETIC_OUTSIDE_IDENTITY content stored outside the vault.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	id := filepath.Join(v.Root, "identity.md")
	if err := os.Remove(id); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, id); err != nil {
		t.Fatal(err)
	}
	rep := mustDoctor(t, v, t0)
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0, Report: rep})
	t.Logf("foreign_file_flag=%t outside_text_injected=%t", hasFlag(rep, "foreign_file"), strings.Contains(text, "SYNTHETIC_OUTSIDE_IDENTITY"))
	if strings.Contains(text, "SYNTHETIC_OUTSIDE_IDENTITY") {
		t.Fatal("identity bypasses the symlink rule even after doctor flags it")
	}
}

func TestReliabilityFrozenRegistrationAndSessionWritesRefused(t *testing.T) {
	v, _ := newVault(t)
	if err := v.Freeze(true, t0); err != nil {
		t.Fatal(err)
	}
	_, err := v.AddProject("other", "Other", t.TempDir())
	v.RecordSession("demo", t0)
	t.Logf("add_project_error=%v created_registration=%t state_written=%t", err, fileExists(v.projectFile("other")), fileExists(v.stateFile("demo")))
	if err == nil || fileExists(v.stateFile("demo")) {
		t.Fatal("freeze does not lock every writer")
	}
}

func TestReliabilityReadOnlyBriefingStillShowsConflict(t *testing.T) {
	v, root := newVault(t)
	old := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	n, err := v.Find(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	n.Confidence = ConfValidated
	if err := v.writeNote(n.Path, n); err != nil {
		t.Fatal(err)
	}
	mustCapture(t, v, CaptureInput{Type: TypeDecision, Title: "Move state to Postgres", Topic: "storage", Statement: "Shared deployments need concurrent writers, so state moves to Postgres.", Why: "SQLite single-writer limit hit in staging."}, t0.Add(time.Hour))
	rep := mustDoctor(t, v, t0.Add(time.Hour))
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0.Add(time.Hour)})
	t.Logf("doctor_conflict=%t briefing_conflict=%t both_decisions_present=%t", hasFlag(rep, "conflict"), strings.Contains(text, "conflict"), strings.Contains(text, "Use SQLite") && strings.Contains(text, "Move state"))
	if !strings.Contains(text, "conflict") {
		t.Fatal("default read-only briefing omits known equal-topic conflict")
	}
}

func TestReliabilityCaptureIDMustNotOverwriteDistinctProposal(t *testing.T) {
	v, _ := newVault(t)
	in := CaptureInput{Type: TypeSummary, Project: "demo", Title: "Daily project checkpoint", Statement: "First distinct result is safely recorded in the proposed summary."}
	seen := map[string]time.Time{}
	var a, b time.Time
	// Generate IDs only until a birthday collision (fixed upper limit); then execute two captures.
	for i := 0; i < 20000; i++ {
		at := t0.Add(time.Duration(i) * time.Nanosecond)
		id := newID(&Note{Type: in.Type, Project: in.Project, Title: in.Title}, at)
		if prev, exists := seen[id]; exists {
			a, b = prev, at
			break
		}
		seen[id] = at
	}
	if a.IsZero() {
		t.Log("no candidate collision in the bounded sample; extended IDs pass this regression")
		return
	}
	n1 := mustCapture(t, v, in, a)
	in.Statement = "Second distinct result replaces the first one although both captures report success."
	n2 := mustCapture(t, v, in, b)
	entries, _ := os.ReadDir(v.inboxDir())
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			count++
		}
	}
	t.Logf("id1=%s id2=%s accepted=2 inbox_files=%d generated_before_collision=%d", n1.ID, n2.ID, count, len(seen)+1)
	if count != 2 {
		t.Fatal("successful Capture overwrites an existing unrelated proposal on 16-bit suffix collision")
	}
}

func TestReliabilityEconomyBudgetRetainsConflictWarning(t *testing.T) {
	v, root := newVault(t)
	for i := 0; i < 5; i++ {
		putLive(t, v, &Note{ID: fmt.Sprintf("opn-20260919-fixture-%03d", i), Type: TypeOpen, Project: "demo", Title: strings.Repeat("ordinary open work ", 5), Status: StatusOpen, Created: t0.Format(time.RFC3339), Statement: strings.Repeat("Pending verification work remains. ", 4), Next: strings.Repeat("Inspect regression results. ", 3)})
	}
	for i := 0; i < 6; i++ {
		putLive(t, v, &Note{ID: fmt.Sprintf("dec-20260919-budget-%03d", i), Type: TypeDecision, Project: "demo", Title: strings.Repeat("Recorded database constraint ", 3), Status: StatusActive, Confidence: ConfValidated, Topic: "storage", Created: t0.Format(time.RFC3339), Statement: fmt.Sprintf("Database constraint variant %d applies to transactional services under review.", i), Why: "The measured service requirements determine this constraint."})
	}
	rep := mustDoctor(t, v, t0)
	if !hasFlag(rep, "conflict") {
		t.Fatal("fixture must contain a real conflict")
	}
	text, meta := v.Briefing(BriefingOptions{CWD: root, Now: t0, Mode: ModeEconomy, Report: rep})
	t.Logf("bytes=%d truncated=%t conflict_visible=%t", len(text), meta.Truncated, strings.Contains(text, "conflict"))
	if !strings.Contains(text, "conflict") {
		t.Fatal("routine open-work consumes budget before conflict warning")
	}
}
