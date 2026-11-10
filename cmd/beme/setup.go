package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/setup"
)

// beme setup: a guided, reversible way to write a user's personal working
// rules. Dry-run is the default. Harness instruction files are only touched
// inside their own managed region, after a backup; harness MCP configuration
// is never edited (it is printed). Nothing leaves the machine.

var rulesMarkers = markers{setup.BlockBegin, setup.BlockEnd}

var managedRuleFile = regexp.MustCompile(`^RULE-\d{3}\.md$`)

const rulesSourceDescriptor = `schema_version: "1"
source_id: personal-rules
type: directory
root: %s
purpose: [reusable_knowledge]
trust: canonical
instruction_semantics: registered_files_only
authority_ceiling: default
sensitivity: personal_private
profiles_allowed: [personal]
ingestion_mode: index_content
include: ["entries/**/*.md"]
`

func setupCmd(args []string) {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		cfgFlag, answersFile, harnessCSV string
		useDefaults, apply, refresh      bool
		remove, printQ, noBuild          bool
	)
	fs.StringVar(&cfgFlag, "config", "", "config directory override")
	fs.StringVar(&answersFile, "answers", "", "JSON answers file (non-interactive; see --print-questions)")
	fs.BoolVar(&useDefaults, "defaults", false, "use the recommended defaults without asking")
	fs.StringVar(&harnessCSV, "harness", "", "comma list: claude-code,codex,cursor,hermes (default: detected)")
	fs.BoolVar(&apply, "apply", false, "write the files (default is a dry run that writes nothing)")
	fs.BoolVar(&refresh, "refresh", false, "re-render from the saved answers.json")
	fs.BoolVar(&remove, "remove", false, "remove the managed rules block from the harness files")
	fs.BoolVar(&printQ, "print-questions", false, "print the interview as JSON (lets any agent ask it)")
	fs.BoolVar(&noBuild, "no-build", false, "do not run build and doctor after --apply")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if printQ {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(setup.Questions())
		return
	}
	_, cfgDir, err := app.ResolveConfig(cfgFlag)
	if err != nil {
		fatalf(1, "error: %v", err)
	}
	harnesses, err := pickHarnesses(harnessCSV)
	if err != nil {
		fatalf(2, "error: %v", err)
	}

	if remove {
		for _, h := range harnesses {
			target, terr := targetFile(h)
			if terr != nil {
				fmt.Printf("%s: no managed instruction file; remove the rules block by hand\n", h)
				continue
			}
			if !apply {
				fmt.Printf("dry run: would remove the rules block from %s\n", target)
				continue
			}
			if err := removeManaged(rulesMarkers, target); err != nil {
				fatalf(1, "error: %v", err)
			}
			fmt.Printf("removed the rules block from %s (other content preserved)\n", target)
		}
		fmt.Printf("kept %s (your answers and rule entries); delete it yourself if unwanted\n", filepath.Join(cfgDir, "rules"))
		return
	}

	rulesDir := filepath.Join(cfgDir, "rules")
	answersPath := filepath.Join(rulesDir, "answers.json")
	var a setup.Answers
	switch {
	case refresh:
		data, rerr := os.ReadFile(answersPath)
		if rerr != nil {
			fatalf(1, "error: --refresh needs %s: %v", answersPath, rerr)
		}
		if a, err = setup.Parse(data); err != nil {
			fatalf(1, "error: %v", err)
		}
	case answersFile != "":
		data, rerr := os.ReadFile(answersFile)
		if rerr != nil {
			fatalf(1, "error: %v", rerr)
		}
		if a, err = setup.Parse(data); err != nil {
			fatalf(1, "error: %v", err)
		}
	case useDefaults:
		a = setup.Defaults()
	default:
		if fi, serr := os.Stdin.Stat(); serr != nil || fi.Mode()&os.ModeCharDevice == 0 {
			fatalf(2, "error: no terminal to ask questions on; pass --answers FILE, --defaults, or run `beme setup --print-questions` and let your agent ask them")
		}
		if a, err = interview(os.Stdin, os.Stdout); err != nil {
			fatalf(1, "error: %v", err)
		}
	}

	if len(harnesses) == 0 {
		fmt.Fprintln(os.Stderr, "warning: no harness instruction file selected (no ~/.claude or ~/.codex found); only the Be Me entries will be written. Pass --harness claude-code,codex,cursor,hermes to choose.")
	}
	block := setup.Block(a)
	rules := setup.Rules(a)
	fmt.Printf("rules (%d):\n%s\n\n", len(rules), block)
	fmt.Println("plan:")
	fmt.Printf("  write  %s\n", answersPath)
	fmt.Printf("  write  %d entries under %s\n", len(rules), filepath.Join(rulesDir, "entries"))
	fmt.Printf("  write  %s (only if absent)\n", filepath.Join(cfgDir, "sources", "personal-rules.yaml"))
	targets := map[string]string{}
	for _, h := range harnesses {
		if t, terr := targetFile(h); terr == nil {
			targets[h] = t
			fmt.Printf("  block  %s (backup first; inside %s only)\n", t, setup.BlockBegin)
		} else {
			fmt.Printf("  print  %s rules text for you to place by hand\n", h)
		}
	}
	if !apply {
		fmt.Println("\ndry run: nothing was written. Re-run with --apply to proceed.")
		return
	}

	backupDir := filepath.Join(cfgDir, "backups")
	stamp := time.Now().UTC().Format("20060102T150405Z")
	if err := applyRules(cfgDir, rulesDir, a, rules); err != nil {
		fatalf(1, "error: %v", err)
	}
	for _, h := range harnesses {
		t, ok := targets[h]
		if !ok {
			fmt.Printf("\n%s: place this text in the harness's rules file:\n%s\n", h, block)
			continue
		}
		if b, err := backupIfChanging(t, backupDir, stamp, block); err != nil {
			fatalf(1, "error: %v", err)
		} else if b != "" {
			fmt.Printf("backup %s\n", b)
		}
		if err := installManaged(rulesMarkers, block, t); err != nil {
			fatalf(1, "error: %v", err)
		}
		fmt.Printf("installed rules block into %s\n", t)
	}
	failed := false
	if !noBuild {
		self, eerr := os.Executable()
		if eerr != nil {
			fatalf(1, "error: cannot locate the beme binary to run build and doctor: %v", eerr)
		}
		for _, sub := range [][]string{{"build", "--profile", "personal"}, {"doctor"}} {
			cmdArgs := append(append([]string{}, sub...), cfgArgs(cfgFlag)...)
			out, rerr := exec.Command(self, cmdArgs...).CombinedOutput()
			fmt.Printf("\n$ beme %s\n%s", strings.Join(sub, " "), strings.TrimRight(string(out), "\n")+"\n")
			if rerr != nil {
				fmt.Printf("FAILED: beme %s: %v\n", strings.Join(sub, " "), rerr)
				failed = true
			}
		}
	}
	printNextSteps(cfgFlag, harnesses)
	if failed {
		fatalf(1, "error: rules were written, but build or doctor failed (output above)")
	}
}

