package privacycorpus_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/0merUfuk/beme/internal/privacycorpus"
)

// TestPrivacyCorpusDeterministic executes the shared registry (the same one
// cmd/beme-threat-corpus runs) and requires 100% acceptance with every §19
// case and every supplementary case present and executed.
func TestPrivacyCorpusDeterministic(t *testing.T) {
	suite, err := privacycorpus.NewSuite(t.TempDir(), privacycorpus.Options{RepoRoot: repoRoot(t)})
	if err != nil {
		t.Fatal(err)
	}
	results := suite.Run()

	want := map[string]bool{}
	for i := 1; i <= 30; i++ {
		want[strconv.Itoa(i)] = true
	}
	for _, id := range privacycorpus.SupplementaryCases {
		want[id] = true
	}
	passed, failed, notRun := 0, []string{}, []string{}
	for _, r := range results {
		if !want[r.Case] {
			t.Errorf("unexpected case %q in registry", r.Case)
		}
		delete(want, r.Case)
		switch r.State {
		case "passed":
			passed++
		case "failed":
			failed = append(failed, fmt.Sprintf("%s (P-group %s): %s", r.Case, r.Group, r.Reason))
		case "not_run":
			notRun = append(notRun, fmt.Sprintf("%s: %s", r.Case, r.Reason))
		}
	}
	t.Logf("privacy corpus: %d passed, %d failed, %d not_run (of %d)", passed, len(failed), len(notRun), len(results))
	for _, f := range failed {
		t.Errorf("THREAT CASE FAILED — %s", f)
	}
	if len(failed) > 0 {
		t.Fatalf("privacy invariant acceptance must be 100%%; %d failing", len(failed))
	}
	if len(want) > 0 {
		t.Fatalf("cases missing from the registry: %v", want)
	}
	if len(notRun) > 0 {
		t.Fatalf("with a repository checkout every case must execute; not_run: %v", notRun)
	}
}

func repoRoot(t *testing.T) string {
	// internal/privacycorpus → repo root is ../..
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
