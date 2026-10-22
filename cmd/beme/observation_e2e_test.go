package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRestoredObservationHiddenOnCLI drives the real binary: an observation
// restating a record is purged with it, a pre-purge copy of its file is
// restored, and candidate list/inspect/review and doctor never surface it;
// build erases it; an unverifiable ledger blocks the candidate surface.
func TestRestoredObservationHiddenOnCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	bin := buildBemeBinary(t, "beme-observations")
	cfg, rt := diagnosticsDeployment(t)
	const id = "obs_e2e00000000000001"
	const hypothesis = "Diagnostics fixture statement."
	obsFile := filepath.Join(rt.Config.DataDir, "observations", id+".json")
	if err := os.MkdirAll(filepath.Dir(obsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	obs := []byte(`{"schema_version":"1","observation_id":"` + id + `","kind":"observation","hypothesis":"` + hypothesis + `","evidence_family":"fam","family_count":1,"source_profile":"personal","inherited_sensitivity":"personal_private","first_observed_at":"2026-01-01T00:00:00Z","last_observed_at":"2026-01-01T00:00:00Z","status":"quarantined"}`)
	if err := os.WriteFile(obsFile, obs, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runBeme(t, bin, "build", "--profile", "personal", "--config", cfg); code != 0 {
		t.Fatalf("build: %d %s", code, stderr)
	}
	if out, _, code := runBeme(t, bin, "candidate", "list", "--json", "--config", cfg); code != 0 || !strings.Contains(out, id) {
		t.Fatalf("positive control: the observation must be listed before purge: %d %s", code, out)
	}
	if _, stderr, code := runBeme(t, bin, "purge", "--config", cfg, "--confirm", "rec_diag-001", "rec_diag-001"); code != 0 {
		t.Fatalf("purge: %d %s", code, stderr)
	}
	if _, err := os.Stat(obsFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fixture: purge must remove the restating observation")
	}
	if err := os.WriteFile(obsFile, obs, 0o600); err != nil {
		t.Fatal(err)
	}

	out, stderr, code := runBeme(t, bin, "candidate", "list", "--json", "--config", cfg)
	var listed struct {
		Count int `json:"count"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || listed.Count != 0 || strings.Contains(out+stderr, id) {
		t.Fatalf("candidate list surfaces a restored purged observation: %d %s %s", code, out, stderr)
	}
	for name, args := range map[string][]string{
		"inspect": {"candidate", "inspect", id, "--json", "--config", cfg},
		"review":  {"candidate", "review", id, "--action", "defer", "--config", cfg},
	} {
		out, stderr, code := runBeme(t, bin, args...)
		if code == 0 || strings.Contains(out+stderr, hypothesis) {
			t.Fatalf("candidate %s must refuse a restored purged observation without content: %d %s %s", name, code, out, stderr)
		}
	}
	if after, err := os.ReadFile(obsFile); err != nil || !bytes.Equal(after, obs) {
		t.Fatalf("a refused review must leave the hidden observation byte-identical: err=%v changed=%v", err, !bytes.Equal(after, obs))
	}
	out, _, _ = runBeme(t, bin, "doctor", "--json", "--config", cfg)
	var doc struct {
		Status   string   `json:"status"`
		Findings []string `json:"findings"`
	}
	if json.Unmarshal([]byte(out), &doc) != nil || doc.Status != "degraded" || !strings.Contains(strings.Join(doc.Findings, "\n"), "purged observation") || strings.Contains(out, id) {
		t.Fatalf("doctor must report the restored purged observation without its ID: %s", out)
	}
	if _, stderr, code := runBeme(t, bin, "build", "--profile", "personal", "--config", cfg); code != 0 {
		t.Fatalf("build: %d %s", code, stderr)
	}
	if _, err := os.Stat(obsFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("build must erase the restored purged observation")
	}

	// A --config flag without a directory must be a usage error, never a
	// silent fallback to the operator's real deployment.
	for _, args := range [][]string{
		{"candidate", "list", "--config"},
		{"candidate", "list", "--config="},
	} {
		out, stderr, code := runBeme(t, bin, args...)
		if code != 2 || !strings.Contains(stderr, "--config requires a directory") {
			t.Fatalf("%v must exit 2 with a usage error; got %d %s %s", args, code, out, stderr)
		}
	}

	if err := os.Remove(rt.LedgerPath()); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runBeme(t, bin, "candidate", "list", "--config", cfg); code != 3 || !strings.Contains(stderr, "policy blocked") {
		t.Fatalf("candidate surface must fail closed on an unverifiable ledger: %d %s", code, stderr)
	}
}
