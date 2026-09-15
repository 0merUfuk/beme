// Command beme-eval is the owner procedure for running the evaluation runner
// (internal/evalrunner) against a real deployment and a golden-case corpus.
// It is never run in CI against private data: CI exercises it only on the
// public synthetic fixtures in its tests.
//
//	beme-eval retrieval  --corpus DIR --config DIR [--out DIR] [--json]
//	beme-eval behavioral --corpus DIR --config DIR --provider command --command 'CMD ARGS'
//	                     [--arms LIST] [--repeats N] [--dry-run] [--out DIR] [--json]
//
// retrieval measures deterministic, local retrieval recall/precision of the
// full Be Me pack against each case's required_evidence_refs. behavioral
// renders each arm's prompt and pipes it to an external command's stdin (no
// shell), reading the generation from stdout; --dry-run renders prompts and
// manifests and reports how many generations would run without executing
// the command.
//
// The corpus comes from --corpus or BEME_PRIVATE_EVAL_DIR; with neither the
// command reports not_run and exits 3. Artifacts go to --out, or to a new
// directory under the OS temp dir; a directory inside the Be Me repository
// is refused.
//
// Exit codes: 0 all passed · 1 failure or blocker · 2 usage · 3 not_run.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/evalrunner"
	"github.com/0merUfuk/beme/internal/resolver"
)

const usage = `usage:
  beme-eval retrieval  --corpus DIR --config DIR [--out DIR] [--json]
  beme-eval behavioral --corpus DIR --config DIR --provider command --command 'CMD ARGS'
                       [--arms LIST] [--repeats N] [--dry-run] [--out DIR] [--json]
The corpus defaults to $BEME_PRIVATE_EVAL_DIR. Run "beme-eval <command> -h" for flags.
Exit codes: 0 passed, 1 failed, 2 usage, 3 not_run.`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "retrieval":
		return retrieval(args[1:], stdout, stderr)
	case "behavioral":
		return behavioral(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "unknown command %q\n%s\n", args[0], usage)
	return 2
}

// common holds the flags both commands share.
type common struct {
	corpus, config, out, capability, networkPolicy, split string
	learned, jsonOut                                      bool
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.corpus, "corpus", "", "golden-case corpus directory (default $BEME_PRIVATE_EVAL_DIR)")
	fs.StringVar(&c.config, "config", "", "deployment config directory (required)")
	fs.StringVar(&c.out, "out", "", "artifact directory (default: a new directory under the OS temp dir)")
	fs.StringVar(&c.capability, "capability", "cap_eval_owner", "capability ID bound for evaluation sessions")
	fs.BoolVar(&c.learned, "experimental-learned", false, "enable experimental learned guidance (personal profile only; required by learned-only)")
	fs.StringVar(&c.networkPolicy, "network-policy", "disabled", "disabled|integration_only")
	fs.StringVar(&c.split, "split", "calibration", "calibration|development|locked_holdout|privacy_red_team")
	fs.BoolVar(&c.jsonOut, "json", false, "print the summary as JSON")
}

// prepare resolves the corpus, runtime, and artifact directory. It returns
// an exit code >= 0 when the command must stop.
func (c *common) prepare(stdout, stderr io.Writer) (*evalrunner.Corpus, *app.Runtime, string, int) {
	if c.corpus == "" {
		c.corpus = os.Getenv("BEME_PRIVATE_EVAL_DIR")
	}
	if c.corpus == "" {
		fmt.Fprintln(stdout, "not_run: no corpus given (use --corpus or set BEME_PRIVATE_EVAL_DIR)")
		return nil, nil, "", 3
	}
	if c.config == "" {
		fmt.Fprintln(stderr, "error: --config is required (an explicit deployment config directory)")
		return nil, nil, "", 2
	}
	if info, err := os.Stat(c.config); err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "error: --config %s is not a directory\n", c.config)
		return nil, nil, "", 2
	}
	corpus, err := evalrunner.LoadCorpus(c.corpus)
	if err != nil {
		fmt.Fprintf(stderr, "error: corpus: %v\n", err)
		return nil, nil, "", 1
	}
	rt, err := app.Load(c.config)
	if err != nil {
		fmt.Fprintf(stderr, "error: deployment: %v\n", err)
		return nil, nil, "", 1
	}
	out, err := resolveOut(c.out)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, nil, "", 2
	}
	return corpus, rt, out, -1
}

