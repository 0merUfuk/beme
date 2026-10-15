package brain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFinalOldGlobalOpenNeverBroadcastsIDs(t *testing.T) {
	v, root := newVault(t)
	n := &Note{ID: "opn-20260101-secret-project-launch-1111", Type: TypeOpen, Project: GlobalProject, Title: "Private project launch", Statement: "SYNTHETIC_OLD_GLOBAL_SECRET needs a project scope instead of global.", Status: StatusOpen, Created: t0.Add(-31 * 24 * time.Hour).Format(time.RFC3339Nano)}
	putLive(t, v, n)
	for _, mode := range []Mode{ModeNormal, ModeEconomy, ModeManual} {
		text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0, Mode: mode})
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, n.ID) {
				t.Logf("mode=%s leaked_line=%s", mode, line)
			}
		}
		if strings.Contains(text, n.ID) || strings.Contains(text, n.Statement) {
			t.Errorf("mode=%s unsupported global project-state metadata was broadcast", mode)
		}
	}
	rep := mustDoctor(t, v, t0)
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0, Report: rep})
	if strings.Contains(text, n.ID) {
		t.Error("maintenance report also broadcasts old-global ID")
	}
	if _, err := v.Find(n.ID); err != nil {
		t.Fatal("old global note was not preserved", err)
	}
}

func TestFinalDelayedRevisionCannotEraseLaterExactRecapture(t *testing.T) {
	v, root := newVault(t)
	in := CaptureInput{Type: TypeDecision, Title: "Gateway retry policy", Topic: "retry-policy", Statement: "The gateway retries the upstream request at most 3 times.", Why: "The measured service latency determines the retry budget."}
	a := mustCapture(t, v, in, t0)
	mustDoctor(t, v, t0)
	mustCapture(t, v, in, t0.Add(2*time.Hour))
	mustDoctor(t, v, t0.Add(2*time.Hour))
	before, err := v.Find(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	in.Statement = "The gateway retries the upstream request at most 5 times."
	b := mustCapture(t, v, in, t0.Add(time.Hour))
	rep := mustDoctor(t, v, t0.Add(3*time.Hour))
	after, findErr := v.Find(a.ID)
	_ = after
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0.Add(3 * time.Hour)})
	t.Logf("latest_A_created=%s latest_A_recaptured=%s delayed_B_created=%s A_still_live=%t actions=%+v", before.Created, before.Updated, b.Created, findErr == nil, rep.Actions)
	if findErr != nil || !strings.Contains(text, "most 3 times") {
		t.Error("revision older than the most recent exact recapture erased the latest observed decision")
	}
}