func cfgArgs(cfgFlag string) []string {
	if cfgFlag == "" {
		return nil
	}
	return []string{"--config", cfgFlag}
}

func fatalf(code int, format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(code)
}

func pickHarnesses(csv string) ([]string, error) {
	if csv == "" {
		var out []string
		for _, h := range setup.KnownHarnesses {
			t, terr := targetFile(h)
			if terr != nil {
				continue // no managed instruction file for this harness
			}
			if st, err := os.Stat(filepath.Dir(t)); err == nil && st.IsDir() {
				out = append(out, h)
			}
		}
		return out, nil
	}
	known := map[string]bool{}
	for _, k := range setup.KnownHarnesses {
		known[k] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, h := range strings.Split(csv, ",") {
		h = strings.TrimSpace(h)
		if !known[h] {
			return nil, fmt.Errorf("unknown harness %q (known: %s)", h, strings.Join(setup.KnownHarnesses, ", "))
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out, nil
}

// applyRules writes the answers file, the managed RULE-*.md entries and the
// source descriptor. Only files this command owns are replaced. New content is
// staged beside its destination and renamed into place, and stale entries are
// removed only after every new one is staged, so a failure leaves the previous
// rules intact.
func applyRules(cfgDir, rulesDir string, a setup.Answers, rules []setup.Rule) error {
	entries := filepath.Join(rulesDir, "entries")
	if err := os.MkdirAll(entries, 0o700); err != nil {
		return err
	}
	type staged struct{ tmp, dst string }
	var plan []staged
	cleanup := func() {
		for _, s := range plan {
			_ = os.Remove(s.tmp)
		}
	}
	stage := func(dst string, data []byte) error {
		tmp := dst + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			return err
		}
		plan = append(plan, staged{tmp, dst})
		return nil
	}
	keep := map[string]bool{}
	for _, r := range rules {
		keep[r.ID+".md"] = true
		if err := stage(filepath.Join(entries, r.ID+".md"), []byte(setup.Entry(r))); err != nil {
			cleanup()
			return err
		}
	}
	if err := stage(filepath.Join(rulesDir, "answers.json"), setup.Marshal(a)); err != nil {
		cleanup()
		return err
	}
	old, _ := os.ReadDir(entries)
	for _, e := range old {
		if !e.IsDir() && managedRuleFile.MatchString(e.Name()) && !keep[e.Name()] {
			if err := os.Remove(filepath.Join(entries, e.Name())); err != nil {
				cleanup()
				return err
			}
		}
	}
	for _, s := range plan {
		if err := os.Rename(s.tmp, s.dst); err != nil {
			cleanup()
			return err
		}
	}
	srcDir := filepath.Join(cfgDir, "sources")
	if err := os.MkdirAll(srcDir, 0o700); err != nil {
		return err
	}
	desc := filepath.Join(srcDir, "personal-rules.yaml")
	want := fmt.Sprintf(rulesSourceDescriptor, strconv.Quote(filepath.ToSlash(rulesDir)))
	existing, err := os.ReadFile(desc)
	switch {
	case os.IsNotExist(err):
		return os.WriteFile(desc, []byte(want), 0o600)
	case err != nil:
		return err
	case string(existing) != want:
		fmt.Printf("note: %s already exists and was not changed; make sure its root is %s so the entries are indexed\n",
			desc, filepath.ToSlash(rulesDir))
	}
	return nil
}

// backupIfChanging copies target into backupDir unless its managed region
// already holds block. It returns the backup path ("" when none was needed).
func backupIfChanging(target, backupDir, stamp, block string) (string, error) {
	data, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if inner := extractManaged(rulesMarkers, string(data)); inner != "" && strings.TrimSpace(inner) == strings.TrimSpace(block) {
		return "", nil
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(backupDir, filepath.Base(target)+".pre-setup-"+stamp)
	return dst, os.WriteFile(dst, data, 0o600)
}

func printNextSteps(cfgFlag string, harnesses []string) {
	self, _ := os.Executable()
	cfg := ""
	if cfgFlag != "" {
		cfg = " --config " + shellQuote(cfgFlag)
	}
	fmt.Println("\nnext steps (the wizard never edits harness MCP configuration):")
	for _, h := range harnesses {
		switch h {
		case "claude-code":
			fmt.Printf("  claude mcp add beme -- %s serve --projection personal --capability cap_personalclaude --transport stdio%s\n", shellQuote(self), cfg)
		case "codex":
			fmt.Printf("  add to ~/.codex/config.toml:\n    [mcp_servers.beme]\n    command = %q\n    args = [\"serve\", \"--projection\", \"personal\", \"--capability\", \"cap_personalcodex\", \"--transport\", \"stdio\"%s]\n", self, tomlCfg(cfgFlag))
		}
	}
	fmt.Println("  start a NEW agent session (instructions load at session start), then ask it:")
	fmt.Println("  \"List the numbered working rules in your instructions.\"")
	fmt.Println("  undo: beme setup --remove --apply   (your answers and entries are kept)")
}

func tomlCfg(cfgFlag string) string {
	if cfgFlag == "" {
		return ""
	}
	return fmt.Sprintf(", \"--config\", %q", cfgFlag)
}

// interview asks the questions on r/w. Enter accepts the shown default.
func interview(r io.Reader, w io.Writer) (setup.Answers, error) {
	sc := bufio.NewScanner(r)
	read := func() (string, bool) {
		if !sc.Scan() {
			return "", false
		}
		return strings.TrimSpace(sc.Text()), true
	}
	a := setup.Defaults()
	yn := func(prompt string, def bool) (bool, error) {
		d := "Y/n"
		if !def {
			d = "y/N"
		}
		for {
			fmt.Fprintf(w, "%s [%s] ", prompt, d)
			s, ok := read()
			if !ok {
				return def, io.ErrUnexpectedEOF
			}
			switch strings.ToLower(s) {
			case "":
				return def, nil
			case "y", "yes":
				return true, nil
			case "n", "no":
				return false, nil
			}
			fmt.Fprintln(w, "  please answer y or n")
		}
	}
	choice := func(prompt string, opts []string, def string) (string, error) {
		for {
			fmt.Fprintf(w, "%s (%s) [%s] ", prompt, strings.Join(opts, "/"), def)
			s, ok := read()
			if !ok {
				return def, io.ErrUnexpectedEOF
			}
			if s == "" {
				return def, nil
			}
			for _, o := range opts {
				if s == o {
					return o, nil
				}
			}
			fmt.Fprintln(w, "  choose one of the listed values")
		}
	}
	var err error
	if a.Evidence, err = yn("Agents must show command output before saying a task is done?", a.Evidence); err != nil {
		return a, err
	}
	if a.OneRecommendation, err = yn("For decisions you delegate, give one recommendation, not a menu?", a.OneRecommendation); err != nil {
		return a, err
	}
	if a.Style, err = choice("Answer style", []string{"short", "balanced"}, a.Style); err != nil {
		return a, err
	}
	if a.FailureReporting, err = yn("Report failed or skipped steps plainly?", a.FailureReporting); err != nil {
		return a, err
	}
	if a.Autonomy, err = choice("Act without asking, or ask before commits and deletions?", []string{"act", "ask"}, a.Autonomy); err != nil {
		return a, err
	}
	if a.Autonomy == "act" {
		for {
			fmt.Fprintf(w, "Always confirm first before (comma list of purchase,public-post,dangerous-deletion; 'none' for nothing) [%s] ", strings.Join(a.Gates, ","))
			s, ok := read()
			if !ok {
				return a, io.ErrUnexpectedEOF
			}
			if s == "" {
				break
			}
			var gates []string
			if !strings.EqualFold(s, "none") {
				for _, g := range strings.Split(s, ",") {
					gates = append(gates, strings.TrimSpace(g))
				}
			}
			if verr := setup.ValidateGates(gates); verr != nil {
				fmt.Fprintf(w, "  %v\n", verr)
				continue
			}
			a.Gates = gates
			break
		}
	} else {
		a.Gates = nil
	}
	for {
		fmt.Fprintf(w, "Max lines for documents shown for review, 0 for no limit [%d] ", a.DocMaxLines)
		s, ok := read()
		if !ok {
			return a, io.ErrUnexpectedEOF
		}
		if s == "" {
			break
		}
		n, perr := strconv.Atoi(s)
		if perr != nil {
			fmt.Fprintln(w, "  enter a number")
			continue
		}
		if verr := setup.ValidateDocLines(n); verr != nil {
			fmt.Fprintf(w, "  %v\n", verr)
			continue
		}
		a.DocMaxLines = n
		break
	}
	fmt.Fprintf(w, "Up to %d extra rules of your own, one per line, blank line to finish:\n", setup.MaxCustom)
	for len(a.Custom) < setup.MaxCustom {
		s, ok := read()
		if !ok || s == "" {
			break
		}
		if verr := setup.ValidateCustom(s); verr != nil {
			fmt.Fprintf(w, "  %v\n", verr)
			continue
		}
		a.Custom = append(a.Custom, s)
	}
	if err := a.Validate(); err != nil {
		return a, err
	}
	return a, nil
}
