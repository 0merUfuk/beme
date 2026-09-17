package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
)

func writeDescriptor(t *testing.T, cfg, id, root string) string {
	t.Helper()
	p := filepath.Join(cfg, "sources", id+".yaml")
	desc := "schema_version: \"1\"\nsource_id: " + id + "\ntype: directory\nroot: " + filepath.ToSlash(root) +
		"\npurpose: [reusable_knowledge]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: default\nsensitivity: personal_private\nprofiles_allowed: [personal]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"
	if err := os.WriteFile(p, []byte(desc), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestFailedSourceIsReportedAndKeepsDoctorUnhealthy: a registered source that
// does not ingest is a reported, persisted failure — the build names it, the
// projection records it, and doctor findings stay until a build ingests it —
// while the sources that did ingest keep serving.
func TestFailedSourceIsReportedAndKeepsDoctorUnhealthy(t *testing.T) {
	f := newPurgeFixture(t)
	base := t.TempDir()

	missing := writeDescriptor(t, f.cfg, "missing-root", filepath.Join(base, "does-not-exist"))
	oversized := filepath.Join(base, "oversized")
	if err := os.MkdirAll(filepath.Join(oversized, "entries"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oversized, "entries", "BIG.md"), make([]byte, 3<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	tooBig := writeDescriptor(t, f.cfg, "oversized-file", oversized)

	rt, err := app.Load(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := rt.BuildProfile(contracts.ProfilePersonal)
	if err != nil {
		t.Fatal(err)
	}
	failed := map[string]string{}
	for _, sf := range rep.Failed {
		failed[sf.SourceID] = sf.Reason
	}
	if !strings.Contains(failed["missing-root"], "root not resolvable") || !strings.Contains(failed["oversized-file"], "size limit") {
		t.Fatalf("both registered sources that did not ingest must be reported with their reason: %+v", rep.Failed)
	}
	if rep.RecordsIngested == 0 {
		t.Fatal("positive control: the healthy source must still ingest")
	}

	findings, err := rt.ProjectionFindings(contracts.ProfilePersonal)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(findings, "\n")
	for _, id := range []string{"missing-root", "oversized-file"} {
		if !strings.Contains(joined, "source "+id+" was not ingested") {
			t.Errorf("doctor findings must name failed source %s; got %q", id, joined)
		}
	}

	// A fresh runtime (a later doctor run) still sees the persisted failures.
	later, err := app.Load(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := later.ProjectionFindings(contracts.ProfilePersonal); err != nil || len(again) != 2 {
		t.Fatalf("failures must persist across processes until a clean build: %v %v", again, err)
	}

	// Fixing the sources and rebuilding clears the findings.
	for _, p := range []string{missing, tooBig} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	fixed, err := app.Load(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rep, err := fixed.BuildProfile(contracts.ProfilePersonal); err != nil || len(rep.Failed) != 0 {
		t.Fatalf("a clean build must report no failures: %+v %v", rep, err)
	}
	if clean, err := fixed.ProjectionFindings(contracts.ProfilePersonal); err != nil || len(clean) != 0 {
		t.Fatalf("a clean build must clear the findings: %v %v", clean, err)
	}
}
