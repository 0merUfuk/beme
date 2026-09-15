package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
)

// exitForReadErr maps read-surface errors to the CLI exit contract: an
// unusable tombstone ledger or purge key is policy-blocked (3) — the command
// never falls back to unfiltered projection data.
func exitForReadErr(err error) int {
	if errors.Is(err, app.ErrLedgerUnusable) {
		return 3
	}
	return 1
}

// explainCmd implements `beme explain --trace <id>` (blueprint §12.2,
// FR-038): explain output is generated from the ACTUAL resolver trace created
// during resolution, never an LLM-generated retrospective.
//
// Traces are persisted to the cache dir at resolution time. explain loads one
// through app.LoadTrace, which drops every step naming a record that is not
// visible in the current projection (revoked, purged, or restored from a
// backup) and fails closed when the tombstone ledger is unusable. Not-found
// and rejected traces are indistinguishable (exit 4).
func explainCmd(rt *app.Runtime, traceID, profile string, jsonOut bool) {
	steps, err := rt.LoadTrace(contracts.Profile(profile), traceID)
	switch {
	case errors.Is(err, app.ErrTraceUnavailable):
		fmt.Fprintln(os.Stderr, "error: trace not available")
		os.Exit(4)
	case errors.Is(err, app.ErrLedgerUnusable):
		fmt.Fprintf(os.Stderr, "policy blocked: %v\n", err)
		os.Exit(3)
	case err != nil:
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if jsonOut {
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"trace_id": traceID,
			"steps":    steps,
		})
		return
	}
	fmt.Printf("trace %s (%d steps, from the actual resolver run):\n", traceID, len(steps))
	for i, s := range steps {
		line := fmt.Sprintf("  %2d. %-12s %s", i+1, s.Step, s.Outcome)
		if s.RecordID != "" {
			line += "  [" + s.RecordID + "]"
		}
		fmt.Println(line)
		for _, r := range s.Reasons {
			fmt.Printf("        reason: %s\n", r)
		}
	}
}

// PersistTrace writes a trace to the cache for explain (bounded retention:
// keep the newest 200; traces are derived cache, always discardable).
func PersistTrace(cacheDir, traceRef string, steps []resolver.TraceStep) {
	dir := filepath.Join(cacheDir, "traces")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return // trace persistence is best-effort; resolution never fails on it
	}
	id := strings.TrimPrefix(traceRef, "trace_")
	data, err := json.Marshal(steps)
	if err != nil {
		return
	}
	tmp := filepath.Join(dir, id+".tmp")
	if os.WriteFile(tmp, data, 0o600) != nil {
		return
	}
	os.Rename(tmp, filepath.Join(dir, id+".json"))
	pruneTraces(dir, 200)
}

func pruneTraces(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	names := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // trace ids sort chronologically (time-based ids)
	for len(names) > keep {
		os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

// exportCmd implements `beme export` (blueprint §12.2): a bounded JSON export
// of a projection's normalized records + provenance. It reads through
// app.ExportProjection, so revoked and purged records (including ones in a
// restored backup) never appear, and an unusable ledger fails closed.
func exportCmd(rt *app.Runtime, profile, outPath string, jsonOut bool) {
	exp, err := rt.ExportProjection(contracts.Profile(profile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitForReadErr(err))
	}
	payload := map[string]any{
		"schema_version": contracts.SchemaVersion,
		"profile":        exp.Profile,
		"exported_at":    cmdNowUTC(),
		"derived_note":   "derived data; never canonical; sensitivity labels must be honored downstream",
		"records":        exp.Records,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if outPath == "-" {
		if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
			fmt.Fprintf(os.Stderr, "error: write export: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if !jsonOut {
		fmt.Printf("exported %d records → %s (derived data; never canonical)\n", len(exp.Records), outPath)
	}
}
