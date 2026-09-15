// Command beme is the administrative CLI (blueprint §12.2): source and
// profile lifecycle, preview, explain, doctor, forget, serve.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
)

const version = "0.1.0-alpha.1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	// Exit codes (versioned contract, FR-042):
	// 0 ok | 1 runtime failure | 2 usage | 3 policy blocked | 4 not found
	var cmd string
	var fs *flag.FlagSet
	args := os.Args[1:]
	cmd = args[0]
	fs = flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		jsonOut             bool
		configDir           string
		profile             string
		task                string
		wsHint              string
		capability          string
		transport           string
		experimentalLearned bool
		exportOut           string
		confirm             string
		removeCanonical     bool
		dryRun              bool
	)
	switch cmd {
	case "status", "doctor", "serve", "mcp":
		fs.BoolVar(&jsonOut, "json", false, "JSON output")
		fs.StringVar(&configDir, "config", "", "config directory override")
		fs.StringVar(&profile, "projection", "personal", "projection profile (personal|work-safe)")
		fs.StringVar(&capability, "capability", "", "capability name for serving")
		fs.StringVar(&transport, "transport", "stdio", "transport (stdio only in v1)")
		fs.BoolVar(&experimentalLearned, "experimental-learned", false, "enable non-normative learned guidance (personal only)")
	case "build":
		fs.BoolVar(&jsonOut, "json", false, "JSON output")
		fs.StringVar(&configDir, "config", "", "config directory override")
		fs.StringVar(&profile, "profile", "personal", "profile to build (personal|work-safe|all)")
	case "preview", "resolve":
		fs.BoolVar(&jsonOut, "json", false, "JSON output")
		fs.StringVar(&configDir, "config", "", "config directory override")
		fs.StringVar(&profile, "projection", "personal", "projection profile")
		fs.StringVar(&task, "task", "", "task text (required)")
		fs.StringVar(&wsHint, "workspace", "", "workspace path hint")
		fs.StringVar(&capability, "capability", "", "capability name")
	case "explain":
		fs.BoolVar(&jsonOut, "json", false, "JSON output")
		fs.StringVar(&configDir, "config", "", "config directory override")
		fs.StringVar(&profile, "projection", "personal", "projection profile")
		fs.StringVar(&task, "trace", "", "trace ID (required)")
	case "export":
		fs.BoolVar(&jsonOut, "json", false, "JSON output")
		fs.StringVar(&configDir, "config", "", "config directory override")
		fs.StringVar(&profile, "projection", "personal", "projection profile")
		fs.StringVar(&exportOut, "out", "-", "output path ('-' for stdout)")
	case "source", "profile-cmd", "forget":
		fs.BoolVar(&jsonOut, "json", false, "JSON output")
		fs.StringVar(&configDir, "config", "", "config directory override")
		fs.StringVar(&profile, "profile", "personal", "profile")
	case "purge":
		fs.BoolVar(&jsonOut, "json", false, "JSON output")
		fs.StringVar(&configDir, "config", "", "config directory override")
		fs.StringVar(&confirm, "confirm", "", "repeat the exact key to confirm this irreversible purge")
		fs.BoolVar(&removeCanonical, "remove-canonical", false, "also delete the canonical source file(s)")
		fs.BoolVar(&dryRun, "dry-run", false, "report what would be purged without changing anything")
	case "adapter", "candidate":
		// handled directly below (need raw positional args)
	default:
		usage()
		os.Exit(2)
	}
	if cmd != "adapter" {
		if err := fs.Parse(args[1:]); err != nil {
			os.Exit(2)
		}
	}

	switch cmd {
	case "status":
		rt, err := app.Load(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		emitStatus(rt, jsonOut)
	case "doctor":
		doctor(configDir, jsonOut)
	case "build":
		rt, err := app.Load(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		profiles := []contracts.Profile{contracts.ProfilePersonal, contracts.ProfileWorkSafe}
		if profile != "all" {
			profiles = []contracts.Profile{contracts.Profile(profile)}
		}
		reports := []app.BuildReport{}
		for _, p := range profiles {
			rep, err := rt.BuildProfile(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error building %s: %v\n", p, err)
				os.Exit(1)
			}
			reports = append(reports, *rep)
		}
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "ok", "reports": reports})
		} else {
			for _, r := range reports {
				fmt.Printf("built %s: %d records from %d sources\n", r.Profile, r.RecordsIngested, len(r.SourcesIngested))
				for _, s := range r.Skipped {
					fmt.Printf("  skipped: %s\n", s)
				}
				for _, s := range r.SecretRejected {
					fmt.Printf("  secret-rejected: %s\n", s)
				}
				if r.PurgeBlocked > 0 {
					fmt.Printf("  purge-blocked: %d entr(ies) refused by physical-purge tombstones (not re-ingested)\n", r.PurgeBlocked)
				}
			}
		}
	case "preview", "resolve":
		if task == "" {
			fmt.Fprintln(os.Stderr, "error: --task required")
			os.Exit(2)
		}
		rt, err := app.Load(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		capID := capability
		if capID == "" {
			capID = "cap_" + profile + "_default"
		}
		sess, err := rt.Serve(contracts.Profile(profile), capID, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer sess.Store.Close()
		req := contracts.ResolutionRequest{
			SchemaVersion: contracts.SchemaVersion,
			Task:          task,
			WorkspaceHint: wsHint,
		}
		pack, trace, err := sess.Resolve(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(3)
		}
		// FR-038: persist the actual trace so `beme explain --trace <id>`
		// explains from the real resolver run, not a reconstruction.
		PersistTrace(sess.Runtime.Config.CacheDir, pack.TraceRef, trace)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(pack)
			_ = trace
		} else {
			renderPackHuman(pack)
			fmt.Printf("\ntrace steps: %d (use explain --json for full trace)\n", len(trace))
		}
	case "forget":
		// forget --profile X <record_id|source:source_id> [reason]
		rest := fs.Args()
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: beme forget [--profile P] <record-id|source:source-id> [reason]")
			os.Exit(2)
		}
		key := rest[0]
		reason := "operator forget"
		if len(rest) > 1 {
			reason = strings.Join(rest[1:], " ")
		}
		rt, err := app.Load(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := rt.Forget(contracts.Profile(profile), key, reason); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("forgotten %s (tombstoned in the projection and the durable ledger; content stays on disk until `beme purge`)\n", key)
	case "adapter":
		adapterCmd(args[1:])
	case "explain":
		if task == "" {
			fmt.Fprintln(os.Stderr, "error: --trace required")
			os.Exit(2)
		}
		rtX, err := app.Load(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		explainCmd(rtX, task, profile, jsonOut)
	case "export":
		rtE, err := app.Load(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		exportCmd(rtE, profile, exportOut, jsonOut)
	case "candidate":
		// candidate keeps raw positional arguments (observation IDs), so its
		// deployment override is extracted here instead of through the flag
		// set: without this it would silently fall back to the operator's
		// real deployment (isolation rule, ADR-026).
		rest, candidateConfig := extractConfigFlag(args[1:])
		rtC, err := app.Load(candidateConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		candidateCmd(rtC, rest)
	case "purge":
		// purge --confirm KEY [--remove-canonical] [--dry-run] [--json] KEY
		rest := fs.Args()
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "usage: beme purge --confirm <key> [--remove-canonical] [--dry-run] [--json] <record-id|source:source-id>")
			os.Exit(2)
		}
		rt, err := app.Load(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		rep, err := rt.PhysicalPurge(app.PurgeRequest{Key: rest[0], Confirm: confirm, RemoveCanonical: removeCanonical, DryRun: dryRun})
		switch {
		case errors.Is(err, app.ErrPurgeNotConfirmed):
			fmt.Fprintf(os.Stderr, "policy blocked: %v\n", err)
			os.Exit(3)
		case errors.Is(err, app.ErrPurgeNotFound):
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(4)
		case err != nil:
			code := exitForReadErr(err)
			resumable := rt.PendingPurges() > 0
			if jsonOut {
				json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "error", "error": err.Error(), "resumable": resumable, "report": rep})
			} else {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				if rep != nil && len(rep.Steps) > 0 {
					fmt.Fprintf(os.Stderr, "partial purge of %s — steps completed before the failure:\n", rep.Key)
					for _, st := range rep.Steps {
						fmt.Fprintf(os.Stderr, "  %-36s %-8s %d\n", st.Step, st.Outcome, st.Count)
					}
				}
				if resumable {
					fmt.Fprintln(os.Stderr, "the purge is resumable: re-run the same command to finish the remaining cleanup")
				}
			}
			os.Exit(code)
		}
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "ok", "report": rep})
			return
		}
		if rep.AlreadyPurged {
			fmt.Printf("already purged: %s (nothing left to erase)\n", rep.Key)
			return
		}
		mode := "purged"
		if rep.DryRun {
			mode = "dry run — would purge"
		}
		if rep.Resumed {
			mode += " (resumed an interrupted purge)"
		}
		fmt.Printf("%s %d record(s) for %s\n", mode, rep.Records, rep.Key)
		for _, st := range rep.Steps {
			fmt.Printf("  %-36s %-8s %d\n", st.Step, st.Outcome, st.Count)
		}
		for _, r := range rep.Residuals {
			fmt.Printf("  residual: %s\n", r)
		}
	case "serve", "mcp":
		if err := app.ValidateTransport(transport); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(3)
		}
		runMCPServer(configDir, profile, capability, strconv.FormatBool(experimentalLearned))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `beme %s — personal execution-context runtime

Usage:
  beme status [--json] [--config DIR]
  beme doctor [--json] [--config DIR]
  beme build --profile personal|work-safe|all [--json] [--config DIR]
  beme preview --task TEXT [--workspace PATH] [--projection P] [--json]
  beme resolve  (alias of preview)
  beme forget [--profile P] <record-id|source:source-id> [reason]
  beme purge --confirm KEY [--remove-canonical] [--dry-run] [--json] KEY
        physical purge (RED, irreversible): erase from projections, traces,
        observations; leave a non-content anti-resurrection tombstone
  beme serve --projection P --capability NAME --transport stdio
  beme mcp      (alias of serve)

Exit codes: 0 ok · 1 failure · 2 usage · 3 policy-blocked · 4 not-found
`, version)
}

