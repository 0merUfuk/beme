package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/brain"
)

// brainCmd implements the V2 second-brain surface (ADR-033). Admin/agent CLI
// only — the MCP tool surface stays exactly four tools (ADR-009).
//
//	beme brain init [--git]
//	beme brain project add <slug> --root PATH [--name N] | list
//	beme brain capture --type T --title T --body B [--why W] [--next N] [--topic K] [--project P]
//	beme brain doctor [--dry-run] [--commit] [--quiet] [--json]
//	beme brain session-start [--format text|claude-hook]
//	beme brain list|show|done|archive
//	beme brain hooks|install-hooks|adapter        harness integration
//	beme brain briefing|status|backup|restore|freeze
//
// Exit codes follow the CLI contract: 0 ok · 1 failure · 2 usage · 3 gate
// rejection · 4 not found.
func brainCmd(args []string) {
	if len(args) < 1 {
		brainUsage()
		os.Exit(2)
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "init":
		brainInit(rest)
	case "project":
		brainProject(rest)
	case "capture":
		brainCapture(rest)
	case "doctor":
		brainDoctor(rest)
	case "session-start":
		brainSessionStart(rest)
	case "stop-check":
		brainStopCheck(rest)
	case "list":
		brainList(rest)
	case "show":
		brainShow(rest)
	case "done":
		brainDone(rest)
	case "archive":
		brainArchive(rest)
	case "hooks":
		brainHooks(rest)
	case "install-hooks":
		brainInstallHooks(rest)
	case "adapter":
		brainAdapter(rest)
	case "briefing":
		brainBriefing(rest)
	case "status":
		brainStatus(rest)
	case "backup":
		brainBackup(rest)
	case "restore":
		brainRestore(rest)
	case "freeze":
		brainFreeze(rest)
	default:
		brainUsage()
		os.Exit(2)
	}
}

func brainUsage() {
	fmt.Fprint(os.Stderr, `beme brain — second-brain work memory (decisions, open work, summaries, learnings)

  beme brain init [--git]
  beme brain project add <slug> --root PATH [--name NAME]
  beme brain project list
  beme brain capture --type decision|open|summary|learning --title T --body B|- [--why W] [--next N] [--topic K] [--project P]
  beme brain doctor [--dry-run] [--commit] [--quiet] [--json]
  beme brain session-start [--format text|claude-hook] [--cwd DIR]
  beme brain stop-check [--format claude-hook]     (Stop hook: remind once enough edits accumulate without a capture; never blocks on error)
  beme brain list [--project P] [--type T] [--all] [--json]
  beme brain show <id>
  beme brain done <id>
  beme brain archive <id> [--reason TEXT]
  beme brain hooks [--binary PATH] [--commit]
  beme brain install-hooks --user|--project|--settings FILE [--binary PATH] [--remove] [--dry-run]
  beme brain adapter install|remove|verify <codex|claude-code>     (instruction block for hook-less harnesses)
  beme brain briefing [--mode normal|economy|manual] [--doctor] [--record-session --session-id ID] (read-only by default; optional maintenance and tracking)
  beme brain status [--json]
  beme brain backup [--list]
  beme brain restore <snapshot.tar.gz> [--force]
  beme brain freeze on|off

All subcommands accept --config DIR (self-contained deployment root, ADR-026).
`)
}

// parseInterspersed lets flags follow positional arguments
// (`beme brain done <id> --config X`), which the stdlib flag package stops at.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

type brainCommon struct {
	fs      *flag.FlagSet
	config  string
	jsonOut bool
}

func newBrainFlags(name string) *brainCommon {
	c := &brainCommon{fs: flag.NewFlagSet("brain "+name, flag.ContinueOnError)}
	c.fs.SetOutput(os.Stderr)
	c.fs.StringVar(&c.config, "config", "", "config directory override (self-contained deployment root)")
	c.fs.BoolVar(&c.jsonOut, "json", false, "JSON output")
	return c
}

func (c *brainCommon) parse(args []string) []string {
	pos, err := parseInterspersed(c.fs, args)
	if err != nil {
		os.Exit(2)
	}
	return pos
}

func openVault(config string) *brain.Vault {
	cfg, _, err := app.ResolveConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return brain.Open(cfg.BrainDir)
}

