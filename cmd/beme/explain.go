package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
)

// explainCmd implements `beme context explain --trace <id>` (blueprint
// §12.2, FR-038): explain output is generated from the ACTUAL resolver
// trace created during resolution, never an LLM-generated retrospective.
//
// v1 trace persistence: traces are written to the cache dir as JSON at
// resolution time (preview/resolve and MCP resolve_context both persist
// them); explain loads one by trace id. Work-safe explain output cannot
// reveal denied record names, raw paths, or personal evidence (FR-039):
// trace steps carry only stage names, record IDs of SELECTED items, and
// exclusion REASONS (which are class-level, e.g. "sensitivity denied",
// never content).
func explainCmd(rt *app.Runtime, traceID, profile string, jsonOut bool) {
	dir := filepath.Join(rt.Config.CacheDir, "traces")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	path := filepath.Join(dir, strings.TrimPrefix(traceID, "trace_")+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		// Not-found and unreadable are indistinguishable by design.
		fmt.Fprintln(os.Stderr, "error: trace not available")
		os.Exit(4)
	}
	var steps []resolver.TraceStep
	if err := json.Unmarshal(data, &steps); err != nil {
		fmt.Fprintln(os.Stderr, "error: trace unreadable")
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

// exportCmd implements `beme export` (blueprint §12.2): a bounded, JSON
// export of a projection's normalized records + provenance. The export is
// derived data, never canonical, and carries its sensitivity labels so a
// downstream consumer can honor them. Secrets (secret_never_ingest) are
// never exportable (they were never ingested).
func exportCmd(rt *app.Runtime, profile, outPath string, jsonOut bool) {
	store, err := app.OpenStoreForProfile(rt, contracts.Profile(profile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	type exportRecord struct {
		Record     contracts.Record       `json:"record"`
		Provenance []contracts.Provenance `json:"provenance"`
	}
	out := []exportRecord{}
	for _, rec := range store.Records() {
		er := exportRecord{Record: rec}
		for _, pid := range rec.ProvenanceRefs {
			if p, ok := store.Provenance(pid); ok {
				er.Provenance = append(er.Provenance, p)
			}
		}
		out = append(out, er)
	}
	payload := map[string]any{
		"schema_version": contracts.SchemaVersion,
		"profile":        profile,
		"exported_at":    cmdNowUTC(),
		"derived_note":   "derived data; never canonical; sensitivity labels must be honored downstream",
		"records":        out,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if outPath == "-" {
		os.Stdout.Write(data)
		fmt.Println()
		return
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if !jsonOut {
		fmt.Printf("exported %d records → %s (derived data; never canonical)\n", len(out), outPath)
	}
}
