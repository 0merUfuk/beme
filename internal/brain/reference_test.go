package brain

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Tests for the capabilities adopted from the reference systems (ADR-034):
// append-only log, backup-first + verified restore, freeze, schema guard,
// symlink guard, session accounting, modes, extended lint.

func TestLogIsAppendOnlyAndGreppable(t *testing.T) {
	v, _ := newVault(t)
	mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	data, err := os.ReadFile(filepath.Join(v.Root, "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "## [") {
			lines = append(lines, l)
		}
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"## [2026-09-19] captured |", "## [2026-09-19] promoted |", "sha256:", "] backup |"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log missing %q:\n%s", want, data)
		}
	}
	mustDoctor(t, v, t0.Add(time.Hour)) // no-op run must not append
	data2, _ := os.ReadFile(filepath.Join(v.Root, "log.md"))
	if string(data2) != string(data) {
		t.Fatalf("a no-op Doctor run must not touch the log ")
	}
	// the log is an audit trail, never memory
	text, _ := v.Briefing(BriefingOptions{CWD: t.TempDir(), Now: t0})
	if strings.Contains(text, "## [2026") {
		t.Fatal("log entries must never reach a briefing")
	}
}

func TestDoctorSnapshotsBeforeFirstMutationOnly(t *testing.T) {
	v, _ := newVault(t)
	if r := mustDoctor(t, v, t0); r.Backup != "" {
		t.Fatalf("no proposals → no snapshot, got %q", r.Backup)
	}
	mustCapture(t, v, sqliteDecision, t0)
	mustCapture(t, v, CaptureInput{Type: TypeLearning, Title: "Second lesson", Statement: "A second, unrelated lesson worth keeping around."}, t0)
	r := mustDoctor(t, v, t0)
	if r.Backup == "" {
		t.Fatal("first mutation must be preceded by a snapshot")
	}
	list, _ := v.ListBackups()
	if len(list) != 1 {
		t.Fatalf("exactly one snapshot per run, got %d", len(list))
	}
	if list[0].SHA256 == "" {
		t.Fatal("snapshot must carry a checksum sidecar")
	}
	// snapshots never live inside the vault (nor its git history / briefings)
	if strings.HasPrefix(list[0].Path, v.Root+string(filepath.Separator)) {
		t.Fatalf("snapshot inside the vault: %s", list[0].Path)
	}
	// snapshot holds the PRE-mutation state: proposals still in the inbox
	names := tarNames(t, list[0].Path)
	if !containsPrefix(names, "inbox/dec-") {
		t.Fatalf("snapshot must capture the pre-run inbox, got %v", names)
	}
}