func brainFail(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	var gate *brain.GateError
	switch {
	case errors.As(err, &gate):
		os.Exit(3)
	case strings.Contains(err.Error(), "not found"):
		os.Exit(4)
	default:
		os.Exit(1)
	}
}

func emitJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// ---- init / project --------------------------------------------------------

func brainInit(args []string) {
	c := newBrainFlags("init")
	withGit := c.fs.Bool("git", false, "git init the vault so every change is a reviewable diff")
	c.parse(args)
	v := openVault(c.config)
	if err := v.Init(); err != nil {
		brainFail(err)
	}
	if *withGit {
		if _, err := os.Stat(filepath.Join(v.Root, ".git")); err != nil {
			if out, err := exec.Command("git", "-C", v.Root, "init", "-q").CombinedOutput(); err != nil {
				brainFail(fmt.Errorf("git init: %v: %s", err, out))
			}
		}
	}
	fmt.Printf("brain vault ready: %s\n", v.Root)
	fmt.Printf("next: edit identity.md, then `%s brain project add%s <slug> --root <path>` and `%s brain hooks%s`\n", shellQuote(currentBrainBinary()), cfgFlag(c.config), shellQuote(currentBrainBinary()), cfgFlag(c.config))
}

func brainProject(args []string) {
	if len(args) < 1 {
		brainUsage()
		os.Exit(2)
	}
	verb, rest := args[0], args[1:]
	c := newBrainFlags("project " + verb)
	root := c.fs.String("root", "", "absolute project root (real path)")
	name := c.fs.String("name", "", "display name")
	pos := c.parse(rest)
	v := openVault(c.config)
	switch verb {
	case "add":
		if len(pos) != 1 || *root == "" {
			fmt.Fprintln(os.Stderr, "usage: beme brain project add <slug> --root PATH [--name NAME]")
			os.Exit(2)
		}
		p, err := v.AddProject(pos[0], *name, *root)
		if err != nil {
			brainFail(err)
		}
		if c.jsonOut {
			emitJSON(p)
			return
		}
		fmt.Printf("project %s registered (roots: %s)\n", p.Slug, strings.Join(p.Roots, ", "))
	case "list":
		ps, err := v.Projects()
		if err != nil {
			brainFail(err)
		}
		if c.jsonOut {
			emitJSON(ps)
			return
		}
		for _, p := range ps {
			fmt.Printf("%-20s %s  %s\n", p.Slug, p.Name, strings.Join(p.Roots, ", "))
		}
	default:
		brainUsage()
		os.Exit(2)
	}
}

// ---- capture ---------------------------------------------------------------

func brainCapture(args []string) {
	c := newBrainFlags("capture")
	binary := c.fs.String("binary", "", "executable for follow-up commands (default: this binary)")
	typ := c.fs.String("type", "", "decision|open|summary|learning")
	project := c.fs.String("project", "", "project slug (default: resolved from --cwd)")
	cwd := c.fs.String("cwd", "", "directory used to resolve the project (default: current directory)")
	title := c.fs.String("title", "", "short title")
	body := c.fs.String("body", "", "the outcome, in your own distilled words ('-' reads stdin)")
	why := c.fs.String("why", "", "rationale (required for decisions)")
	next := c.fs.String("next", "", "concrete next step (open items)")
	topic := c.fs.String("topic", "", "stable key; reuse it to replace an earlier decision")
	c.parse(args)

	if *body == "-" {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 64*1024))
		if err != nil {
			brainFail(err)
		}
		*body = string(data)
	}
	v := openVault(c.config)
	slug := *project
	if slug == "" {
		dir := *cwd
		if dir == "" {
			dir, _ = os.Getwd()
		}
		m := v.ResolveProject(dir)
		if m.Slug == "" {
			fmt.Fprintf(os.Stderr, "error: cannot resolve a project for %s (%s); register the intended project or pass --project <slug>; global is only for explicitly cross-project decisions/learnings\n", dir, m.Reason)
			os.Exit(2)
		}
		slug = m.Slug
	}
	n, err := v.Capture(brain.CaptureInput{
		Type: brain.Type(*typ), Project: slug, Title: *title, Statement: *body,
		Why: *why, Next: *next, Topic: *topic,
	}, time.Now())
	if err != nil {
		brainFail(err)
	}
	if n.AccountingWarning != "" {
		fmt.Fprintln(os.Stderr, "warning: "+n.AccountingWarning)
	}
	if c.jsonOut {
		emitJSON(map[string]any{"status": "queued", "id": n.ID, "project": n.Project, "type": n.Type, "accounting_warning": n.AccountingWarning})
		return
	}
	fmt.Printf("queued %s [%s] — Brain Doctor promotes it at session end/start (or run: %s brain doctor%s)\n", n.ID, n.Project, shellQuote(firstNonEmpty(*binary, currentBrainBinary())), cfgFlag(c.config))
}