func emitStatus(rt *app.Runtime, jsonOut bool) {
	type srcLine struct {
		SourceID    string `json:"source_id"`
		Root        string `json:"root"`
		Trust       string `json:"trust"`
		Profiles    string `json:"profiles_allowed"`
		Sensitivity string `json:"sensitivity"`
	}
	srcs := []srcLine{}
	for _, s := range rt.Sources {
		srcs = append(srcs, srcLine{s.SourceID, s.Root, string(s.Trust), strings.Join(profilesToStrs(s.ProfilesAllowed), ","), s.Sensitivity})
	}
	if jsonOut {
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"status":     "ok",
			"version":    version,
			"sources":    srcs,
			"workspaces": len(rt.Registry.Workspaces),
		})
		return
	}
	fmt.Printf("beme %s\n", version)
	fmt.Printf("registered sources: %d\n", len(srcs))
	for _, s := range srcs {
		fmt.Printf("  %-24s trust=%-14s profiles=%-12s sensitivity=%s\n", s.SourceID, s.Trust, s.Profiles, s.Sensitivity)
	}
	fmt.Printf("registered workspaces: %d\n", len(rt.Registry.Workspaces))
}

func doctor(configDir string, jsonOut bool) {
	rt, err := app.Load(configDir)
	findings := []string{}
	status := "healthy"
	// raise keeps the most severe health state: a later, milder finding
	// (e.g. a pending purge) never masks policy_blocked or unavailable.
	raise := func(next string) {
		if healthSeverity[next] > healthSeverity[status] {
			status = next
		}
	}
	if err != nil {
		raise("unavailable")
		findings = append(findings, "config load failed: "+err.Error())
	} else {
		if len(rt.Sources) == 0 {
			raise("degraded")
			findings = append(findings, "no registered sources; register sources under <config>/sources/")
		}
		for _, s := range rt.Sources {
			if _, err := os.Stat(s.Root); err != nil {
				raise("degraded")
				findings = append(findings, fmt.Sprintf("source %s root missing: %s", s.SourceID, s.Root))
			}
		}
		for _, p := range []string{"personal", "work-safe"} {
			if _, err := os.Stat(rt.ProjectionPath(contracts.Profile(p))); err != nil {
				findings = append(findings, fmt.Sprintf("projection %s not built yet (run: beme build --profile %s)", p, p))
			}
		}
		for _, p := range []contracts.Profile{contracts.ProfilePersonal, contracts.ProfileWorkSafe} {
			pf, err := rt.ProjectionFindings(p)
			if err != nil {
				raise("policy_blocked")
				findings = append(findings, fmt.Sprintf("projection %s cannot be read safely: %v", p, err))
				continue
			}
			if len(pf) > 0 {
				raise("degraded")
			}
			findings = append(findings, pf...)
		}
		if of, err := rt.ObservationFindings(); err != nil {
			raise("policy_blocked")
			findings = append(findings, fmt.Sprintf("observation store cannot be read safely: %v", err))
		} else if len(of) > 0 {
			raise("degraded")
			findings = append(findings, of...)
		}
		if n := rt.PendingPurges(); n > 0 {
			raise("degraded")
			findings = append(findings, fmt.Sprintf("%d interrupted physical purge(s) pending: re-run the same `beme purge --confirm <key> <key>` command to finish the cleanup", n))
		}
	}
	if jsonOut {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"status": status, "findings": findings, "checked_at": time.Now().UTC().Format(time.RFC3339)})
		return
	}
	fmt.Printf("doctor: %s\n", status)
	for _, f := range findings {
		fmt.Printf("  - %s\n", f)
	}
}