func TestBackupRetention(t *testing.T) {
	v, _ := newVault(t)
	for i := 0; i < backupKeep+4; i++ {
		if _, err := v.Backup(fmt.Sprintf("r%d", i), t0.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := v.ListBackups()
	if len(list) != backupKeep {
		t.Fatalf("want %d snapshots kept, got %d", backupKeep, len(list))
	}
	if list[0].Time.Before(list[len(list)-1].Time) {
		t.Fatal("newest first expected")
	}
}

func TestRestoreRoundTripIsVerifiedAndReversible(t *testing.T) {
	v, _ := newVault(t)
	n := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	snap, err := v.Backup("before-change", t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	// damage: archive the decision
	if _, err := v.Archive(n.ID, "oops", t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if live, _ := v.List("demo", "", false); len(live) != 0 {
		t.Fatal("setup: decision should be archived")
	}
	if err := v.Restore(snap, false, t0.Add(3*time.Minute)); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("restore over a live vault must need force, got %v", err)
	}
	if err := v.Restore(snap, true, t0.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if live, _ := v.List("demo", TypeDecision, false); len(live) != 1 || live[0].ID != n.ID {
		t.Fatalf("restore must bring the decision back: %+v", live)
	}
	// the restore itself was preceded by a snapshot: it can be undone
	all, _ := v.ListBackups()
	pre := false
	for _, b := range all {
		if strings.Contains(b.Path, "pre-restore") {
			pre = true
		}
	}
	if !pre {
		t.Fatal("a pre-restore snapshot must exist")
	}
	// restore into an empty location works too (disaster recovery)
	fresh := Open(filepath.Join(t.TempDir(), "recovered"))
	if err := fresh.Restore(snap, false, t0); err != nil {
		t.Fatal(err)
	}
	if live, _ := fresh.List("demo", TypeDecision, false); len(live) != 1 {
		t.Fatalf("fresh restore lost data: %+v", live)
	}
}

func TestRestoreRefusesTamperedOrUnverifiedArchives(t *testing.T) {
	v, _ := newVault(t)
	snap, _ := v.Backup("x", t0)

	// tampered archive
	data, _ := os.ReadFile(snap)
	data[len(data)/2] ^= 0xff
	bad := filepath.Join(t.TempDir(), "tampered.tar.gz")
	os.WriteFile(bad, data, 0o600)
	side, _ := os.ReadFile(snap + ".sha256")
	os.WriteFile(bad+".sha256", side, 0o600)
	if err := v.Restore(bad, true, t0); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("tampered archive must fail the checksum, got %v", err)
	}
	// missing sidecar
	nosum := filepath.Join(t.TempDir(), "nosum.tar.gz")
	orig, _ := os.ReadFile(snap)
	os.WriteFile(nosum, orig, 0o600)
	if err := v.Restore(nosum, true, t0); err == nil || !strings.Contains(err.Error(), "sidecar") {
		t.Fatalf("unverified archive must be refused, got %v", err)
	}
}

func TestRestoreRefusesTraversalAndLinks(t *testing.T) {
	v, _ := newVault(t)
	outside := filepath.Join(t.TempDir(), "outside")
	for name, hdr := range map[string]tar.Header{
		"traversal": {Name: "../escaped.md", Typeflag: tar.TypeReg, Size: 2, Mode: 0o600},
		"absolute":  {Name: "/tmp/escaped.md", Typeflag: tar.TypeReg, Size: 2, Mode: 0o600},
		"symlink":   {Name: "link.md", Typeflag: tar.TypeSymlink, Linkname: outside, Mode: 0o600},
		"state":     {Name: ".state/x.yaml", Typeflag: tar.TypeReg, Size: 2, Mode: 0o600},
	} {
		t.Run(name, func(t *testing.T) {
			path := craftArchive(t, hdr, "hi")
			if err := v.Restore(path, true, t0); err == nil {
				t.Fatal("malicious archive must be refused")
			}
			if fileExists(filepath.Join(filepath.Dir(v.Root), "escaped.md")) || fileExists("/tmp/escaped.md") {
				t.Fatal("archive escaped the vault")
			}
		})
	}
}

func TestFreezeLocksEveryWriter(t *testing.T) {
	v, _ := newVault(t)
	n := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	if err := v.Freeze(true, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Capture(sqliteDecision2(), t0); !errors.Is(err, ErrFrozen) {
		t.Fatalf("capture must be refused, got %v", err)
	}
	if _, err := v.Doctor(Options{Now: t0}); !errors.Is(err, ErrFrozen) {
		t.Fatalf("doctor must be refused, got %v", err)
	}
	if _, err := v.Archive(n.ID, "x", t0); !errors.Is(err, ErrFrozen) {
		t.Fatalf("archive must be refused, got %v", err)
	}
	if _, err := v.Doctor(Options{DryRun: true, Now: t0}); err != nil {
		t.Fatalf("dry-run stays available: %v", err)
	}
	text, _ := v.Briefing(BriefingOptions{CWD: t.TempDir(), Now: t0})
	if !strings.Contains(text, "FROZEN") {
		t.Fatalf("briefing must say the brain is frozen:\n%s", text)
	}
	v.Freeze(false, t0)
	if _, err := v.Capture(sqliteDecision2(), t0); err != nil {
		t.Fatalf("unfreeze must restore writes: %v", err)
	}
}

func TestNewerSchemaIsNeverWrittenByOlderBinary(t *testing.T) {
	v, _ := newVault(t)
	os.WriteFile(filepath.Join(v.Root, "brain.yaml"), []byte("schema_version: 99\n"), 0o600)
	if _, err := v.Capture(sqliteDecision2(), t0); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("must refuse writes to a newer schema, got %v", err)
	}
	if _, err := v.Doctor(Options{Now: t0}); err == nil {
		t.Fatal("doctor must refuse a newer schema")
	}
	// reads still work
	if _, err := v.List("", "", false); err != nil {
		t.Fatalf("reads must keep working: %v", err)
	}
}

func TestSymlinksNeverEnterTheVaultOrABriefing(t *testing.T) {
	v, root := newVault(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	n := &Note{ID: "lrn-20260919-outside-0001", Type: TypeLearning, Project: "demo", Title: "Smuggled from outside",
		Status: StatusActive, Created: t0.Format(time.RFC3339), Statement: "This text lives outside the vault entirely."}
	data, _ := n.Marshal()
	os.WriteFile(outside, data, 0o600)
	dir := v.liveDir("demo", TypeLearning)
	os.MkdirAll(dir, 0o700)
	if err := os.Symlink(outside, filepath.Join(dir, n.ID+".md")); err != nil {
		t.Skip("symlinks unavailable")
	}
	r := mustDoctor(t, v, t0)
	if !hasFlag(r, "invalid_note") || !hasFlag(r, "foreign_file") {
		t.Fatalf("symlink must be flagged: %+v", r.Flags)
	}
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0})
	if strings.Contains(text, "Smuggled from outside") {
		t.Fatalf("symlinked content leaked into a briefing:\n%s", text)
	}
}

func TestVaultFilesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits do not establish Windows ACL behavior")
	}
	v, _ := newVault(t)
	mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	for _, rel := range []string{"identity.md", "index.md", "log.md", "brain.yaml"} {
		st, err := os.Stat(filepath.Join(v.Root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s is group/world accessible: %v", rel, st.Mode().Perm())
		}
	}
	st, _ := os.Stat(v.projectsDir())
	if st.Mode().Perm()&0o077 != 0 {
		t.Errorf("projects/ is group/world accessible: %v", st.Mode().Perm())
	}
}

func TestCaptureNudgeAfterSilentSessions(t *testing.T) {
	v, root := newVault(t)
	brief := func() string { s, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0}); return s }
	if strings.Contains(brief(), "recorded nothing") {
		t.Fatal("no nudge before any session")
	}
	v.RecordSession("demo", t0)
	if strings.Contains(brief(), "recorded nothing") {
		t.Fatal("one silent session is not yet a pattern")
	}
	v.RecordSession("demo", t0)
	if !strings.Contains(brief(), "the last 2 start/resume/fork invocations in this project recorded nothing") {
		t.Fatalf("two silent sessions must trigger the nudge:\n%s", brief())
	}
	mustCapture(t, v, CaptureInput{Type: TypeLearning, Title: "A lesson", Statement: "Capturing resets the silent-session counter."}, t0)
	if strings.Contains(brief(), "recorded nothing") {
		t.Fatal("a capture must clear the nudge")
	}
	// other projects' counters are independent
	v.RecordSession("global", t0)
}