// ---- doctor ----------------------------------------------------------------

func brainDoctor(args []string) {
	c := newBrainFlags("doctor")
	dry := c.fs.Bool("dry-run", false, "report what would change; write nothing")
	commit := c.fs.Bool("commit", false, "git-commit any pending vault changes after the run (needs `init --git`)")
	quiet := c.fs.Bool("quiet", false, "no output on success (hook use)")
	strict := c.fs.Bool("strict", false, "exit 1 when any hygiene flag is raised (CI use)")
	c.parse(args)
	v := openVault(c.config)
	if *commit && !*dry {
		// Fail before touching anything: --commit on a non-repository vault
		// would otherwise run maintenance and then refuse to record it.
		if _, err := os.Stat(filepath.Join(v.Root, ".git")); err != nil {
			brainFail(fmt.Errorf("vault is not a git repository (run: beme brain init --git)"))
		}
	}
	rep, err := v.Doctor(brain.Options{DryRun: *dry})
	if err != nil {
		if rep != nil {
			if c.jsonOut {
				emitJSON(rep)
			} else if !*quiet {
				fmt.Fprint(os.Stderr, rep.Human())
			}
		}
		if *quiet && (errors.Is(err, brain.ErrLocked) || errors.Is(err, brain.ErrNotInitialized)) {
			return // hook use: another process is maintaining the vault, or none is set up here
		}
		brainFail(err)
	}
	if *commit && !*dry {
		// Commits whatever is pending (fresh captures included), not only this
		// run's actions; a clean tree is a no-op (no empty commits).
		if err := commitVault(v.Root, fmt.Sprintf("brain: doctor — %d action(s)", len(rep.Actions))); err != nil {
			brainFail(err)
		}
	}
	switch {
	case c.jsonOut:
		emitJSON(rep)
	case *quiet:
	default:
		fmt.Print(rep.Human())
	}
	if *strict && len(rep.Flags) > 0 {
		os.Exit(1)
	}
}

// commitVault records the vault's current state as one git commit so the
// change set is a reviewable diff. It refuses to touch a vault that is not
// its own repository root (never commits into an enclosing repo).
func commitVault(root, msg string) error {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return fmt.Errorf("vault is not a git repository (run: beme brain init --git)")
	}
	git := func(a ...string) ([]byte, error) {
		return exec.Command("git", append([]string{"-C", root}, a...)...).CombinedOutput()
	}
	if out, err := git("add", "-A"); err != nil {
		return fmt.Errorf("git add: %v: %s", err, out)
	}
	if _, err := git("diff", "--cached", "--quiet"); err == nil {
		return nil // nothing to commit
	}
	// Automation must not depend on (or block on) the user's signing setup.
	if out, err := git("-c", "user.name=beme-brain", "-c", "user.email=beme-brain@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "commit", "-q", "-m", msg); err != nil {
		return fmt.Errorf("git commit: %v: %s", err, out)
	}
	return nil
}

// ---- session-start ---------------------------------------------------------

