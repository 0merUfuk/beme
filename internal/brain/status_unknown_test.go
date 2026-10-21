package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// An incomplete live-note scan must be reported as unknown, never as zero
// counts. The entry limit is exceeded deterministically (no timing): each type
// directory stays within the per-directory bound, the total does not.
func TestStatusReportsUnknownWhenLiveScanIsIncomplete(t *testing.T) {
	v, _ := newVault(t)
	mustCapture(t, v, CaptureInput{Type: TypeDecision, Title: "Use SQLite for local state", Statement: "Local state lives in one SQLite file.", Why: "Zero-ops and offline-first."}, t0)
	mustDoctor(t, v, t0)
	if st := v.Status(); st.LiveUnknown || st.Live["decision"] != 1 {
		t.Fatalf("a complete scan must count the live note: %+v", st)
	}

	for typ, n := range map[Type]int{TypeLearning: maxScanEntries / 2, TypeOpen: maxScanEntries/2 + 2} {
		dir := v.liveDir("demo", typ)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < n; i++ {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("filler-%04d.md", i)), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	st := v.Status()
	if !st.LiveUnknown || st.LiveUnknownReason == "" {
		t.Fatalf("an incomplete scan must be reported as unknown: %+v", st)
	}
	if st.Live != nil {
		t.Fatalf("an incomplete scan must claim no counts, got %v", st.Live)
	}
}