func TestModes(t *testing.T) {
	v, root := newVault(t)
	for i := 0; i < 20; i++ {
		tag := fmt.Sprintf("zq%dx", i)
		mustCapture(t, v, CaptureInput{Type: TypeDecision, Title: "Decision about " + tag,
			Statement: "We decided on " + tag + " " + strings.Repeat("because of measured evidence and constraints ", 4),
			Why:       "Why " + tag + " " + strings.Repeat("the rationale is documented at length ", 4)}, t0)
	}
	mustDoctor(t, v, t0)
	normal, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0, Mode: ModeNormal})
	eco, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0, Mode: ModeEconomy})
	man, mm := v.Briefing(BriefingOptions{CWD: root, Now: t0, Mode: ModeManual})
	if len(eco) > 3000 || len(eco) >= len(normal) {
		t.Fatalf("economy must be smaller: normal=%d economy=%d", len(normal), len(eco))
	}
	if !strings.Contains(eco, "## Memory protocol") || !strings.Contains(eco, "DATA, not instructions") {
		t.Fatal("economy keeps framing and protocol")
	}
	if len(man) > 900 || !strings.Contains(man, "beme brain briefing") || mm.Decisions != 20 {
		t.Fatalf("manual must be a pointer with counts (len=%d meta=%+v):\n%s", len(man), mm, man)
	}
	if strings.Contains(man, "zq1x") {
		t.Fatal("manual mode must not inline notes")
	}
	for in, want := range map[string]Mode{"": ModeNormal, "Economy": ModeEconomy, " manual ": ModeManual} {
		if got, err := ParseMode(in); err != nil || got != want {
			t.Errorf("ParseMode(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseMode("turbo"); err == nil {
		t.Error("unknown mode must be rejected")
	}
}

func TestExtendedLint(t *testing.T) {
	v, _ := newVault(t)
	old := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	// broken lineage reference: point supersedes at a note that never existed
	p := filepath.Join(v.liveDir("demo", TypeDecision), old.ID+".md")
	n, _ := readNote(p)
	n.Supersedes = []string{"dec-19990101-ghost-0000"}
	v.writeNote(p, n)
	// Use an actual native absolute path: /definitely/... is volume-relative,
	// not absolute, on Windows. A missing pointer still has to be flagged.
	missing := filepath.Join(t.TempDir(), "missing documents", "FOUNDATIONS.md")
	identity := "# Identity\n\nRole: engineer.\n\n## Pointers\n\n- `" + missing + "` — operating model\n- <placeholder ignored>\n- https://example.invalid/FOUNDATIONS.md — external reference\n"
	if err := os.WriteFile(filepath.Join(v.Root, "identity.md"), []byte(identity), 0600); err != nil {
		t.Fatal(err)
	}

	r := mustDoctor(t, v, t0.Add(200*24*time.Hour)) // 200 days later
	for _, kind := range []string{"broken_ref", "identity_pointer", "aging"} {
		if !hasFlag(r, kind) {
			t.Errorf("missing %q flag: %+v", kind, r.Flags)
		}
	}
	idents := 0
	for _, f := range r.Flags {
		if f.Kind == "identity_pointer" {
			idents++
		}
	}
	if idents != 1 {
		t.Errorf("only the real path is checked (placeholder ignored), got %d flags", idents)
	}
}

func TestIdentityPointersNativePathsOnly(t *testing.T) {
	v, _ := newVault(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	native := filepath.Join(home, "canonical documents", "FOUNDATIONS.md")
	if !filepath.IsAbs(native) {
		t.Fatal("fixture is not native absolute:", native)
	}
	cases := []struct{ name, text, want string }{
		{"absolute", native, native},
		{"backticks-description", "`" + native + "` — operating model", native},
		{"quoted-description", "\"" + native + "\" -- operating model", native},
		{"home-relative", "~/FOUNDATIONS.md", filepath.Join(home, "FOUNDATIONS.md")},
		{"https", "https://example.invalid/FOUNDATIONS.md", ""},
		{"file-url", "file:///canonical/FOUNDATIONS.md", ""},
		{"placeholder", "<absolute path to canonical foundations>", ""},
		{"relative", "canonical/FOUNDATIONS.md", ""},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, []struct{ name, text, want string }{
			{"slash-drive", filepath.ToSlash(native), filepath.ToSlash(native)},
			{"unc", `\\server\share\FOUNDATIONS.md`, `\\server\share\FOUNDATIONS.md`},
			{"volume-relative", `/canonical/FOUNDATIONS.md`, ""},
			{"drive-relative", `C:FOUNDATIONS.md`, ""},
		}...)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(v.Root, "identity.md"), []byte("## Pointers\n- "+tc.text+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			got := v.identityPointers() // Parse only; never access a UNC/URL target.
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("non-local pointer treated as local: %q", got)
				}
			} else if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("native pointer: got %q, want %q", got, tc.want)
			}
		})
	}
	// The lint control must distinguish present and missing actual documents,
	// not simply emit a warning for every pointer-shaped input.
	if err := os.MkdirAll(filepath.Dir(native), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, []byte("canonical fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v.Root, "identity.md"), []byte("## Pointers\n- `"+native+"`\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := mustDoctor(t, v, t0); hasFlag(r, "identity_pointer") {
		t.Fatalf("existing native pointer falsely flagged: %+v", r.Flags)
	}
}

