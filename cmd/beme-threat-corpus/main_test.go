package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

type runnerReport struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	NotRun  int `json:"not_run"`
	Results []struct {
		Case  string `json:"case"`
		State string `json:"state"`
	} `json:"results"`
}

// TestThreatCorpusRunner proves the runner executes the shared registry with
// the documented exit-code contract: 0 when every case passes (repository
// checkout available), 3 when a process-gate case cannot run.
func TestThreatCorpusRunner(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"--json", "--repo", root, "--work", t.TempDir()}, &out, &errb); code != 0 {
		t.Fatalf("with a checkout the runner must exit 0; got %d\n%s%s", code, out.String(), errb.String())
	}
	var rep runnerReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Failed != 0 || rep.NotRun != 0 || rep.Passed != len(rep.Results) || len(rep.Results) < 33 {
		t.Fatalf("unexpected report: passed=%d failed=%d not_run=%d results=%d", rep.Passed, rep.Failed, rep.NotRun, len(rep.Results))
	}

	out.Reset()
	if code := run([]string{"--json", "--work", t.TempDir()}, &out, &errb); code != 3 {
		t.Fatalf("without a checkout the runner must exit 3 (not_run), got %d", code)
	}
	rep = runnerReport{}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.NotRun != 1 || rep.Failed != 0 {
		t.Fatalf("without a checkout exactly case 19 is not_run: %+v", rep)
	}
	for _, r := range rep.Results {
		if r.State == "not_run" && r.Case != "19" {
			t.Fatalf("unexpected not_run case %s", r.Case)
		}
	}

	if code := run([]string{"--bogus"}, &out, &errb); code != 2 {
		t.Fatalf("bad flags must exit 2, got %d", code)
	}
}