// brainSessionStart is the SessionStart hook entry point. It is FAIL-OPEN by
// design: whatever goes wrong, the session must start, so errors degrade to a
// one-line notice instead of a non-zero exit.
func brainSessionStart(args []string) {
	c := newBrainFlags("session-start")
	format := c.fs.String("format", "text", "text|claude-hook")
	cwd := c.fs.String("cwd", "", "working directory (default: hook input cwd, then current directory)")
	noDoctor := c.fs.Bool("no-doctor", false, "skip the maintenance pass")
	binary := c.fs.String("binary", "", "executable for follow-up commands (default: this binary)")
	sessionID := c.fs.String("session-id", "", "opaque session identity (optional; otherwise invocation-only accounting)")
	modeFlag := c.fs.String("mode", "", "normal|economy|manual (default: brain_mode in config.yaml, else normal)")
	c.parse(args)

	dir := *cwd
	source := "startup"
	if *format == "claude-hook" {
		var in struct {
			CWD       string `json:"cwd"`
			Source    string `json:"source"`
			SessionID string `json:"session_id"`
		}
		if data := readHookStdin(); len(data) > 0 {
			_ = json.Unmarshal(data, &in)
		}
		if *sessionID == "" {
			*sessionID = in.SessionID
		}
		if dir == "" {
			dir = in.CWD
		}
		if in.Source != "" {
			source = in.Source
		}
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}

	var text string
	cfg, _, err := app.ResolveConfig(c.config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "beme brain: %v\n", err)
		text = "# Be Me Brain\n\nBrain unavailable (config error); continuing without recorded memory. Do not fall back to guessing past decisions.\n"
	} else {
		v := brain.Open(cfg.BrainDir)
		mode, merr := brain.ParseMode(firstNonEmpty(*modeFlag, cfg.BrainMode))
		if merr != nil {
			fmt.Fprintf(os.Stderr, "beme brain: %v (using normal)\n", merr)
			mode = brain.ModeNormal
		}
		var rep *brain.Report
		notice := ""
		if v.Initialized() && !*noDoctor && !v.Frozen() {
			rep, err = v.Doctor(brain.Options{})
			if err != nil {
				notice = "maintenance pass skipped: " + brainFailureClass(err) + "; pending proposals were not injected"
			}
		}
		flagHint := ""
		if c.config != "" {
			flagHint = "--config " + shellQuote(c.config)
		}
		match := v.ResolveProject(dir)
		text, _ = v.Briefing(brain.BriefingOptions{CWD: dir, Mode: mode, Report: rep, ConfigFlag: flagHint, CommandPrefix: shellQuote(firstNonEmpty(*binary, currentBrainBinary())), Notice: notice})
		// IDs make resumes idempotent; no-ID starts remain invocation counts.
		// Fork is a new session only when its opaque identity is new.
		if source == "startup" || source == "resume" || source == "fork" {
			harness := "cli"
			if *format == "claude-hook" {
				harness = "claude-code"
			}
			if err := v.RecordSessionEvent(match.Slug, harness, *sessionID, time.Now()); err != nil && !errors.Is(err, brain.ErrFrozen) {
				notice = joinBrainNotice(notice, "session accounting not recorded: "+brainFailureClass(err))
				text, _ = v.Briefing(brain.BriefingOptions{CWD: dir, Mode: mode, Report: rep, ConfigFlag: flagHint, CommandPrefix: shellQuote(firstNonEmpty(*binary, currentBrainBinary())), Notice: notice})
			}
		}
	}

	if *format == "claude-hook" {
		emitJSON(map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName":     "SessionStart",
			"additionalContext": text,
		}})
		return
	}
	fmt.Print(text)
}