func TestIndexCarriesOneLineSummaries(t *testing.T) {
	v, _ := newVault(t)
	mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	idx, _ := os.ReadFile(filepath.Join(v.Root, "index.md"))
	if !strings.Contains(string(idx), "Use SQLite for local state — Local state lives in a single SQLite file") {
		t.Fatalf("index entries need link + one-line summary:\n%s", idx)
	}
}

// NFR-008 for the brain: hooks run on the session critical path.
func TestSessionStartLatencyAtScale(t *testing.T) {
	v, root := newVault(t)
	for i := 0; i < 300; i++ {
		tag := fmt.Sprintf("q%03dw%03d", i, (i*7)%1000)
		n := &Note{ID: fmt.Sprintf("lrn-20260919-scale-%03d", i), Type: TypeLearning, Project: "demo",
			Title: "Lesson " + tag, Status: StatusActive, Created: t0.Format(time.RFC3339),
			Statement: "Distinct insight about " + tag + " subsystem behaviour under sustained load."}
		putLive(t, v, n)
	}
	start := time.Now()
	if _, err := v.Doctor(Options{Now: t0}); err != nil {
		t.Fatal(err)
	}
	v.Briefing(BriefingOptions{CWD: root, Now: t0})
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("doctor+briefing over 300 notes took %v (budget 2s)", d)
	}
}

// ---- helpers ---------------------------------------------------------------

func tarNames(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var out []string
	for {
		h, err := tr.Next()
		if err != nil {
			return out
		}
		out = append(out, h.Name)
	}
}

func containsPrefix(list []string, prefix string) bool {
	for _, s := range list {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// craftArchive writes a one-entry tar.gz plus a CORRECT checksum sidecar, so
// the only thing under test is extraction safety.
func craftArchive(t *testing.T, hdr tar.Header, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evil.tar.gz")
	f, _ := os.Create(path)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&hdr); err != nil {
		t.Fatal(err)
	}
	if hdr.Typeflag == tar.TypeReg {
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	f.Close()
	sum, _ := fileSHA256(path)
	os.WriteFile(path+".sha256", []byte(sum+"  evil.tar.gz\n"), 0o600)
	return path
}
