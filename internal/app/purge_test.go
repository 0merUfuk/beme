package app_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/learning"
	"github.com/0merUfuk/beme/internal/storage"
)

// Physical-purge workflow tests (FR-055, §7.8, threat cases 18 and 30).
// Everything runs against synthetic, disposable deployments in t.TempDir().

const purgeCanary = "disposable canary preference for purge tests"

type purgeFixture struct {
	rt         *app.Runtime
	cfg        string
	sourceFile string
}

func newPurgeFixture(t *testing.T) *purgeFixture {
	t.Helper()
	base := t.TempDir()
	cfg := filepath.Join(base, "cfg")
	root := filepath.Join(base, "personal")
	entries := filepath.Join(root, "entries")
	for _, d := range []string{filepath.Join(cfg, "sources"), entries} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	src := filepath.Join(entries, "CAN-001.md")
	writeCanary(t, src)
	if err := os.WriteFile(filepath.Join(entries, "KEEP-001.md"), []byte("---\nid: KEEP-001\ntitle: \"Kept preference\"\ntype: preference\nstatus: active\n---\n\nKeep changes small and reviewable.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	desc := "schema_version: \"1\"\nsource_id: purge-src\ntype: directory\nroot: " + filepath.ToSlash(root) +
		"\npurpose: [reusable_knowledge]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: default\nsensitivity: personal_private\nprofiles_allowed: [personal]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"
	if err := os.WriteFile(filepath.Join(cfg, "sources", "src.yaml"), []byte(desc), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, err := app.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	return &purgeFixture{rt: rt, cfg: cfg, sourceFile: src}
}

func writeCanary(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("---\nid: CAN-001\ntitle: \"Canary preference\"\ntype: preference\nstatus: active\n---\n\nThe owner has a "+purgeCanary+".\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// resolvedText returns the pack content fields (not the request echo).
func resolvedText(t *testing.T, rt *app.Runtime) (string, []string) {
	t.Helper()
	sess, err := rt.Serve(contracts.ProfilePersonal, "cap_purge_test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()
	pack, err := sess.ResolveOnly("owner preference for working", "")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, g := range pack.Guidance {
		b.WriteString(g.Text + "\n")
	}
	for _, c := range pack.Constraints {
		b.WriteString(c.Text + "\n")
	}
	for _, p := range pack.Precedents {
		b.WriteString(p.Text + "\n")
	}
	for _, k := range pack.Knowledge {
		b.WriteString(k.Title + " " + k.Description + "\n")
	}
	degr := []string{}
	raw, _ := json.Marshal(pack.Degradations)
	degr = append(degr, string(raw))
	return b.String(), degr
}

// bytesUnder reports every file under root whose raw bytes contain needle.
func bytesUnder(t *testing.T, root, needle string) []string {
	t.Helper()
	hits := []string{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil && bytes.Contains(data, []byte(needle)) {
			hits = append(hits, path)
		}
		return nil
	})
	return hits
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// snapshotStore copies a closed store's files (db + WAL side files).
func snapshotStore(t *testing.T, storePath, dir string) {
	t.Helper()
	os.MkdirAll(dir, 0o700)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(storePath + suffix); err == nil {
			copyFile(t, storePath+suffix, filepath.Join(dir, "store.db"+suffix))
		}
	}
}

func restoreStore(t *testing.T, dir, storePath string) {
	t.Helper()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(storePath + suffix)
		if _, err := os.Stat(filepath.Join(dir, "store.db"+suffix)); err == nil {
			copyFile(t, filepath.Join(dir, "store.db"+suffix), storePath+suffix)
		}
	}
}

func TestPhysicalPurgeRequiresExactConfirmation(t *testing.T) {
	f := newPurgeFixture(t)
	for _, confirm := range []string{"", "rec_can-00", "yes"} {
		_, err := f.rt.PhysicalPurge(app.PurgeRequest{Key: "rec_can-001", Confirm: confirm, RemoveCanonical: true})
		if !errors.Is(err, app.ErrPurgeNotConfirmed) {
			t.Fatalf("confirm %q: want ErrPurgeNotConfirmed, got %v", confirm, err)
		}
	}
	if _, err := os.Stat(f.sourceFile); err != nil {
		t.Fatal("unconfirmed purge must not touch the canonical source")
	}
	if text, _ := resolvedText(t, f.rt); !strings.Contains(text, purgeCanary) {
		t.Fatal("unconfirmed purge must not change resolution")
	}
	if _, err := f.rt.PhysicalPurge(app.PurgeRequest{Key: "rec_absent", Confirm: "rec_absent"}); !errors.Is(err, app.ErrPurgeNotFound) {
		t.Fatalf("unknown key: want ErrPurgeNotFound, got %v", err)
	}
}

func TestPhysicalPurgeDryRunChangesNothing(t *testing.T) {
	f := newPurgeFixture(t)
	store := f.rt.ProjectionPath(contracts.ProfilePersonal)
	before, _ := os.ReadFile(store)
	rep, err := f.rt.PhysicalPurge(app.PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001", RemoveCanonical: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || rep.Records != 1 {
		t.Fatalf("dry run report: %+v", rep)
	}
	for _, s := range rep.Steps {
		if s.Outcome == "done" {
			t.Fatalf("dry run executed step %s", s.Step)
		}
	}
	after, _ := os.ReadFile(store)
	if !bytes.Equal(before, after) {
		t.Fatal("dry run modified the projection store")
	}
	if _, err := os.Stat(f.rt.LedgerPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry run wrote the ledger")
	}
	if _, err := os.Stat(f.sourceFile); err != nil {
		t.Fatal("dry run removed the canonical source")
	}
}

// TestPhysicalPurgeErasesAndBlocksResurrection is the end-to-end synthetic
// purge: erasure from every derived location, canonical removal, and no
// resurrection through sync, rebuild, backup restore, or migration rollback.
func TestPhysicalPurgeErasesAndBlocksResurrection(t *testing.T) {
	f := newPurgeFixture(t)
	rt := f.rt
	storePath := rt.ProjectionPath(contracts.ProfilePersonal)

	if text, _ := resolvedText(t, rt); !strings.Contains(text, purgeCanary) {
		t.Fatal("fixture precondition: canary must resolve before purge")
	}
	// Backup taken before the purge (simulates an operator/Time Machine copy).
	backup := filepath.Join(t.TempDir(), "backup")
	snapshotStore(t, storePath, backup)

	// Derived copies elsewhere: a persisted trace and a pending observation.
	traceDir := filepath.Join(rt.Config.CacheDir, "traces")
	os.MkdirAll(traceDir, 0o700)
	os.WriteFile(filepath.Join(traceDir, "t1.json"), []byte(`[{"step":"select","record_id":"rec_can-001"}]`), 0o600)
	os.WriteFile(filepath.Join(traceDir, "t2.json"), []byte(`[{"step":"select","record_id":"rec_keep-001"}]`), 0o600)
	obs, err := learning.Open(rt.Config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	o, err := obs.Observe("observation", "The owner has a "+purgeCanary, "fam-test", "personal", "personal_private", "")
	if err != nil {
		t.Fatal(err)
	}
	keep, err := obs.Observe("observation", "Prefers small reviewable changes", "fam-test", "personal", "personal_private", "")
	if err != nil {
		t.Fatal(err)
	}

	rep, err := rt.PhysicalPurge(app.PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001", RemoveCanonical: true})
	if err != nil {
		t.Fatal(err)
	}
	steps := map[string]app.PurgeStep{}
	for _, s := range rep.Steps {
		steps[s.Step] = s
	}
	for name, want := range map[string]int{
		"anti_resurrection_tombstone":       1,
		"derived_purge_projection_personal": 1,
		"derived_purge_traces":              1,
		"derived_purge_observations":        1,
		"canonical_source_removed":          1,
	} {
		if s := steps[name]; s.Outcome != "done" || s.Count != want {
			t.Fatalf("step %s: got %+v, want done/%d", name, s, want)
		}
	}
	repJSON, _ := json.Marshal(rep)
	if bytes.Contains(repJSON, []byte(purgeCanary)) || bytes.Contains(repJSON, []byte("Canary preference")) {
		t.Fatal("purge report must never carry content")
	}

	// 1. Erasure: no raw bytes remain anywhere in the deployment's data,
	// cache, or canonical root (store file, WAL, SHM, ledger, traces, obs).
	for _, root := range []string{rt.Config.DataDir, rt.Config.CacheDir, rt.Config.CanonicalRoot} {
		if hits := bytesUnder(t, root, purgeCanary); len(hits) > 0 {
			t.Fatalf("purged content still on disk: %v", hits)
		}
	}
	ledger, _ := os.ReadFile(rt.LedgerPath())
	if bytes.Contains(ledger, []byte("can-001")) || bytes.Contains(ledger, []byte("Canary")) {
		t.Fatal("ledger must hold fingerprints only")
	}
	if _, err := os.Stat(f.sourceFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canonical source file must be removed with --remove-canonical")
	}
	if _, err := os.Stat(filepath.Join(traceDir, "t2.json")); err != nil {
		t.Fatal("unrelated trace must be kept")
	}
	if _, err := obs.Get(o.ObservationID); err == nil {
		t.Fatal("observation restating purged content must be removed")
	}
	if _, err := obs.Get(keep.ObservationID); err != nil {
		t.Fatal("unrelated observation must be kept")
	}
	text, _ := resolvedText(t, rt)
	if strings.Contains(text, purgeCanary) {
		t.Fatal("purged content still resolves")
	}
	if !strings.Contains(text, "small and reviewable") {
		t.Fatal("unrelated record must still resolve after purge")
	}

	// 2. Sync resurrection: the file reappears in the source; rebuild refuses it.
	writeCanary(t, f.sourceFile)
	brep, err := rt.BuildProfile(contracts.ProfilePersonal)
	if err != nil {
		t.Fatal(err)
	}
	if brep.PurgeBlocked != 1 {
		t.Fatalf("rebuild must block the purged entry; report %+v", brep)
	}
	if hits := bytesUnder(t, rt.Config.DataDir, purgeCanary); len(hits) > 0 {
		t.Fatalf("rebuild re-ingested purged content: %v", hits)
	}
	// Same bytes under a new record ID are blocked by content hash.
	renamed := strings.Replace(mustRead(t, f.sourceFile), "id: CAN-001", "id: CAN-RENAMED", 1)
	os.Remove(f.sourceFile)
	os.WriteFile(filepath.Join(filepath.Dir(f.sourceFile), "CAN-RENAMED.md"), []byte(renamed), 0o600)
	if brep, err = rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	if text, _ := resolvedText(t, rt); strings.Contains(text, purgeCanary) {
		t.Fatal("renamed identical content must not be re-ingested")
	}
	os.Remove(filepath.Join(filepath.Dir(f.sourceFile), "CAN-RENAMED.md"))
	_ = brep

	// 3. Backup restore: the pre-purge store file comes back.
	restoreStore(t, backup, storePath)
	text, degr := resolvedText(t, rt)
	if strings.Contains(text, purgeCanary) {
		t.Fatal("restored backup resurrected purged content")
	}
	if !strings.Contains(strings.Join(degr, " "), "physically purged") {
		t.Fatalf("restored purged store must carry a degradation notice; got %v", degr)
	}

	// 4. Migration rollback + re-migrate + rebuild.
	st, err := storage.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	if text, _ := resolvedText(t, rt); strings.Contains(text, purgeCanary) {
		t.Fatal("rollback + rebuild resurrected purged content")
	}
}

// TestForgetSurvivesRestoreAndCorruptRecovery pins threat case 18: a logical
// forget must survive a pre-forget backup restore and a corrupt-store rebuild,
// both of which discard the store-local tombstone.
func TestForgetSurvivesRestoreAndCorruptRecovery(t *testing.T) {
	f := newPurgeFixture(t)
	rt := f.rt
	storePath := rt.ProjectionPath(contracts.ProfilePersonal)
	backup := filepath.Join(t.TempDir(), "backup")
	snapshotStore(t, storePath, backup)

	if err := rt.Forget(contracts.ProfilePersonal, "rec_can-001", "test forget"); err != nil {
		t.Fatal(err)
	}
	if text, _ := resolvedText(t, rt); strings.Contains(text, purgeCanary) {
		t.Fatal("forgotten record still resolves")
	}

	restoreStore(t, backup, storePath)
	if text, _ := resolvedText(t, rt); strings.Contains(text, purgeCanary) {
		t.Fatal("pre-forget backup restore resurrected the forgotten record")
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		os.Remove(storePath + suffix)
	}
	if err := os.WriteFile(storePath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	if text, _ := resolvedText(t, rt); strings.Contains(text, purgeCanary) {
		t.Fatal("corrupt-store recovery resurrected the forgotten record")
	}
}

func TestCorruptLedgerFailsClosed(t *testing.T) {
	f := newPurgeFixture(t)
	os.MkdirAll(filepath.Dir(f.rt.LedgerPath()), 0o700)
	if err := os.WriteFile(f.rt.LedgerPath(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	sess, err := f.rt.Serve(contracts.ProfilePersonal, "cap_purge_test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()
	if _, err := sess.ResolveOnly("owner preference", ""); err == nil {
		t.Fatal("an unreadable tombstone ledger must fail resolution closed")
	}
	if _, err := f.rt.BuildProfile(contracts.ProfilePersonal); err == nil {
		t.Fatal("an unreadable tombstone ledger must block rebuild")
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