func TestFinalBoundedBacklogKeepsLatestRecapture(t *testing.T) {
	v, _ := newVault(t)
	aIn := CaptureInput{Type: TypeDecision, Title: "Alpha gateway retry policy", Topic: "retry-policy", Statement: "The gateway retries the upstream request at most 3 times.", Why: "The measured latency determines this retry policy."}
	a := mustCapture(t, v, aIn, t0)
	mustDoctor(t, v, t0)
	bIn := aIn
	bIn.Title = "Zulu gateway revision"
	bIn.Statement = "The gateway retries the upstream request at most 5 times."
	b := mustCapture(t, v, bIn, t0.Add(time.Hour))
	for i := 0; i < 127; i++ {
		mustCapture(t, v, CaptureInput{Type: TypeDecision, Title: fmt.Sprintf("Beta filler policy %03d", i), Topic: fmt.Sprintf("filler-%d", i), Statement: fmt.Sprintf("Synthetic fixture lane %d uses an isolated bounded queue.", i), Why: "Independent queues isolate ordinary test work."}, t0.Add(90*time.Minute))
	}
	recap := mustCapture(t, v, aIn, t0.Add(2*time.Hour))
	first := mustDoctor(t, v, t0.Add(3*time.Hour))
	if !has(first, "merged", recap.ID) || !fileExists(b.Path) {
		t.Fatalf("fixture did not delay older revision by the 128-proposal page: %+v", first)
	}
	before, err := v.Find(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	second := mustDoctor(t, v, t0.Add(4*time.Hour))
	_, findErr := v.Find(a.ID)
	t.Logf("older_revision_pending_after_first=true first_page=%d recapture_at=%s older_revision_at=%s second_actions=%+v latest_decision_live=%t", first.Inbox, before.Updated, b.Created, second.Actions, findErr == nil)
	if findErr != nil {
		t.Error("ordinary bounded queue order lets older revision erase a later exact recapture")
	}
}

func TestFinalEqualInstantChangesPreservedAsConflict(t *testing.T) {
	for _, body := range []string{"The gateway retries the upstream request at most 5 times.", "The gateway never retries the upstream request at most 3 times."} {
		v, _ := newVault(t)
		in := CaptureInput{Type: TypeDecision, Title: "Gateway retry policy", Topic: "retry-policy", Statement: "The gateway retries the upstream request at most 3 times.", Why: "The service budget determines this policy."}
		a := mustCapture(t, v, in, t0)
		in.Statement = body
		b := mustCapture(t, v, in, t0)
		rep := mustDoctor(t, v, t0)
		live, err := v.List("demo", TypeDecision, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(live) != 2 || !hasFlag(rep, "conflict") || has(rep, "merged", a.ID) || has(rep, "merged", b.ID) {
			t.Fatalf("equal-time change was decided arbitrarily: %+v", rep)
		}
	}
}

func TestFinalPartialNewSchemaInitMustNotWrite(t *testing.T) {
	v := Open(filepath.Join(t.TempDir(), "new-vault"))
	if err := os.MkdirAll(v.Root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v.Root, "brain.yaml"), []byte("schema_version: 99\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := v.Init()
	t.Logf("init_error=%v projects_created=%t", err, fileExists(v.projectsDir()))
	if err == nil || fileExists(v.projectsDir()) {
		t.Error("Init wrote an explicitly newer schema when projects skeleton was absent")
	}
}

func TestFinalUnreadableSchemaCannotAuthorizeWrites(t *testing.T) {
	for _, kind := range []string{"symlink", "malformed", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			v, _ := newVault(t)
			marker := filepath.Join(v.Root, "brain.yaml")
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink":
				outside := filepath.Join(t.TempDir(), "v99.yaml")
				os.WriteFile(outside, []byte("schema_version: 99\n"), 0600)
				if err := os.Symlink(outside, marker); err != nil {
					t.Fatal(err)
				}
			case "malformed":
				os.WriteFile(marker, []byte("schema_version: [99\n"), 0600)
			case "oversized":
				os.WriteFile(marker, []byte("schema_version: 99\n#"+strings.Repeat("x", maxDocumentBytes)), 0600)
			}
			_, err := v.AddProject("other", "Other", t.TempDir())
			t.Logf("schema=%d add_error=%v wrote_registration=%t", v.schemaVersion(), err, fileExists(v.projectFile("other")))
			if err == nil || fileExists(v.projectFile("other")) {
				t.Error("unreadable/invalid schema defaulted to writable current format")
			}
		})
	}
}

