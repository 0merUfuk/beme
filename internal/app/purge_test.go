package app_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/learning"
	"github.com/0merUfuk/beme/internal/storage"
)

// Physical-purge workflow tests (FR-055, §7.8, ADR-027; threat cases 18/30
// and supplementary S1–S3). Everything runs against synthetic, disposable
// deployments in t.TempDir().

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
	writeCanary(t, src, "CAN-001")
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

func writeCanary(t *testing.T, path, id string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("---\nid: "+id+"\ntitle: \"Canary preference\"\ntype: preference\nstatus: active\n---\n\nThe owner has a "+purgeCanary+".\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func resolvePersonal(rt *app.Runtime) (string, []string, error) {
	sess, err := rt.Serve(contracts.ProfilePersonal, "cap_purge_test", false)
	if err != nil {
		return "", nil, err
	}
	defer sess.Store.Close()
	pack, err := sess.ResolveOnly("owner preference for working", "")
	if err != nil {
		return "", nil, err
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
	raw, _ := json.Marshal(pack.Degradations)
	return b.String(), []string{string(raw)}, nil
}

// resolvedText returns the pack content fields (not the request echo).
func resolvedText(t *testing.T, rt *app.Runtime) (string, []string) {
	t.Helper()
	text, degr, err := resolvePersonal(rt)
	if err != nil {
		t.Fatal(err)
	}
	return text, degr
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
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
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

func purgeReq(key string) app.PurgeRequest {
	return app.PurgeRequest{Key: key, Confirm: key, RemoveCanonical: true}
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
	req := purgeReq("rec_can-001")
	req.DryRun = true
	rep, err := f.rt.PhysicalPurge(req)
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
	for _, p := range []string{f.rt.LedgerPath(), f.rt.PurgeKeyPath()} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dry run wrote %s", filepath.Base(p))
		}
	}
	if f.rt.PendingPurges() != 0 {
		t.Fatal("dry run left a pending journal")
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
	backup := filepath.Join(t.TempDir(), "backup")
	snapshotStore(t, storePath, backup)
	scene := addDerivedCopies(t, f)

	rep, err := rt.PhysicalPurge(purgeReq("rec_can-001"))
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
		"derived_purge_traces":              2,
		"derived_purge_observations":        2,
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
	assertFullyPurged(t, f, scene)

	// Sync resurrection: the same record reappears in the source.
	writeCanary(t, f.sourceFile, "CAN-001")
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

	// Backup restore: the pre-purge store file comes back.
	restoreStore(t, backup, storePath)
	text, degr := resolvedText(t, rt)
	if strings.Contains(text, purgeCanary) {
		t.Fatal("restored backup resurrected purged content")
	}
	joined := strings.Join(degr, " ")
	if !strings.Contains(joined, "out of date") || strings.Contains(joined, "purge") || strings.Contains(joined, "can-001") {
		t.Fatalf("restored purged store must carry a generic rebuild notice that does not reveal the purge; got %v", degr)
	}

	// Migration rollback + re-migrate + rebuild.
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

type derivedScene struct {
	traceDir       string
	traces         []string
	unrelatedTrace string
	obs            *learning.Store
	obsIDs         []string
	unrelatedObs   string
}

// addDerivedCopies creates two traces and two observations that name or
// restate the canary record, plus one unrelated of each.
func addDerivedCopies(t *testing.T, f *purgeFixture) derivedScene {
	t.Helper()
	sc := derivedScene{traceDir: filepath.Join(f.rt.Config.CacheDir, "traces")}
	os.MkdirAll(sc.traceDir, 0o700)
	for _, name := range []string{"t1.json", "t2.json"} {
		os.WriteFile(filepath.Join(sc.traceDir, name), []byte(`[{"step":"select","record_id":"rec_can-001"}]`), 0o600)
		sc.traces = append(sc.traces, name)
	}
	sc.unrelatedTrace = "t3.json"
	os.WriteFile(filepath.Join(sc.traceDir, sc.unrelatedTrace), []byte(`[{"step":"select","record_id":"rec_keep-001"}]`), 0o600)
	obs, err := learning.Open(f.rt.Config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	sc.obs = obs
	for i, h := range []string{"The owner has a " + purgeCanary, "Remember: " + strings.ToUpper(purgeCanary) + "!"} {
		o, err := obs.Observe("observation", h, "fam-"+string(rune('a'+i)), "personal", "personal_private", "")
		if err != nil {
			t.Fatal(err)
		}
		sc.obsIDs = append(sc.obsIDs, o.ObservationID)
	}
	keep, err := obs.Observe("observation", "Prefers small reviewable changes", "fam-keep", "personal", "personal_private", "")
	if err != nil {
		t.Fatal(err)
	}
	sc.unrelatedObs = keep.ObservationID
	return sc
}

func (sc derivedScene) remainingTraces() int {
	n := 0
	for _, name := range sc.traces {
		if _, err := os.Stat(filepath.Join(sc.traceDir, name)); err == nil {
			n++
		}
	}
	return n
}

func (sc derivedScene) remainingObservations() int {
	n := 0
	for _, id := range sc.obsIDs {
		if _, err := sc.obs.Get(id); err == nil {
			n++
		}
	}
	return n
}

func assertFullyPurged(t *testing.T, f *purgeFixture, sc derivedScene) {
	t.Helper()
	rt := f.rt
	for _, root := range []string{rt.Config.DataDir, rt.Config.CacheDir, rt.Config.CanonicalRoot} {
		if hits := bytesUnder(t, root, purgeCanary); len(hits) > 0 {
			t.Fatalf("purged content still on disk: %v", hits)
		}
	}
	if n := sc.remainingTraces(); n != 0 {
		t.Fatalf("%d trace(s) naming the purged record remain", n)
	}
	if _, err := os.Stat(filepath.Join(sc.traceDir, sc.unrelatedTrace)); err != nil {
		t.Fatal("unrelated trace must be kept")
	}
	if n := sc.remainingObservations(); n != 0 {
		t.Fatalf("%d observation(s) restating purged content remain", n)
	}
	if _, err := sc.obs.Get(sc.unrelatedObs); err != nil {
		t.Fatal("unrelated observation must be kept")
	}
	if _, err := os.Stat(f.sourceFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canonical source file must be removed")
	}
	if rt.PendingPurges() != 0 {
		t.Fatal("a completed purge must leave no pending journal")
	}
	text, _ := resolvedText(t, rt)
	if strings.Contains(text, purgeCanary) {
		t.Fatal("purged content still resolves")
	}
	if !strings.Contains(text, "small and reviewable") {
		t.Fatal("unrelated record must still resolve after purge")
	}
}

func stageBase(stage string) string {
	base, _, _ := strings.Cut(stage, ":")
	return base
}

// TestPhysicalPurgeResumesAfterFailureAtEveryStage injects a failure at every
// deletion stage (mid-stage for multi-item stages) and proves a second run of
// the same purge completes all remaining store, trace, observation, and
// canonical cleanup, and a third run is an idempotent no-op.
func TestPhysicalPurgeResumesAfterFailureAtEveryStage(t *testing.T) {
	// Every stage the workflow executes must have a failure case below.
	seen := map[string]bool{}
	{
		f := newPurgeFixture(t)
		addDerivedCopies(t, f)
		req := purgeReq("rec_can-001")
		req.FailAt = func(stage string) error { seen[stageBase(stage)] = true; return nil }
		if _, err := f.rt.PhysicalPurge(req); err != nil {
			t.Fatal(err)
		}
	}
	errInjected := errors.New("injected purge failure")
	cases := []struct {
		name    string
		stage   string
		nth     int
		resumed bool
		pending int
	}{
		{"ledger", app.StageLedger, 1, false, 0},
		{"journal", app.StageJournal, 1, false, 0},
		{"projection", app.StageProjection + ":personal", 1, true, 1},
		{"compact", app.StageCompact + ":personal", 1, true, 1},
		{"trace-partial", app.StageTrace, 2, true, 1},
		{"observation-partial", app.StageObservation, 2, true, 1},
		{"canonical", app.StageCanonical, 1, true, 1},
		{"finalize", app.StageFinalize, 1, true, 1},
	}
	covered := map[string]bool{}
	for _, c := range cases {
		covered[stageBase(c.stage)] = true
	}
	for _, stage := range app.PurgeStages {
		if !seen[stage] {
			t.Fatalf("stage %q never executed in a full purge — fixture does not exercise it", stage)
		}
		if !covered[stage] {
			t.Fatalf("stage %q has no failure-injection case", stage)
		}
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newPurgeFixture(t)
			sc := addDerivedCopies(t, f)
			req := purgeReq("rec_can-001")
			calls := 0
			req.FailAt = func(stage string) error {
				if stage == c.stage {
					calls++
					if calls == c.nth {
						return errInjected
					}
				}
				return nil
			}
			if _, err := f.rt.PhysicalPurge(req); !errors.Is(err, errInjected) {
				t.Fatalf("first run: want injected failure at %s, got %v", c.stage, err)
			}
			if got := f.rt.PendingPurges(); got != c.pending {
				t.Fatalf("pending journals after failure at %s: got %d, want %d", c.stage, got, c.pending)
			}
			switch c.name {
			case "trace-partial":
				if n := sc.remainingTraces(); n != 1 {
					t.Fatalf("partial trace cleanup: %d remain, want 1", n)
				}
			case "observation-partial":
				if n := sc.remainingObservations(); n != 1 {
					t.Fatalf("partial observation cleanup: %d remain, want 1", n)
				}
			}
			if c.stage != app.StageLedger {
				// Fingerprints are written before any deletion: the content
				// no longer resolves even though cleanup is incomplete.
				if text, _ := resolvedText(t, f.rt); strings.Contains(text, purgeCanary) {
					t.Fatalf("content still resolves after failure at %s", c.stage)
				}
			}

			req.FailAt = nil
			rep, err := f.rt.PhysicalPurge(req)
			if err != nil {
				t.Fatalf("second run: %v", err)
			}
			if rep.Resumed != c.resumed {
				t.Fatalf("second run resumed=%v, want %v", rep.Resumed, c.resumed)
			}
			assertFullyPurged(t, f, sc)

			again, err := f.rt.PhysicalPurge(req)
			if err != nil || !again.AlreadyPurged {
				t.Fatalf("third run must be an idempotent no-op; got %+v err=%v", again, err)
			}
		})
	}
}

func TestPhysicalPurgeIsIdempotent(t *testing.T) {
	f := newPurgeFixture(t)
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatal(err)
	}
	ledgerBefore, _ := os.ReadFile(f.rt.LedgerPath())
	for i := 0; i < 2; i++ {
		rep, err := f.rt.PhysicalPurge(purgeReq("rec_can-001"))
		if err != nil || !rep.AlreadyPurged || rep.Records != 0 {
			t.Fatalf("repeat purge %d: %+v err=%v", i, rep, err)
		}
	}
	ledgerAfter, _ := os.ReadFile(f.rt.LedgerPath())
	if !bytes.Equal(ledgerBefore, ledgerAfter) {
		t.Fatal("repeat purges must not change the ledger")
	}

	g := newPurgeFixture(t)
	for i := 0; i < 2; i++ {
		rep, err := g.rt.PhysicalPurge(purgeReq("source:purge-src"))
		if err != nil {
			t.Fatalf("source purge run %d: %v", i, err)
		}
		if (i == 0) == rep.AlreadyPurged {
			t.Fatalf("source purge run %d: already_purged=%v", i, rep.AlreadyPurged)
		}
	}
	// A source-level purge also blocks records later added to that source.
	writeCanary(t, filepath.Join(filepath.Dir(g.sourceFile), "NEW-001.md"), "NEW-001")
	brep, err := g.rt.BuildProfile(contracts.ProfilePersonal)
	if err != nil {
		t.Fatal(err)
	}
	if brep.RecordsIngested != 0 || brep.PurgeBlocked != 1 {
		t.Fatalf("purged source must not re-ingest: %+v", brep)
	}
}

// TestPhysicalPurgeRemovesEveryProvenanceRef pins that purge follows the
// record's actual provenance refs — several, with nonconventional IDs — for
// both projection cleanup and canonical-file removal.
func TestPhysicalPurgeRemovesEveryProvenanceRef(t *testing.T) {
	f := newPurgeFixture(t)
	rt := f.rt
	const text = "multi provenance canary for purge tests"
	root := filepath.Dir(filepath.Dir(f.sourceFile))
	locs := []string{"entries/MULTI-A.md", "entries/deep/MULTI-B.md"}
	for _, loc := range locs {
		p := filepath.Join(root, filepath.FromSlash(loc))
		os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	refs := []string{"custom:ref/alpha", "p-β-002"}
	st, err := storage.Open(rt.ProjectionPath(contracts.ProfilePersonal))
	if err != nil {
		t.Fatal(err)
	}
	rec := contracts.Record{SchemaVersion: contracts.SchemaVersion, RecordID: "rec_multi-prov", SourceID: "purge-src", SourceRecordID: "MULTI-PROV",
		Kind: contracts.Kind("preference"), Title: text, Statement: text, CompactText: text, Status: contracts.StatusActive,
		Authority: contracts.AuthorityDefault, SourceRole: contracts.SourceRole("canonical_reusable_knowledge"), Trust: contracts.Trust("canonical"),
		Sensitivity: "personal_private", ProvenanceRefs: refs}
	provs := []contracts.Provenance{
		{ProvenanceID: refs[0], SourceID: "purge-src", SourceRecordID: "MULTI-PROV", Locator: locs[0], ContentHash: "sha256:a", CapturedAt: "2026-09-15T00:00:00Z", IngestionVersion: 1},
		{ProvenanceID: refs[1], SourceID: "purge-src", SourceRecordID: "MULTI-PROV-LEGACY", Locator: locs[1], ContentHash: "sha256:b", CapturedAt: "2026-09-15T00:00:00Z", IngestionVersion: 1},
	}
	err = st.PutRecords([]contracts.Record{rec}, provs)
	st.Close()
	if err != nil {
		t.Fatal(err)
	}

	rep, err := rt.PhysicalPurge(purgeReq("rec_multi-prov"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range rep.Steps {
		if s.Step == "canonical_source_removed" && s.Count != 2 {
			t.Fatalf("both canonical files must be removed; got %d", s.Count)
		}
	}
	st, err = storage.Open(rt.ProjectionPath(contracts.ProfilePersonal))
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if _, ok := st.Provenance(ref); ok {
			t.Fatalf("provenance %q survived purge", ref)
		}
	}
	if _, ok := st.Provenance("prov_multi-prov"); ok {
		t.Fatal("unexpected convention-derived provenance row")
	}
	st.Close()
	if hits := bytesUnder(t, rt.Config.DataDir, text); len(hits) > 0 {
		t.Fatalf("purged content still on disk: %v", hits)
	}
	for _, loc := range locs {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(loc))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("canonical file %s survived purge", loc)
		}
	}
	if text, _ := resolvedText(t, rt); !strings.Contains(text, purgeCanary) {
		t.Fatal("purging one record must not affect others")
	}
}

// TestPurgeLedgerIsKeyedAndContentFree pins ADR-027's minimality: the ledger
// holds only keyed fingerprints — no IDs, content hashes, text, or unkeyed
// digests — and a leaked ledger cannot be matched without its key.
func TestPurgeLedgerIsKeyedAndContentFree(t *testing.T) {
	f := newPurgeFixture(t)
	original, _ := os.ReadFile(f.sourceFile)
	if err := f.rt.Forget(contracts.ProfilePersonal, "rec_can-001", "before purge"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatal(err)
	}
	ledger, err := os.ReadFile(f.rt.LedgerPath())
	if err != nil {
		t.Fatal(err)
	}
	contentSum := sha256.Sum256(original)
	idSum := sha256.Sum256([]byte("rec_can-001"))
	low := bytes.ToLower(ledger)
	for _, needle := range []string{"can-001", "purge-src", purgeCanary, "canary", hex.EncodeToString(contentSum[:]), hex.EncodeToString(idSum[:]), `"at"`} {
		if bytes.Contains(low, bytes.ToLower([]byte(needle))) {
			t.Fatalf("ledger reveals %q", needle)
		}
	}
	var parsed struct {
		Revocations []app.LedgerRevocation `json:"revocations"`
		Purges      []string               `json:"purges"`
	}
	if err := json.Unmarshal(ledger, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Revocations) != 0 {
		t.Fatalf("purge must drop superseded plain revocations; got %v", parsed.Revocations)
	}
	if len(parsed.Purges) != 2 || !sort.StringsAreSorted(parsed.Purges) {
		t.Fatalf("want 2 sorted fingerprints (key + record); got %v", parsed.Purges)
	}
	for _, fp := range parsed.Purges {
		if !strings.HasPrefix(fp, "hmac-sha256:") || len(fp) != len("hmac-sha256:")+64 {
			t.Fatalf("purge entry is not a keyed fingerprint: %q", fp)
		}
	}
	keyData, err := os.ReadFile(f.rt.PurgeKeyPath())
	var keyFile struct {
		Key       string `json:"key"`
		KeyID     string `json:"key_id"`
		Committed bool   `json:"committed"`
	}
	if err != nil || json.Unmarshal(keyData, &keyFile) != nil || len(keyFile.Key) != 64 || !keyFile.Committed {
		t.Fatalf("purge key must exist as a committed 32-byte hex key: %v %s", err, keyData)
	}
	if bytes.Contains(ledger, []byte(keyFile.Key)) {
		t.Fatal("ledger must not contain the purge key")
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(f.rt.PurgeKeyPath()); info.Mode().Perm() != 0o600 {
			t.Fatalf("purge key permissions %v, want 0600", info.Mode().Perm())
		}
	}
	ignore, _ := os.ReadFile(filepath.Join(filepath.Dir(f.rt.LedgerPath()), ".gitignore"))
	if !strings.Contains(string(ignore), "purge.key") || !strings.Contains(string(ignore), "pending/") {
		t.Fatalf("ledger .gitignore must exclude the key and journals; got %q", ignore)
	}

	// A leaked ledger in another deployment with the same record identity.
	g := newPurgeFixture(t)
	copyFile(t, f.rt.LedgerPath(), g.rt.LedgerPath())
	if _, _, err := resolvePersonal(g.rt); !errors.Is(err, app.ErrPurgeKeyMissing) {
		t.Fatalf("ledger without its key must fail closed; got %v", err)
	}
	// A committed key file for a DIFFERENT key: well-formed and current, so
	// the rejection can only come from the ledger's key binding.
	foreign := make([]byte, 32)
	rand.Read(foreign)
	foreignID := sha256.Sum256(append([]byte("beme-purge-key-id\x00"), foreign...))
	foreignKeyFile, err := json.Marshal(map[string]any{
		"schema_version": "3",
		"key":            hex.EncodeToString(foreign),
		"key_id":         hex.EncodeToString(foreignID[:16]),
		"generation":     1,
		"committed":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(g.rt.PurgeKeyPath(), foreignKeyFile, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolvePersonal(g.rt); !errors.Is(err, app.ErrLedgerUnusable) {
		t.Fatalf("a ledger paired with another deployment's key must fail closed; got %v", err)
	}
	copyFile(t, f.rt.PurgeKeyPath(), g.rt.PurgeKeyPath())
	if text, _ := resolvedText(t, g.rt); strings.Contains(text, purgeCanary) {
		t.Fatal("with the original key the same identity must match")
	}
}

// TestPurgeLedgerMatchesIdentityNotContent pins the accepted boundary of the
// minimal ledger: accidental resurrection (same record identity via sync,
// restore, rollback, rebuild) is blocked; deliberately re-authoring the same
// words under a new record ID is not, because the ledger holds no content.
func TestPurgeLedgerMatchesIdentityNotContent(t *testing.T) {
	f := newPurgeFixture(t)
	req := purgeReq("rec_can-001")
	req.RemoveCanonical = false
	if _, err := f.rt.PhysicalPurge(req); err != nil {
		t.Fatal(err)
	}
	brep, err := f.rt.BuildProfile(contracts.ProfilePersonal)
	if err != nil {
		t.Fatal(err)
	}
	if brep.PurgeBlocked != 1 {
		t.Fatalf("same identity must be blocked; %+v", brep)
	}
	writeCanary(t, f.sourceFile, "CAN-REAUTHORED")
	if brep, err = f.rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	if brep.PurgeBlocked != 0 {
		t.Fatalf("a new identity must not be matched by content; %+v", brep)
	}
	if text, _ := resolvedText(t, f.rt); !strings.Contains(text, purgeCanary) {
		t.Fatal("re-authored record under a new ID resolves (documented boundary)")
	}
}

func TestMissingPurgeKeyFailsClosed(t *testing.T) {
	f := newPurgeFixture(t)
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.rt.PurgeKeyPath()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolvePersonal(f.rt); !errors.Is(err, app.ErrPurgeKeyMissing) {
		t.Fatalf("resolution without the purge key must fail closed; got %v", err)
	}
	if _, err := f.rt.BuildProfile(contracts.ProfilePersonal); !errors.Is(err, app.ErrPurgeKeyMissing) {
		t.Fatalf("rebuild without the purge key must fail closed; got %v", err)
	}
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_keep-001")); !errors.Is(err, app.ErrPurgeKeyMissing) {
		t.Fatalf("purge without the purge key must fail closed; got %v", err)
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
	if _, _, err := resolvePersonal(f.rt); err == nil {
		t.Fatal("an unreadable tombstone ledger must fail resolution closed")
	}
	if _, err := f.rt.BuildProfile(contracts.ProfilePersonal); err == nil {
		t.Fatal("an unreadable tombstone ledger must block rebuild")
	}
}