func (c *common) inputs(rt *app.Runtime) evalrunner.InputsFn {
	return func(p contracts.Profile, task, ws string) (resolver.EvalInputs, error) {
		return rt.EvalInputs(p, c.capability, c.learned, task, ws)
	}
}

func retrieval(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("beme-eval retrieval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c common
	c.register(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "error: unexpected arguments %q\n", fs.Args())
		return 2
	}
	corpus, rt, out, code := c.prepare(stdout, stderr)
	if code >= 0 {
		return code
	}
	sum, err := evalrunner.Run(corpus, evalrunner.RunConfig{
		OutputDir: out, NetworkPolicy: c.networkPolicy, Split: c.split,
		Refs: refsLookup(rt, c.capability, c.learned),
	}, nil, nil, c.inputs(rt))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return report(stdout, sum, out, c.jsonOut)
}

func behavioral(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("beme-eval behavioral", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c common
	c.register(fs)
	provider := fs.String("provider", "", "model provider (only: command)")
	command := fs.String("command", "", "external command; the prompt is written to its stdin, its stdout is the generation")
	armsFlag := fs.String("arms", "all", "comma-separated arms, or all")
	repeats := fs.Int("repeats", 0, "repeats per case×arm (1-5; 0 = each case's grading.repeats)")
	dryRun := fs.Bool("dry-run", false, "render prompts and manifests without executing the command")
	timeout := fs.Duration("timeout", evalrunner.DefaultCommandTimeout, "timeout per generation")
	harness := fs.String("harness", "", "harness name recorded in manifests")
	harnessVersion := fs.String("harness-version", "", "harness version recorded in manifests")
	modelProvider := fs.String("model-provider", "", "model provider recorded in manifests (default: command)")
	modelID := fs.String("model-id", "", "model ID the command uses (default: unreported)")
	modelVersion := fs.String("model-version", "", "exact model version the command uses (default: unreported)")
	seed := fs.Int("seed", -1, "seed the command uses (-1 = not declared)")
	temperature := fs.Float64("temperature", -1, "temperature the command uses (-1 = not declared)")
	topP := fs.Float64("top-p", -1, "top_p the command uses (-1 = not declared)")
	maxOutput := fs.Int("max-output-tokens", -1, "max output tokens the command uses (-1 = not declared)")
	reasoning := fs.Int("reasoning-budget", -1, "reasoning budget the command uses (-1 = none)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "error: unexpected arguments %q\n", fs.Args())
		return 2
	}
	if *provider != "command" {
		fmt.Fprintf(stderr, "error: --provider must be command (got %q)\n", *provider)
		return 2
	}
	arms, err := evalrunner.ParseArms(*armsFlag)
	if err != nil {
		fmt.Fprintf(stderr, "error: --arms: %v\n", err)
		return 2
	}
	if *repeats < 0 || *repeats > 5 {
		fmt.Fprintln(stderr, "error: --repeats must be 0-5")
		return 2
	}
	declared := evalrunner.ModelSettings{
		Provider: *modelProvider, ModelID: *modelID, ModelVersion: *modelVersion,
		Seed: *seed, Temperature: *temperature, TopP: *topP, MaxOutputTokens: *maxOutput,
	}
	if *reasoning >= 0 {
		declared.ReasoningBudget = reasoning
	}
	cp, err := evalrunner.NewCommandProvider(*command, declared, *timeout)
	if err != nil {
		fmt.Fprintf(stderr, "error: --command: %v\n", err)
		return 2
	}
	corpus, rt, out, code := c.prepare(stdout, stderr)
	if code >= 0 {
		return code
	}
	sum, err := evalrunner.Run(corpus, evalrunner.RunConfig{
		Arms: arms, Repeats: *repeats, OutputDir: out, Harness: *harness, HarnessVersion: *harnessVersion,
		NetworkPolicy: c.networkPolicy, Split: c.split, SkipRetrieval: true, DryRun: *dryRun,
	}, cp, evalrunner.DeterministicGrader{}, c.inputs(rt))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return report(stdout, sum, out, c.jsonOut)
}

// refsLookup maps each visible record (revocation and purge applied) to the
// identifiers gold required_evidence_refs may use.
func refsLookup(rt *app.Runtime, capabilityID string, learned bool) evalrunner.RefsFn {
	cache := map[contracts.Profile]map[string][]string{}
	return func(p contracts.Profile, recordID string) []string {
		m, ok := cache[p]
		if !ok {
			m = map[string][]string{}
			if sess, err := rt.Serve(p, capabilityID, learned); err == nil {
				if recs, err := sess.VisibleRecords(); err == nil {
					for _, r := range recs {
						refs := []string{r.SourceRecordID}
						if r.Key != "" {
							refs = append(refs, r.Key)
						}
						m[r.RecordID] = refs
					}
				}
				sess.Store.Close()
			}
			cache[p] = m
		}
		return m[recordID]
	}
}

func report(stdout io.Writer, sum *evalrunner.Summary, out string, jsonOut bool) int {
	code := sum.ExitCode()
	artifacts := filepath.Join(out, sum.RunID)
	if jsonOut {
		json.NewEncoder(stdout).Encode(map[string]any{"exit_code": code, "artifacts": artifacts, "summary": sum})
		return code
	}
	for _, cr := range sum.CaseResults {
		line := fmt.Sprintf("%-8s %-15s %s", cr.Outcome, cr.Arm, cr.CaseID)
		if cr.Outcome == evalrunner.OutcomePassed {
			line += fmt.Sprintf("  score=%.2f", cr.Score)
		}
		if cr.Reason != "" {
			line += "  — " + cr.Reason
		}
		fmt.Fprintln(stdout, line)
	}
	for _, rc := range sum.Retrieval.Cases {
		line := fmt.Sprintf("%-8s %-15s %s  recall=%.3f precision=%.3f", rc.Outcome, "retrieval", rc.CaseID, rc.Recall, rc.Precision)
		if rc.Reason != "" {
			line += "  — " + rc.Reason
		}
		fmt.Fprintln(stdout, line)
	}
	if len(sum.Retrieval.Cases) > 0 {
		fmt.Fprintf(stdout, "retrieval: measured=%d recall=%.3f precision=%.3f thresholds_met=%v\n", sum.Retrieval.Measured, sum.Retrieval.Recall, sum.Retrieval.Precision, sum.Retrieval.ThresholdsMet)
	}
	if sum.DryRun {
		fmt.Fprintf(stdout, "dry run: %d generation(s) would run; none executed\n", sum.PlannedGenerations)
	} else if len(sum.CaseResults) > 0 {
		fmt.Fprintf(stdout, "generations executed: %d\n", sum.Generations)
	}
	fmt.Fprintf(stdout, "artifacts: %s\nexit: %d\n", artifacts, code)
	return code
}

// resolveOut returns the artifact directory: a fresh temp dir by default, or
// the given directory unless it lies inside the Be Me repository.
func resolveOut(flagOut string) (string, error) {
	if flagOut == "" {
		return os.MkdirTemp("", "beme-eval-")
	}
	abs, err := filepath.Abs(flagOut)
	if err != nil {
		return "", err
	}
	if repo := enclosingRepository(abs); repo != "" {
		return "", fmt.Errorf("refusing to write evaluation artifacts inside the Be Me repository (%s)", repo)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", err
	}
	return abs, nil
}

var moduleLine = regexp.MustCompile(`(?m)^module\s+github\.com/0merUfuk/beme\s*$`)

// enclosingRepository returns the nearest ancestor of p (or p) holding this
// module's go.mod, or "".
func enclosingRepository(p string) string {
	for dir := p; ; {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil && moduleLine.Match(data) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || strings.TrimSpace(parent) == "" {
			return ""
		}
		dir = parent
	}
}