func TestFinalSessionWindowAndOpaqueIdentity(t *testing.T) {
	v, _ := newVault(t)
	for i := 0; i < sessionIDWindow; i++ {
		if err := v.RecordSessionEvent("demo", "synthetic", fmt.Sprintf("opaque-session-%d", i), t0); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.RecordSessionEvent("demo", "synthetic", "opaque-session-0", t0); err != nil {
		t.Fatal(err)
	}
	st := v.loadState("demo")
	if st.SessionsSinceCapture != 512 || len(st.SeenSessions) != 512 || st.InvocationsSinceCapture != 513 {
		t.Fatalf("window before eviction: %+v", st)
	}
	if err := v.RecordSessionEvent("demo", "synthetic", "opaque-session-512", t0); err != nil {
		t.Fatal(err)
	}
	if err := v.RecordSessionEvent("demo", "synthetic", "opaque-session-0", t0); err != nil {
		t.Fatal(err)
	}
	st = v.loadState("demo")
	data, err := os.ReadFile(v.stateFile("demo"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("window=%d identified_count=%d invocation_count=%d persisted_bytes=%d evicted_ID_counts_again=true", len(st.SeenSessions), st.SessionsSinceCapture, st.InvocationsSinceCapture, len(data))
	if st.SessionsSinceCapture != 514 || len(st.SeenSessions) != 512 || st.InvocationsSinceCapture != 515 || strings.Contains(string(data), "opaque-session") {
		t.Fatal("dedup retention/opaque-state invariant failed")
	}
	if v.Status().SessionWindow != 512 || strings.Contains(v.Status().Accounting, "measured benefit") {
		t.Fatal("public semantics misleading")
	}
}

func TestFinalMergeRepeatedIDsStayReadable(t *testing.T) {
	v, root := newVault(t)
	in := CaptureInput{Type: TypeDecision, Title: "The repeated durable decision title", Topic: "stable-policy", Statement: "The client uses a bounded queue for each synthetic work item.", Why: "Bounded queues preserve predictable resource usage."}
	first := mustCapture(t, v, in, t0)
	mustDoctor(t, v, t0)
	// Construct an otherwise valid pre-existing note just below the bounded reader.
	// Each applied ID has the exact shape/length generated by Capture. This is the
	// durable state produced by repeated exact recapture; the last transition is
	// exercised through the real Capture->Doctor->Briefing public path.
	live, err := v.Find(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; ; i++ {
		id := fmt.Sprintf("dec-20260919-%s-%032x", slugify(in.Title, 36), i)
		live.RecapturedBy = append(live.RecapturedBy, id)
		live.Confirmed++
		data, _ := live.Marshal()
		if len(data) > maxDocumentBytes {
			live.RecapturedBy = live.RecapturedBy[:len(live.RecapturedBy)-1]
			live.Confirmed--
			break
		}
	}
	if err := v.writeNote(live.Path, live); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(live.Path)
	newest := mustCapture(t, v, in, t0.Add(time.Hour))
	rep := mustDoctor(t, v, t0.Add(time.Hour))
	after, _ := os.Stat(live.Path)
	_, findErr := v.Find(first.ID)
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0.Add(time.Hour)})
	t.Logf("applied_ids_before=%d bytes_before=%d bytes_after=%d reader_limit=%d merged=%t find_error=%v", len(live.RecapturedBy), before.Size(), after.Size(), maxDocumentBytes, has(rep, "merged", newest.ID), findErr)
	if findErr != nil || !strings.Contains(text, in.Statement) {
		t.Error("successful exact recapture made the live decision unreadable and removed it from briefing")
	}
}

func TestFinalRepeatedRecapturesPublicLifecycleReadable(t *testing.T) {
	v, _ := newVault(t)
	in := CaptureInput{Type: TypeDecision, Title: "The repeated durable decision title", Topic: "stable-policy", Statement: "The client uses a bounded queue for each synthetic work item.", Why: "Bounded queues preserve predictable resource usage."}
	first := mustCapture(t, v, in, t0)
	mustDoctor(t, v, t0)
	for i := 1; i <= 800; i++ {
		at := t0.Add(time.Duration(i) * time.Minute)
		proposal := mustCapture(t, v, in, at)
		rep := mustDoctor(t, v, at)
		if !has(rep, "merged", proposal.ID) {
			t.Fatalf("unexpected non-merge before reader boundary: recapture=%d report=%+v", i, rep)
		}
		if _, err := v.Find(first.ID); err != nil {
			st, _ := os.Stat(v.notePath(first))
			t.Logf("real_exact_recaptures=%d live_file_bytes=%d read_limit=%d last_merge_success=true error=%v", i, st.Size(), maxDocumentBytes, err)
			t.Fatal("all captures/promotions succeeded but live decision is unreadable")
			return
		}
	}
}

func finalArchive(t *testing.T, entries []tar.Header, bodies []string) string {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for i, h := range entries {
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(bodies[i])); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "snapshot.tar.gz")
	data := b.Bytes()
	sum := sha256.Sum256(data)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p+".sha256", []byte(hex.EncodeToString(sum[:])+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestFinalRestoreRefusalCasesPreserveTree(t *testing.T) {
	for _, kind := range []string{"duplicate", "unsupported-schema", "truncated-gzip", "oversized-entry", "not-vault"} {
		t.Run(kind, func(t *testing.T) {
			v, _ := newVault(t)
			n := mustCapture(t, v, sqliteDecision, t0)
			mustDoctor(t, v, t0)
			before, _ := os.ReadFile(v.notePath(n))
			entries := []tar.Header{{Name: "projects/", Typeflag: tar.TypeDir, Mode: 0700}, {Name: "brain.yaml", Typeflag: tar.TypeReg, Mode: 0600, Size: 18}}
			bodies := []string{"", "schema_version: 1\n"}
			switch kind {
			case "duplicate":
				entries = append(entries, entries[1])
				bodies = append(bodies, bodies[1])
			case "unsupported-schema":
				bodies[1] = "schema_version: 99\n"
				entries[1].Size = int64(len(bodies[1]))
			case "not-vault":
				entries = entries[1:]
				bodies = bodies[1:]
			case "oversized-entry":
				bodies = append(bodies, strings.Repeat("x", restoreMaxFile+1))
				entries = append(entries, tar.Header{Name: "too-large.md", Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(bodies[2]))})
			}
			p := finalArchive(t, entries, bodies)
			if kind == "truncated-gzip" {
				data, _ := os.ReadFile(p)
				data = data[:len(data)-4]
				sum := sha256.Sum256(data)
				os.WriteFile(p, data, 0600)
				os.WriteFile(p+".sha256", []byte(hex.EncodeToString(sum[:])), 0600)
			}
			err := v.Restore(p, true, t0.Add(time.Hour))
			after, readErr := os.ReadFile(v.notePath(n))
			t.Logf("restore_error=%v live_preserved=%t", err, bytes.Equal(before, after))
			if err == nil || readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("refused snapshot changed current note")
			}
		})
	}
}

func TestFinalRestoreEveryRenameFailureRollsBack(t *testing.T) {
	// Discover successful rename count in this same fixture shape, then fault
	// each individual step in a new fixture. No race or hostile same-user claim.
	fixture := func() (*Vault, string, *Note) {
		v, _ := newVault(t)
		n := mustCapture(t, v, sqliteDecision, t0)
		mustDoctor(t, v, t0)
		s := filepath.Join(t.TempDir(), "stage")
		if err := Open(s).Init(); err != nil {
			t.Fatal(err)
		}
		return v, s, n
	}
	v, s, _ := fixture()
	steps := 0
	if err := v.installRestore(s, func(a, b string) error { steps++; return os.Rename(a, b) }); err != nil {
		t.Fatal(err)
	}
	for fail := 1; fail <= steps; fail++ {
		t.Run(fmt.Sprintf("rename-%d", fail), func(t *testing.T) {
			v, s, n := fixture()
			before, _ := os.ReadFile(v.notePath(n))
			calls := 0
			err := v.installRestore(s, func(a, b string) error {
				calls++
				if calls == fail {
					return errors.New("synthetic rename fault")
				}
				return os.Rename(a, b)
			})
			after, readErr := os.ReadFile(v.notePath(n))
			if err == nil || readErr != nil || !bytes.Equal(before, after) {
				t.Fatalf("rollback failed: step=%d err=%v readErr=%v", fail, err, readErr)
			}
		})
	}
	t.Logf("ordinary_rename_failure_positions_exercised=%d", steps)
}

func TestFinalRestoreRollbackFailurePreservesOldTree(t *testing.T) {
	v, _ := newVault(t)
	n := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	before, _ := os.ReadFile(v.notePath(n))
	s := filepath.Join(t.TempDir(), "stage")
	if err := Open(s).Init(); err != nil {
		t.Fatal(err)
	}
	err := v.installRestore(s, func(a, b string) error {
		if strings.HasPrefix(a, s+string(filepath.Separator)) && filepath.Base(a) == "projects" {
			return errors.New("synthetic install failure")
		}
		if strings.Contains(a, ".brain-rollback-") && filepath.Base(a) == "projects" {
			return errors.New("synthetic rollback failure")
		}
		return os.Rename(a, b)
	})
	dirs, _ := filepath.Glob(filepath.Join(filepath.Dir(v.Root), ".brain-rollback-*"))
	preserved := false
	for _, dir := range dirs {
		p := filepath.Join(dir, "projects", "demo", TypeDecision.dir(), n.ID+".md")
		data, _ := os.ReadFile(p)
		if bytes.Equal(data, before) {
			preserved = true
		}
	}
	t.Logf("error=%v old_content_preserved=%t rollback_directories=%d", err, preserved, len(dirs))
	if err == nil || !strings.Contains(err.Error(), "rollback incomplete") || !preserved {
		t.Fatal("rollback failure lost the original")
	}
}

func TestFinalFrozenWriterInventoryAndRecoveryException(t *testing.T) {
	v, _ := newVault(t)
	open := mustCapture(t, v, CaptureInput{Type: TypeOpen, Title: "Outstanding work item", Statement: "The synthetic lifecycle verification still needs a final check."}, t0)
	mustDoctor(t, v, t0)
	snap, err := v.Backup("recovery", t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Freeze(true, t0); err != nil {
		t.Fatal(err)
	}
	operations := map[string]func() error{
		"init": v.Init,
		"capture": func() error {
			_, err := v.Capture(CaptureInput{Type: TypeLearning, Project: "demo", Title: "Frozen attempt", Statement: "This attempt cannot be accepted by a frozen vault."}, t0)
			return err
		},
		"register": func() error { _, err := v.AddProject("other", "Other", t.TempDir()); return err },
		"session":  func() error { return v.RecordSessionEvent("demo", "synthetic", "id", t0) },
		"doctor":   func() error { _, err := v.Doctor(Options{Now: t0}); return err },
		"done":     func() error { _, err := v.Done(open.ID, t0); return err },
		"archive":  func() error { _, err := v.Archive(open.ID, "synthetic", t0); return err },
	}
	for name, op := range operations {
		if err := op(); !errors.Is(err, ErrFrozen) {
			t.Errorf("%s did not honor freeze: %v", name, err)
		}
	}
	// ADR-028 explicitly permits recovery while frozen; do not report this as
	// an all-writers bypass. It must leave the freeze marker in force.
	if err := v.Restore(snap, true, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !v.Frozen() {
		t.Fatal("restore removed incident freeze")
	}
	if _, err := v.Find(open.ID); err != nil {
		t.Fatal(err)
	}
}

func TestFinalSymlinkedProjectParentsCannotInject(t *testing.T) {
	v, root := newVault(t)
	outside := t.TempDir()
	n := &Note{ID: "lrn-outside-project-parent", Type: TypeLearning, Project: "demo", Title: "Outside project parent", Statement: "SYNTHETIC_EXTERNAL_PARENT must never be read through a linked project directory.", Status: StatusActive, Created: t0.Format(time.RFC3339Nano)}
	data, _ := n.Marshal()
	os.MkdirAll(filepath.Join(outside, "learnings"), 0700)
	os.WriteFile(filepath.Join(outside, "learnings", n.ID+".md"), data, 0600)
	project := filepath.Join(v.projectsDir(), "demo")
	registration, _ := os.ReadFile(v.projectFile("demo"))
	os.WriteFile(filepath.Join(outside, "project.yaml"), registration, 0600)
	if err := os.RemoveAll(project); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, project); err != nil {
		t.Fatal(err)
	}
	text, meta := v.Briefing(BriefingOptions{CWD: root, Now: t0})
	if strings.Contains(text, "SYNTHETIC_EXTERNAL_PARENT") || meta.Matched {
		t.Fatal("symlinked project parent granted memory scope")
	}
}

func TestFinalFullyForgedOutsideMetadataDoesNotInheritScope(t *testing.T) {
	v, root := newVault(t)
	foreign := t.TempDir()
	common := filepath.Join(t.TempDir(), ".git")
	gitdir := filepath.Join(common, "worktrees", "foreign")
	os.MkdirAll(gitdir, 0700)
	os.MkdirAll(filepath.Join(common, "objects"), 0700)
	for _, p := range []string{filepath.Join(common, "HEAD"), filepath.Join(common, "config"), filepath.Join(gitdir, "HEAD")} {
		os.WriteFile(p, []byte("synthetic\n"), 0600)
	}
	marker := filepath.Join(foreign, ".git")
	os.WriteFile(marker, []byte("gitdir: "+gitdir+"\n"), 0600)
	os.WriteFile(filepath.Join(gitdir, "gitdir"), []byte(marker+"\n"), 0600)
	// A forged reciprocal gitdir must not make a *different* common directory
	// into an alias of the registered project, even if it names that root.
	os.WriteFile(filepath.Join(gitdir, "commondir"), []byte(filepath.Join(root, ".git")+"\n"), 0600)
	if m := v.ResolveProject(foreign); m.Slug != "" {
		t.Fatalf("forged external metadata widened scope: %+v", m)
	}
	os.WriteFile(filepath.Join(gitdir, "commondir"), []byte("../..\n"), 0600)
	if m := v.ResolveProject(foreign); m.Slug != "" {
		t.Fatalf("unregistered independent fake repo inherited scope: %+v", m)
	}
}
