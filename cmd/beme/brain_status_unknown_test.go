package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The operator view must never print zero live notes for a scan it could not
// complete. The scan entry bound (4096) is exceeded without timing: each type
// directory stays within the per-directory bound, the total does not.
func TestBrainStatusPrintsUnknownForIncompleteLiveScan(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	r := newBrainRig(t)
	proj := filepath.Join(r.home, "proj")
	os.MkdirAll(proj, 0o755)
	r.mustRun("", "brain", "init")
	r.mustRun("", "brain", "project", "add", "demo", "--root", proj)
	vault := filepath.Join(r.cfg, "data", "brain", "projects", "demo")
	for dir, n := range map[string]int{"learnings": 2048, "open": 2050} {
		d := filepath.Join(vault, dir)
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < n; i++ {
			if err := os.WriteFile(filepath.Join(d, fmt.Sprintf("filler-%04d.md", i)), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	out := r.mustRun("", "brain", "status")
	if !strings.Contains(out, "live notes:   unknown (") || strings.Contains(out, "decision 0") {
		t.Fatalf("an incomplete scan must print unknown, not zero counts:\n%s", out)
	}
	var st struct {
		Live          map[string]int `json:"live_by_type"`
		LiveUnknown   bool           `json:"live_unknown"`
		UnknownReason string         `json:"live_unknown_reason"`
	}
	if err := json.Unmarshal([]byte(r.mustRun("", "brain", "status", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	if !st.LiveUnknown || st.UnknownReason == "" || st.Live != nil {
		t.Fatalf("JSON must mark live counts unknown and claim none: %+v", st)
	}
}
