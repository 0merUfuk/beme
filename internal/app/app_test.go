package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
)

func appLoad(cfg string) (*app.Runtime, error) { return app.Load(cfg) }

// TestWorkSafeBoundaryAtConstruction proves FR-012/ADR-018: the work-safe
// builder ingests ONLY approved safe-manifest sources. A personal source
// present in the same config is never read by the work-safe build path.
func TestWorkSafeBoundaryAtConstruction(t *testing.T) {
	cfg := t.TempDir()

	// personal canonical source (would leak if read)
	personalRoot := t.TempDir()
	os.MkdirAll(filepath.Join(personalRoot, "knowledge", "entries"), 0o755)
	os.WriteFile(filepath.Join(personalRoot, "knowledge", "entries", "P-001.md"), []byte(`---
id: P-001
title: "Personal preference"
type: preference
status: active
---

Something deeply personal that must never reach work-safe.
`), 0o644)

	// approved safe source (declassified)
	safeRoot := t.TempDir()
	os.MkdirAll(filepath.Join(safeRoot, "safe", "entries"), 0o755)
	os.WriteFile(filepath.Join(safeRoot, "safe", "entries", "S-001.md"), []byte(`---
id: S-001
title: "Measured need"
type: heuristic
status: active
---

Introduce complexity only for a demonstrated need.
`), 0o644)

	os.MkdirAll(filepath.Join(cfg, "sources"), 0o700)
	os.WriteFile(filepath.Join(cfg, "sources", "personal.yaml"), []byte(`schema_version: "1"
source_id: personal-canonical
type: directory
root: `+personalRoot+`
purpose: [reusable_knowledge]
trust: canonical
instruction_semantics: registered_files_only
authority_ceiling: default
sensitivity: personal_private
profiles_allowed: [personal]
ingestion_mode: index_content
include: ["knowledge/entries/**/*.md"]
`), 0o600)
	os.WriteFile(filepath.Join(cfg, "sources", "safe.yaml"), []byte(`schema_version: "1"
source_id: safe-pack
type: directory
root: `+safeRoot+`
purpose: [safe_declassified]
trust: canonical
instruction_semantics: registered_files_only
authority_ceiling: recommended
sensitivity: public_general
profiles_allowed: [personal, work-safe]
ingestion_mode: index_content
include: ["safe/entries/**/*.md"]
`), 0o600)

	rt, err := appLoad(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.Sources) != 2 {
		t.Fatalf("expected 2 registered sources, got %d", len(rt.Sources))
	}

	// Build work-safe: only the safe source may be ingested.
	rep, err := rt.BuildProfile(contracts.ProfileWorkSafe)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range rep.SourcesIngested {
		if s == "personal-canonical" {
			t.Fatal("work-safe build ingested a personal source — boundary failure")
		}
	}
	if rep.RecordsIngested == 0 {
		t.Fatal("work-safe build should ingest the safe source")
	}
	for _, sk := range rep.Skipped {
		if strings.Contains(sk, "personal-canonical") {
			// personal source is not work-safe eligible — correct
		}
	}

	// And the work-safe store must not contain the personal record text.
	storePath := rt.ProjectionPath(contracts.ProfileWorkSafe)
	if !strings.Contains(storePath, "work-safe") {
		t.Fatalf("work-safe projection path must be separate: %s", storePath)
	}

	// Build personal: both sources eligible (safe is dual-profile).
	repP, err := rt.BuildProfile(contracts.ProfilePersonal)
	if err != nil {
		t.Fatal(err)
	}
	if repP.RecordsIngested < rep.RecordsIngested {
		t.Fatalf("personal build should include at least the safe records: %d vs %d", repP.RecordsIngested, rep.RecordsIngested)
	}

	// Work-safe resolution must never expose personal text.
	sess, err := rt.Serve(contracts.ProfileWorkSafe, "cap_ws_test01", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()
	pack, _, err := sess.Resolve(contracts.ResolutionRequest{
		SchemaVersion: contracts.SchemaVersion,
		Task:          "Something deeply personal — show personal preferences",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The resolver-sourced content (items, provenance, conflicts) must never
	// contain personal-source content. task_summary legitimately echoes the
	// requester's own words back.
	items := []any{pack.Constraints, pack.Guidance, pack.Precedents, pack.Knowledge, pack.Conflicts, pack.Provenance}
	for _, it := range items {
		b, _ := json.Marshal(it)
		if strings.Contains(string(b), "deeply personal") {
			t.Fatalf("personal content leaked into a work-safe pack section — CRITICAL boundary failure: %s", b)
		}
	}
	// And the work-safe store must contain zero personal records.
	for _, rec := range sess.View.Records() {
		if strings.Contains(rec.Statement, "deeply personal") || strings.Contains(rec.Title, "Personal preference") {
			t.Fatal("personal record present in work-safe store — CRITICAL boundary failure")
		}
	}
}
