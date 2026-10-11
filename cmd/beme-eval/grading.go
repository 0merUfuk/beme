package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/0merUfuk/beme/internal/evalrunner"
)

type gradePaths []string

func (p *gradePaths) String() string     { return strings.Join(*p, ",") }
func (p *gradePaths) Set(v string) error { *p = append(*p, v); return nil }

func gradesCommand(command string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundle := fs.String("bundle", "", "immutable generation bundle directory")
	out := fs.String("out", "", "new template file or grade-result parent directory (outside repository and bundle)")
	evaluator := fs.String("evaluator", "", "independent reviewer identifier (template only)")
	adjudication := fs.String("adjudication", "", "independent adjudication document binding all review hashes")
	jsonOut := fs.Bool("json", false, "JSON output")
	var paths gradePaths
	fs.Var(&paths, "grades", "review document; repeat flag for multiple independent reviewers")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *bundle == "" {
		fmt.Fprintln(stderr, "error: --bundle required; no positional arguments")
		return 2
	}
	if *out != "" {
		if err := evalrunner.CheckOutputOutsideBundle(*out, *bundle); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	}
	if command == "grade-template" || command == "reviewer-packet" {
		if *out == "" || *evaluator == "" {
			fmt.Fprintln(stderr, "error: --out FILE and --evaluator ID required")
			return 2
		}
		dir, err := resolveOut(filepath.Dir(*out))
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		var g any
		if command == "reviewer-packet" {
			g, err = evalrunner.ExportReviewerPacket(*bundle, *evaluator)
		} else {
			g, err = evalrunner.GradeTemplate(*bundle, *evaluator)
		}
		if err != nil {
			fmt.Fprintf(stderr, "error: grades: %v\n", err)
			return 1
		}
		path := filepath.Join(dir, filepath.Base(*out))
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		err = enc.Encode(g)
		if err == nil {
			err = f.Sync()
		}
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if *jsonOut {
			json.NewEncoder(stdout).Encode(map[string]any{"template": path, "export_type": command, "status": "grading_pending"})
		} else {
			fmt.Fprintf(stdout, "grading_pending: %s %s; no judgments prefilled; share only a reviewer-packet export, never the owner bundle\n", command, path)
		}
		return 0
	}
	if len(paths) == 0 {
		fmt.Fprintln(stderr, "error: --grades required")
		return 2
	}
	dir, err := resolveOut(*out)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	result, err := evalrunner.ImportGrades(*bundle, paths, *adjudication, dir)
	if err != nil {
		fmt.Fprintf(stderr, "error: grades: %v\n", err)
		return 1
	}
	if *jsonOut {
		json.NewEncoder(stdout).Encode(map[string]any{"exit_code": result.ExitCode(), "result": result})
	} else {
		for _, g := range result.Gates {
			fmt.Fprintf(stdout, "%s %s: %s\n", g.Outcome, g.Name, g.Reason)
		}
		fmt.Fprintf(stdout, "outcome: %s; benefit: %s; class: %s\nartifacts: %s\n", result.Outcome, result.Benefit, result.EvidenceClass, result.Artifacts)
	}
	return result.ExitCode()
}