// brainBriefing is the manual pull (`beme brain briefing`), deliberately
// separate from the session-start hook: it is a TRUE read — no lock file, no
// directory creation, no writes of any kind — so it works under a read-only
// sandbox (Codex's `-s read-only`, for one) where the hook's maintenance pass
// cannot run. `--doctor` opts into a best-effort maintenance pass first (the
// natural choice for a harness with no session-end hook of its own, so
// captures still get promoted); a failure there — sandboxed, frozen, locked
// by a concurrent run — degrades to a one-line notice, never blocks the read.
func brainBriefing(args []string) {
	c := newBrainFlags("briefing")
	cwd := c.fs.String("cwd", "", "working directory (default: current directory)")
	modeFlag := c.fs.String("mode", "", "normal|economy|manual (default: brain_mode in config.yaml, else normal)")
	doDoctor := c.fs.Bool("doctor", false, "also attempt the maintenance pass first (best-effort; never fails the read)")
	binary := c.fs.String("binary", "", "executable for follow-up commands (default: this binary)")
	record := c.fs.Bool("record-session", false, "explicitly record this startup invocation (writes machine-local state)")
	sessionID := c.fs.String("session-id", "", "opaque ID for idempotent --record-session accounting")
	c.parse(args)

	dir := *cwd
	if dir == "" {
		dir, _ = os.Getwd()
	}
	cfg, _, err := app.ResolveConfig(c.config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "beme brain: %v\n", err)
		fmt.Print("# Be Me Brain\n\nBrain unavailable (config error); continuing without recorded memory. Do not fall back to guessing past decisions.\n")
		return
	}
	v := brain.Open(cfg.BrainDir)
	mode, merr := brain.ParseMode(firstNonEmpty(*modeFlag, cfg.BrainMode))
	if merr != nil {
		fmt.Fprintf(os.Stderr, "beme brain: %v (using normal)\n", merr)
		mode = brain.ModeNormal
	}
	var rep *brain.Report
	note := ""
	if *doDoctor && v.Initialized() {
		if r, derr := v.Doctor(brain.Options{}); derr == nil {
			rep = r
		} else {
			note = "maintenance pass skipped: " + brainFailureClass(derr) + "; pending proposals were not injected"
		}
	}
	flagHint := ""
	if c.config != "" {
		flagHint = "--config " + shellQuote(c.config)
	}
	if *record {
		if err := v.RecordSessionEvent(v.ResolveProject(dir).Slug, "hookless", *sessionID, time.Now()); err != nil {
			note = joinBrainNotice(note, "session accounting not recorded: "+brainFailureClass(err))
		}
	}
	text, _ := v.Briefing(brain.BriefingOptions{CWD: dir, Mode: mode, Report: rep, ConfigFlag: flagHint, CommandPrefix: shellQuote(firstNonEmpty(*binary, currentBrainBinary())), Notice: note})
	fmt.Print(text)
}

// readHookStdin reads the hook payload without ever blocking a session start:
// only when stdin is a pipe/file, capped, and time-boxed.
func readHookStdin() []byte {
	if st, err := os.Stdin.Stat(); err != nil || st.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	ch := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		ch <- data
	}()
	select {
	case data := <-ch:
		return data
	case <-time.After(1500 * time.Millisecond):
		return nil
	}
}

func currentBrainBinary() string {
	exe, err := os.Executable()
	if err != nil {
		brainFail(fmt.Errorf("cannot identify the running brain executable: %w", err))
	}
	return exe
}

func brainFailureClass(err error) string {
	switch {
	case errors.Is(err, brain.ErrLocked):
		return "vault locked"
	case errors.Is(err, brain.ErrFrozen):
		return "vault frozen"
	case errors.Is(err, brain.ErrNotInitialized):
		return "vault unavailable"
	case os.IsPermission(err):
		return "permission denied (read-only or sandboxed)"
	default:
		return "vault I/O or validation failure; operator inspection required"
	}
}
func joinBrainNotice(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " 	\n'\"\\$`&|;<>()*?[]{}~!#") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ---- list / show / done / archive -----------------------------------------

func brainList(args []string) {
	c := newBrainFlags("list")
	project := c.fs.String("project", "", "filter by project")
	typ := c.fs.String("type", "", "filter by type")
	all := c.fs.Bool("all", false, "include archived notes")
	c.parse(args)
	v := openVault(c.config)
	notes, err := v.List(*project, brain.Type(*typ), *all)
	if err != nil {
		brainFail(err)
	}
	if c.jsonOut {
		type row struct {
			ID      string `json:"id"`
			Project string `json:"project"`
			Type    string `json:"type"`
			Status  string `json:"status"`
			Title   string `json:"title"`
			Topic   string `json:"topic,omitempty"`
		}
		rows := make([]row, 0, len(notes))
		for _, n := range notes {
			rows = append(rows, row{n.ID, n.Project, string(n.Type), n.Status, n.Title, n.Topic})
		}
		emitJSON(rows)
		return
	}
	for _, n := range notes {
		fmt.Printf("%-52s %-9s %-10s %s\n", n.ID, n.Type, n.Status, n.Title)
	}
	if len(notes) == 0 {
		fmt.Println("(no notes)")
	}
}

func brainShow(args []string) {
	c := newBrainFlags("show")
	pos := c.parse(args)
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: beme brain show <id>")
		os.Exit(2)
	}
	data, rel, err := openVault(c.config).ReadAny(pos[0])
	if err != nil {
		brainFail(err)
	}
	fmt.Printf("# %s\n%s", rel, data)
}

