// Command beme-threat-corpus runs the deterministic privacy/policy threat
// corpus — every blueprint §19 case plus the supplementary purge-reliability
// cases — and reports passed | failed | not_run per case. It executes the same
// registry as TestPrivacyCorpusDeterministic (privacycorpus.NewSuite), on
// disposable synthetic deployments only; it never reads or writes real user
// data.
//
//	go run ./cmd/beme-threat-corpus --repo . [--json]
//
// Exit codes: 0 all passed · 1 at least one failed · 2 usage · 3 no failures
// but at least one not_run (never a silent pass).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/0merUfuk/beme/internal/privacycorpus"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("beme-threat-corpus", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", "", "repository checkout for process-gate cases (empty: those cases are not_run)")
	work := fs.String("work", "", "work dir for disposable deployments (default: a temp dir removed afterwards)")
	jsonOut := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := *work
	if dir == "" {
		tmp, err := os.MkdirTemp("", "beme-threat-corpus-")
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		defer os.RemoveAll(tmp)
		dir = tmp
	}

	suite, err := privacycorpus.NewSuite(dir, privacycorpus.Options{RepoRoot: *repo})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	results := suite.Run()
	passed, failed, notRun := 0, 0, 0
	for _, r := range results {
		switch r.State {
		case "passed":
			passed++
		case "failed":
			failed++
		default:
			notRun++
		}
	}

	if *jsonOut {
		json.NewEncoder(stdout).Encode(map[string]any{
			"passed": passed, "failed": failed, "not_run": notRun, "results": results,
		})
	} else {
		for _, r := range results {
			line := fmt.Sprintf("%-8s %-4s %s", r.State, r.Case, r.Group)
			if r.Reason != "" {
				line += "  — " + r.Reason
			}
			fmt.Fprintln(stdout, line)
		}
		fmt.Fprintf(stdout, "threat corpus: %d passed, %d failed, %d not_run (of %d)\n", passed, failed, notRun, len(results))
	}
	switch {
	case failed > 0:
		return 1
	case notRun > 0:
		return 3
	}
	return 0
}