// extractConfigFlag removes "--config DIR" / "--config=DIR" from raw
// arguments and returns the remaining arguments and the directory.
func extractConfigFlag(args []string) (rest []string, configDir string) {
	rest = []string{}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--config" || args[i] == "-config":
			if i+1 < len(args) {
				i++
				configDir = args[i]
			}
		case strings.HasPrefix(args[i], "--config="), strings.HasPrefix(args[i], "-config="):
			_, configDir, _ = strings.Cut(args[i], "=")
		default:
			rest = append(rest, args[i])
		}
	}
	return rest, configDir
}

// healthSeverity orders doctor states (§23.1); doctor reports the most severe.
var healthSeverity = map[string]int{"healthy": 0, "degraded": 1, "rebuild_required": 2, "policy_blocked": 3, "unavailable": 4}

func renderPackHuman(pack any) {
	b, _ := json.MarshalIndent(pack, "", "  ")
	// compact human view: section counts + constraint text
	var p struct {
		SchemaVersion string `json:"schema_version"`
		PackID        string `json:"pack_id"`
		Resolution    struct {
			Profile      string `json:"profile"`
			Completeness string `json:"completeness"`
		} `json:"resolution"`
		Constraints []struct {
			Text string `json:"text"`
		} `json:"constraints"`
		Guidance []struct {
			Text string `json:"text"`
		} `json:"guidance"`
		Precedents []struct {
			Text string `json:"text"`
		} `json:"precedents"`
		Unknowns []struct {
			Question string `json:"question"`
		} `json:"unknowns"`
		Conflicts []struct {
			Description string `json:"description"`
		} `json:"conflicts"`
	}
	json.Unmarshal(b, &p)
	fmt.Printf("pack %s (profile=%s completeness=%s)\n", p.PackID, p.Resolution.Profile, p.Resolution.Completeness)
	fmt.Printf("constraints: %d\n", len(p.Constraints))
	for i, c := range p.Constraints {
		fmt.Printf("  %d. MUST: %s\n", i+1, c.Text)
	}
	fmt.Printf("guidance: %d\n", len(p.Guidance))
	for i, g := range p.Guidance {
		fmt.Printf("  %d. %s\n", i+1, g.Text)
	}
	fmt.Printf("precedents: %d\n", len(p.Precedents))
	for i, pr := range p.Precedents {
		fmt.Printf("  %d. %s\n", i+1, pr.Text)
	}
	fmt.Printf("unknowns: %d\n", len(p.Unknowns))
	for _, u := range p.Unknowns {
		fmt.Printf("  ? %s\n", u.Question)
	}
	fmt.Printf("conflicts: %d\n", len(p.Conflicts))
	for _, c := range p.Conflicts {
		fmt.Printf("  ! %s\n", c.Description)
	}
	_ = strconv.Itoa
}