func brainDone(args []string) {
	c := newBrainFlags("done")
	pos := c.parse(args)
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: beme brain done <id>")
		os.Exit(2)
	}
	n, err := openVault(c.config).Done(pos[0], time.Now())
	if err != nil {
		brainFail(err)
	}
	fmt.Printf("done %s (archived)\n", n.ID)
}

func brainArchive(args []string) {
	c := newBrainFlags("archive")
	reason := c.fs.String("reason", "", "why this note is retired")
	pos := c.parse(args)
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: beme brain archive <id> [--reason TEXT]")
		os.Exit(2)
	}
	n, err := openVault(c.config).Archive(pos[0], *reason, time.Now())
	if err != nil {
		brainFail(err)
	}
	fmt.Printf("archived %s\n", n.ID)
}

// ---- hooks -----------------------------------------------------------------

// brainHooks prints the Claude Code settings fragment. Like the V1 MCP
// fragment, harness configuration is user-owned: Be Me prints it, and only
// `install-hooks` (an explicit operator command) merges it into a file.
func brainHooks(args []string) {
	c := newBrainFlags("hooks")
	binary := c.fs.String("binary", currentBrainBinary(), "beme executable (default: this binary)")
	commit := c.fs.Bool("commit", false, "also git-commit the vault at session end")
	c.parse(args)
	emitJSON(map[string]any{"hooks": hooksFragment(*binary, cfgFlag(c.config), *commit)})
}

func cfgFlag(config string) string {
	if config == "" {
		return ""
	}
	return " --config " + shellQuote(config)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// brainInstallHooks merges the fragment into a Claude Code settings file:
// idempotent, other hooks and settings preserved, original kept as .beme-bak.
func brainInstallHooks(args []string) {
	c := newBrainFlags("install-hooks")
	user := c.fs.Bool("user", false, "target ~/.claude/settings.json")
	project := c.fs.Bool("project", false, "target ./.claude/settings.json")
	file := c.fs.String("settings", "", "target settings file")
	binary := c.fs.String("binary", "", "beme executable (default: this binary)")
	remove := c.fs.Bool("remove", false, "remove the brain hooks instead")
	dry := c.fs.Bool("dry-run", false, "show the result; write nothing")
	commit := c.fs.Bool("commit", false, "also git-commit the vault at session end")
	c.parse(args)

	target := *file
	switch {
	case target != "":
	case *user:
		home, err := os.UserHomeDir()
		if err != nil {
			brainFail(err)
		}
		target = filepath.Join(home, ".claude", "settings.json")
	case *project:
		target = filepath.Join(".claude", "settings.json")
	default:
		fmt.Fprintln(os.Stderr, "usage: beme brain install-hooks --user|--project|--settings FILE [--binary PATH] [--remove] [--dry-run]")
		os.Exit(2)
	}
	bin := *binary
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			brainFail(err)
		}
		if looksTemporary(exe) {
			fmt.Fprintln(os.Stderr, "error: this beme is a throwaway build (go run); install it first (go install ./cmd/beme) or pass --binary PATH")
			os.Exit(2)
		}
		bin = exe
	}
	msg, err := installSettingsHooks(target, hooksFragment(bin, cfgFlag(c.config), *commit), *remove, *dry)
	if err != nil {
		brainFail(err)
	}
	fmt.Println(msg)
}

// brainAdapter manages the instruction block for harnesses without hooks (or
// manual mode). It reuses V1's byte-exact managed-block mechanics with its own
// markers, so it never disturbs the V1 bootstrap block.
func brainAdapter(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: beme brain adapter install|remove|verify <codex|claude-code> [--config DIR]")
		os.Exit(2)
	}
	verb, harness := args[0], args[1]
	c := newBrainFlags("adapter")
	binary := c.fs.String("binary", currentBrainBinary(), "executable for generated instructions")
	c.parse(args[2:])
	target, err := targetFile(harness)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	text := brainInstructions(cfgFlag(c.config), shellQuote(*binary))
	switch verb {
	case "install":
		if err := installManaged(brainMarkers, text, target); err != nil {
			brainFail(err)
		}
		fmt.Printf("installed brain instruction block into %s\n", target)
	case "remove":
		if err := removeManaged(brainMarkers, target); err != nil {
			brainFail(err)
		}
		fmt.Printf("removed brain instruction block from %s (unrelated content preserved)\n", target)
	case "verify":
		ok, err := verifyManaged(brainMarkers, text, target)
		if err != nil {
			brainFail(err)
		}
		if !ok {
			fmt.Printf("drift: brain instruction block missing or differs in %s\n", target)
			os.Exit(4)
		}
		fmt.Printf("ok: brain instruction block matches in %s\n", target)
	default:
		fmt.Fprintln(os.Stderr, "error: unknown verb", verb)
		os.Exit(2)
	}
}

