package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBenchSurfacesCleanupFailure pins finding 4: when removing the default
// temporary work dir fails, the failure is reported on stderr and a run that
// would otherwise exit 0 exits 1. A run that already failed keeps its code and
// still reports the cleanup failure. A caller-supplied --work is never
// removed. The report on stdout proves the run reached the cleanup path.
func TestBenchSurfacesCleanupFailure(t *testing.T) {
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	var removed []string
	removeAll = func(p string) error {
		removed = append(removed, p)
		_ = os.RemoveAll(p) // keep the test sandbox tidy; the failure is what is under test
		return errors.New("synthetic cleanup failure")
	}
	t.Cleanup(func() { removeAll = os.RemoveAll })

	seed := filepath.Join("..", "..", "testdata", "synthetic", "records.json")
	small := []string{"--scales", "1", "--iterations", "1", "--warmup", "0", "--build-runs", "1"}

	var stdout, stderr bytes.Buffer
	code := run(append([]string{"--seed", seed}, small...), &stdout, &stderr)
	if !strings.Contains(stdout.String(), `"seed_records": 5`) {
		t.Fatalf("positive control: the run did not complete and write its report: %s", stderr.String())
	}
	if code != 1 {
		t.Errorf("cleanup failure after a successful run: exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "cleanup of temporary work dir failed") || !strings.Contains(stderr.String(), "synthetic cleanup failure") {
		t.Errorf("cleanup failure not reported on stderr: %s", stderr.String())
	}
	if len(removed) != 1 || !strings.HasPrefix(filepath.Base(removed[0]), "beme-bench-") {
		t.Fatalf("cleanup did not target the temp work dir: %v", removed)
	}

	stderr.Reset()
	code = run(append([]string{"--seed", filepath.Join(t.TempDir(), "missing.json")}, small...), &bytes.Buffer{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "seed corpus") || !strings.Contains(stderr.String(), "cleanup of temporary work dir failed") {
		t.Errorf("failed run with failed cleanup: exit %d, stderr=%s", code, stderr.String())
	}

	removed = nil
	stderr.Reset()
	work := filepath.Join(t.TempDir(), "work")
	if code := run(append([]string{"--seed", seed, "--work", work}, small...), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("--work run: exit %d, stderr=%s", code, stderr.String())
	}
	if len(removed) != 0 {
		t.Errorf("a caller-supplied --work dir must not be removed: %v", removed)
	}
}
