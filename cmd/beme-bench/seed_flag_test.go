package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBenchRequiresExplicitSeed pins finding 3: there is no repository-
// relative default seed path. A missing or empty --seed is a usage error
// (exit 2) with a message that names the flag, and nothing runs. The positive
// control proves the same invocation succeeds once --seed is given, from a
// working directory that is not the repository.
func TestBenchRequiresExplicitSeed(t *testing.T) {
	seed, err := filepath.Abs(filepath.Join("..", "..", "testdata", "synthetic", "records.json"))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	t.Chdir(t.TempDir()) // an installed binary runs outside the repository

	small := []string{"--scales", "1", "--iterations", "1", "--warmup", "0", "--build-runs", "1"}
	for name, args := range map[string][]string{
		"omitted":     small,
		"empty":       append([]string{"--seed", ""}, small...),
		"blank":       append([]string{"--seed", "  "}, small...),
		"equals form": append([]string{"--seed="}, small...),
	} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code != 2 {
			t.Errorf("%s --seed: exit %d, want 2 (usage error); stderr=%s", name, code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "--seed is required") {
			t.Errorf("%s --seed: stderr does not explain the missing flag: %s", name, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("%s --seed: a report was written: %s", name, stdout.String())
		}
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("usage error created a work dir: %v", entries)
	}

	var stdout, stderr bytes.Buffer
	if code := run(append([]string{"--seed", seed}, small...), &stdout, &stderr); code != 0 {
		t.Fatalf("explicit --seed outside the repository: exit %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"seed_records": 5`) {
		t.Fatalf("report missing seed_records: %s", stdout.String())
	}
}

// TestBenchReportFileWriteFailureExitsNonZero keeps the output-write contract
// for --out files (the stdout case is in main_test.go): exit 1, a clear
// message, and the temp work dir still removed.
func TestBenchReportFileWriteFailureExitsNonZero(t *testing.T) {
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	out := filepath.Join(t.TempDir(), "missing-dir", "report.json")
	var stderr bytes.Buffer
	code := run([]string{"--seed", filepath.Join("..", "..", "testdata", "synthetic", "records.json"), "--scales", "1", "--iterations", "1", "--warmup", "0", "--build-runs", "1", "--out", out}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("unwritable --out: exit %d, want 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "write report") {
		t.Fatalf("stderr does not report the write failure: %s", stderr.String())
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("temporary work dir left behind: %v", entries)
	}
}
