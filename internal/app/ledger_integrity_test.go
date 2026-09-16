package app_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/durable"
	"github.com/0merUfuk/beme/internal/learning"
)

// Ledger integrity (ADR-030). Enforcement state is ledger/tombstones.json plus
// ledger/purge.key; every partial loss, partial rollback, mismatch, or
// malformed entry must fail closed on every surface — content, metadata,
// counts, learning, rebuild, and maintenance.

func readJSONFile(t *testing.T, path string, into any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatal(err)
	}
}

func rewriteLedger(t *testing.T, rt *app.Runtime, edit func(map[string]any)) {
	t.Helper()
	var doc map[string]any
	readJSONFile(t, rt.LedgerPath(), &doc)
	edit(doc)
	data, _ := json.Marshal(doc)
	if err := os.WriteFile(rt.LedgerPath(), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLedgerIntegrityFailsClosedOnEverySurface(t *testing.T) {
	for _, c := range []struct {
		name   string
		break_ func(t *testing.T, rt *app.Runtime)
	}{
		{"missing ledger with committed key", func(t *testing.T, rt *app.Runtime) {
			if err := os.Remove(rt.LedgerPath()); err != nil {
				t.Fatal(err)
			}
		}},
		{"rolled-back ledger", func(t *testing.T, rt *app.Runtime) {
			old, err := os.ReadFile(rt.LedgerPath())
			if err != nil {
				t.Fatal(err)
			}
			if err := rt.Forget(contracts.ProfilePersonal, "rec_later", "advances the generation"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(rt.LedgerPath(), old, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"truncated fingerprint", func(t *testing.T, rt *app.Runtime) {
			rewriteLedger(t, rt, func(doc map[string]any) {
				purges := doc["purges"].([]any)
				purges[0] = strings.TrimSuffix(purges[0].(string), purges[0].(string)[len(purges[0].(string))-1:])
			})
		}},
		{"non-hex fingerprint", func(t *testing.T, rt *app.Runtime) {
			rewriteLedger(t, rt, func(doc map[string]any) {
				purges := doc["purges"].([]any)
				purges[0] = "hmac-sha256:" + strings.Repeat("Z", 64)
			})
		}},
		{"malformed observation fingerprint", func(t *testing.T, rt *app.Runtime) {
			rewriteLedger(t, rt, func(doc map[string]any) { doc["observations"] = []string{"hmac-sha256:abc"} })
		}},
		{"key from another ledger", func(t *testing.T, rt *app.Runtime) {
			g := newPurgeFixture(t)
			if err := g.rt.Forget(contracts.ProfilePersonal, "rec_other", "creates another key"); err != nil {
				t.Fatal(err)
			}
			copyFile(t, g.rt.PurgeKeyPath(), rt.PurgeKeyPath())
		}},
		{"missing key", func(t *testing.T, rt *app.Runtime) {
			if err := os.Remove(rt.PurgeKeyPath()); err != nil {
				t.Fatal(err)
			}
		}},
		{"entries deleted with the key and generation intact", func(t *testing.T, rt *app.Runtime) {
			rewriteLedger(t, rt, func(doc map[string]any) { doc["purges"] = []string{} })
		}},
		{"well-formed entry added to the ledger", func(t *testing.T, rt *app.Runtime) {
			// the right shape but not under this ledger's key: an entry set
			// that was edited after the ledger was written
			rewriteLedger(t, rt, func(doc map[string]any) {
				doc["observations"] = []string{"hmac-sha256:" + strings.Repeat("ab", 32)}
			})
		}},
		{"revocation removed from the ledger", func(t *testing.T, rt *app.Runtime) {
			if err := rt.Forget(contracts.ProfilePersonal, "rec_keep-001", "recorded, then dropped"); err != nil {
				t.Fatal(err)
			}
			rewriteLedger(t, rt, func(doc map[string]any) { doc["revocations"] = []any{} })
		}},
		{"pending journal with a pre-purge ledger restored over it", func(t *testing.T, rt *app.Runtime) {
			pending := filepath.Join(filepath.Dir(rt.LedgerPath()), "pending", "0123456789abcdef01234567.json")
			if err := os.MkdirAll(filepath.Dir(pending), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pending, []byte(`{"schema_version":"1","records":[],"traces":[],"observations":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			// a schema-2 ledger with no purges, exactly what restoring a
			// pre-purge backup of the canonical root leaves behind
			legacy, _ := json.Marshal(map[string]any{"schema_version": "2", "revocations": []any{}, "purges": []string{}})
			if err := os.WriteFile(rt.LedgerPath(), legacy, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(rt.PurgeKeyPath()); err != nil {
				t.Fatal(err)
			}
		}},
		{"pending journal with a purge-free schema-3 ledger", func(t *testing.T, rt *app.Runtime) {
			pending := filepath.Join(filepath.Dir(rt.LedgerPath()), "pending", "89abcdef0123456789abcdef.json")
			if err := os.MkdirAll(filepath.Dir(pending), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pending, []byte(`{"schema_version":"1","records":[],"traces":[],"observations":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			rewriteLedger(t, rt, func(doc map[string]any) { doc["purges"] = []string{} })
		}},
		{"pending journal directory unreadable", func(t *testing.T, rt *app.Runtime) {
			if runtime.GOOS == "windows" {
				t.Skip("directory permissions do not block reads on Windows")
			}
			if os.Geteuid() == 0 {
				t.Skip("root bypasses directory permissions")
			}
			pending := filepath.Join(filepath.Dir(rt.LedgerPath()), "pending")
			if err := os.MkdirAll(pending, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(pending, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(pending, 0o700) })
		}},
		{"pending journal without ledger or key", func(t *testing.T, rt *app.Runtime) {
			pending := filepath.Join(filepath.Dir(rt.LedgerPath()), "pending", "0123456789abcdef01234567.json")
			if err := os.MkdirAll(filepath.Dir(pending), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pending, []byte(`{"schema_version":"1","records":[],"traces":[],"observations":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{rt.LedgerPath(), rt.PurgeKeyPath()} {
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newPurgeFixture(t)
			rt := f.rt
			sess, err := rt.Serve(contracts.ProfilePersonal, "cap_integrity", false)
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Store.Close()
			pack, trace, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "owner preference"})
			if err != nil {
				t.Fatal(err)
			}
			persistTrace(t, rt, pack.TraceRef, trace)
			if _, err := rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
				t.Fatal(err)
			}
			// Positive control: every surface works on the intact state.
			if _, err := rt.OpenLearning(); err != nil {
				t.Fatalf("intact ledger must open the learning store: %v", err)
			}
			if _, err := rt.ExportProjection(contracts.ProfilePersonal); err != nil {
				t.Fatalf("intact ledger must export: %v", err)
			}

			c.break_(t, rt)

			checks := map[string]error{}
			_, checks["load"] = rt.LoadLedger()
			_, _, checks["resolve"] = sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "owner preference"})
			_, checks["visible records"] = sess.VisibleRecords()
			_, checks["status count"] = sess.VisibleCount()
			_, checks["export"] = rt.ExportProjection(contracts.ProfilePersonal)
			_, checks["expand"] = sess.ExpandItem(pack.PackID, "rec_keep-001")
			_, checks["explain"] = rt.LoadTrace(contracts.ProfilePersonal, pack.TraceRef)
			_, checks["doctor projection findings"] = rt.ProjectionFindings(contracts.ProfilePersonal)
			_, checks["doctor observation findings"] = rt.ObservationFindings()
			_, checks["learning list/inspect/review/feedback"] = rt.OpenLearning()
			_, checks["rebuild"] = rt.BuildProfile(contracts.ProfilePersonal)
			_, checks["purge"] = rt.PhysicalPurge(purgeReq("rec_keep-001"))
			checks["forget"] = rt.Forget(contracts.ProfilePersonal, "rec_keep-001", "must not write over an unverified ledger")
			for surface, err := range checks {
				if !errors.Is(err, app.ErrLedgerUnusable) {
					t.Errorf("%s did not fail closed: %v", surface, err)
				}
			}
		})
	}
}

// TestLedgerRemovedTogetherIsUndetectable documents the local detection
// boundary: removing the ledger, the key, and every pending journal together
// is indistinguishable from a fresh deployment.
func TestLedgerRemovedTogetherIsUndetectable(t *testing.T) {
	f := newPurgeFixture(t)
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{f.rt.LedgerPath(), f.rt.PurgeKeyPath()} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.rt.LoadLedger(); err != nil {
		t.Fatalf("with all enforcement state removed the deployment looks fresh (documented boundary); got %v", err)
	}
}

func keyFileState(t *testing.T, rt *app.Runtime) (key string, generation float64, committed bool) {
	t.Helper()
	var doc map[string]any
	readJSONFile(t, rt.PurgeKeyPath(), &doc)
	return doc["key"].(string), doc["generation"].(float64), doc["committed"].(bool)
}

// TestInterruptedFirstLedgerWriteRecovers: a crash after the key is created
// but before the first ledger write commits leaves a recognizable state that
// loads, reuses the key, and completes on retry.
func TestInterruptedFirstLedgerWriteRecovers(t *testing.T) {
	f := newPurgeFixture(t)
	injected := errors.New("injected crash before the ledger reached disk")
	restore := durable.SetFlushHooks(nil, func(file *os.File) error {
		if strings.Contains(filepath.Base(file.Name()), "tombstones.json") {
			return injected
		}
		return file.Sync()
	})
	_, err := f.rt.PhysicalPurge(purgeReq("rec_can-001"))
	restore()
	if !errors.Is(err, injected) {
		t.Fatalf("fixture: the first ledger write must fail; got %v", err)
	}
	if _, err := os.Stat(f.rt.LedgerPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fixture: the ledger must be absent after the interrupted first write")
	}
	key, _, committed := keyFileState(t, f.rt)
	if committed {
		t.Fatal("fixture: the key must be uncommitted after the interrupted first write")
	}
	if _, err := f.rt.LoadLedger(); err != nil {
		t.Fatalf("an interrupted first write must be recoverable: %v", err)
	}
	if text, _ := resolvedText(t, f.rt); !strings.Contains(text, purgeCanary) {
		t.Fatal("nothing may be erased before the ledger commits")
	}
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatalf("retry must complete: %v", err)
	}
	after, generation, committed := keyFileState(t, f.rt)
	if after != key || !committed || generation < 1 {
		t.Fatalf("retry must reuse and commit the key: same=%v committed=%v generation=%v", after == key, committed, generation)
	}
	if text, _ := resolvedText(t, f.rt); strings.Contains(text, purgeCanary) {
		t.Fatal("purged content resolves after recovery")
	}
}

// TestCrashBeforeKeyCommitKeepsEnforcement: a crash after the ledger write
// but before the key records its generation leaves a ledger newer than the
// key, which enforces normally.
func TestCrashBeforeKeyCommitKeepsEnforcement(t *testing.T) {
	f := newPurgeFixture(t)
	injected := errors.New("injected crash before the key commit")
	keyWrites := 0
	restore := durable.SetFlushHooks(nil, func(file *os.File) error {
		if strings.Contains(filepath.Base(file.Name()), "purge.key") {
			keyWrites++
			if keyWrites == 2 {
				return injected
			}
		}
		return file.Sync()
	})
	_, err := f.rt.PhysicalPurge(purgeReq("rec_can-001"))
	restore()
	if !errors.Is(err, injected) {
		t.Fatalf("fixture: the key commit must fail; got %v", err)
	}
	if _, _, committed := keyFileState(t, f.rt); committed {
		t.Fatal("fixture: the key must still be uncommitted")
	}
	if text, _ := resolvedText(t, f.rt); strings.Contains(text, purgeCanary) {
		t.Fatal("a committed ledger must enforce even before the key records its generation")
	}
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatalf("retry must complete: %v", err)
	}
	var ledger struct {
		Generation float64 `json:"generation"`
	}
	readJSONFile(t, f.rt.LedgerPath(), &ledger)
	if _, generation, committed := keyFileState(t, f.rt); !committed || generation != ledger.Generation {
		t.Fatalf("retry must commit the key at the ledger generation: committed=%v key=%v ledger=%v", committed, generation, ledger.Generation)
	}
}

// TestLearningSurfacesRequireVerifiedLedger: the positive control for the
// learning store's fail-closed path on a fresh deployment.
func TestLearningSurfacesRequireVerifiedLedger(t *testing.T) {
	f := newPurgeFixture(t)
	store, err := f.rt.OpenLearning()
	if err != nil {
		t.Fatalf("a fresh deployment must open the learning store: %v", err)
	}
	if _, err := store.Observe("observation", "fresh deployment observation", "fam", "personal", "personal_private", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(f.rt.LedgerPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.rt.LedgerPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.OpenLearning(); !errors.Is(err, app.ErrLedgerUnusable) {
		t.Fatalf("learning surfaces must fail closed on a corrupt ledger; got %v", err)
	}
	// The raw store stays openable for purge inspection — and still holds the
	// observation, which is why the filtered surface must be the one callers
	// use: an unverifiable ledger must not silently downgrade to raw reads.
	raw, err := learning.Open(f.rt.Config.DataDir)
	if err != nil {
		t.Fatalf("the raw store (purge inspection only) stays openable: %v", err)
	}
	if all, err := raw.ListAll(); err != nil || len(all) != 1 {
		t.Fatalf("fixture: the raw store must still hold the observation the filtered surface refuses to serve: %d %v", len(all), err)
	}
}
