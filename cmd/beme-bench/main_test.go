package main

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }

// TestBenchCleansUpOnEveryExit: the default temp work dir is removed even when
// the run fails, and a failed report write is a non-zero exit.
func TestBenchCleansUpOnEveryExit(t *testing.T) {
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	var stderr bytes.Buffer
	if code := run([]string{"--seed", "/nonexistent/seed.json"}, &bytes.Buffer{}, &stderr); code != 1 {
		t.Fatalf("missing seed must exit 1, got %d (%s)", code, stderr.String())
	}
	if code := run([]string{"--seed", "../../testdata/synthetic/records.json", "--scales", "1", "--iterations", "1", "--warmup", "0", "--build-runs", "1"}, failingWriter{}, &stderr); code != 1 {
		t.Fatalf("a failed report write must exit 1, got %d", code)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("temporary work dir left behind: %s", e.Name())
	}
}