// ---- status / backup / restore / freeze ------------------------------------

func brainStatus(args []string) {
	c := newBrainFlags("status")
	c.parse(args)
	cfg, _, err := app.ResolveConfig(c.config)
	if err != nil {
		brainFail(err)
	}
	v := brain.Open(cfg.BrainDir)
	st := v.Status()
	if c.jsonOut {
		emitJSON(st)
		return
	}
	if !st.Initialized {
		fmt.Printf("brain vault: %s\ninitialized: no (run: beme brain init)\n", st.Root)
		return
	}
	mode, _ := brain.ParseMode(cfg.BrainMode)
	fmt.Printf("brain vault:  %s\nschema:       v%d   frozen: %v   mode: %s\nprojects:     %s\n",
		st.Root, st.SchemaVersion, st.Frozen, mode, strings.Join(st.Projects, ", "))
	if st.LiveUnknown {
		fmt.Printf("live notes:   unknown (%s; nothing was counted — retry when no maintenance is running)\n", st.LiveUnknownReason)
	} else {
		fmt.Printf("live notes:   decision %d · open %d · summary %d · learning %d\n",
			st.Live["decision"], st.Live["open"], st.Live["summary"], st.Live["learning"])
	}
	fmt.Printf("inbox:        %d pending · %d rejected · %d archived\n", st.Inbox, st.Rejected, st.Archived)
	fmt.Printf("snapshots:    %d", st.Snapshots)
	if st.LatestSnap != "" {
		fmt.Printf(" (latest %s)", st.LatestSnap)
	}
	fmt.Println()
	for slug, n := range st.SilentSession {
		fmt.Printf("identified sessions: %s=%d since last accepted capture (retained %d-ID window; not measured use)\n", slug, n, st.SessionWindow)
	}
	for slug, n := range st.Invocations {
		fmt.Printf("start/resume/fork invocations: %s=%d since last accepted capture (not distinct sessions)\n", slug, n)
	}
	if st.LastLog != "" {
		fmt.Printf("last log:     %s\n", st.LastLog)
	}
}

func brainBackup(args []string) {
	c := newBrainFlags("backup")
	list := c.fs.Bool("list", false, "list snapshots instead of creating one")
	c.parse(args)
	v := openVault(c.config)
	if *list {
		snaps, err := v.ListBackups()
		if err != nil {
			brainFail(err)
		}
		if c.jsonOut {
			emitJSON(snaps)
			return
		}
		for _, s := range snaps {
			fmt.Printf("%s  %8d B  sha256:%.12s  %s\n", s.Time.Format("2006-01-02T15:04:05Z"), s.Bytes, s.SHA256, s.Path)
		}
		if len(snaps) == 0 {
			fmt.Println("(no snapshots)")
		}
		return
	}
	path, err := v.Backup("manual", time.Now())
	if err != nil {
		brainFail(err)
	}
	fmt.Println("snapshot written:", path)
}

func brainRestore(args []string) {
	c := newBrainFlags("restore")
	force := c.fs.Bool("force", false, "replace a non-empty vault (a pre-restore snapshot is taken first)")
	pos := c.parse(args)
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: beme brain restore <snapshot.tar.gz> [--force]")
		os.Exit(2)
	}
	if err := openVault(c.config).Restore(pos[0], *force, time.Now()); err != nil {
		brainFail(err)
	}
	fmt.Println("restored from", pos[0])
}

func brainFreeze(args []string) {
	c := newBrainFlags("freeze")
	pos := c.parse(args)
	if len(pos) != 1 || (pos[0] != "on" && pos[0] != "off") {
		fmt.Fprintln(os.Stderr, "usage: beme brain freeze on|off")
		os.Exit(2)
	}
	if err := openVault(c.config).Freeze(pos[0] == "on", time.Now()); err != nil {
		brainFail(err)
	}
	fmt.Println("brain freeze:", pos[0])
}
