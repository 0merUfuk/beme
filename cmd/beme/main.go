// Command beme is the administrative CLI (blueprint §12.2): source and
// profile lifecycle, preview, explain, doctor, forget, serve.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/storage"
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
	case "source", "profile-cmd", "forget", "explain":
		fs.BoolVar(&jsonOut, "json", false, "JSON output")
		fs.StringVar(&configDir, "config", "", "config directory override")
		fs.StringVar(&profile, "profile", "personal", "profile")
	case "adapter":
		// handled directly below (needs raw positional args)
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
			fmt.Fprintf(os.Stderr, "error: %v\\n", err)
			os.Exit(1)
		}
		emitStatus(rt, jsonOut)
	case "doctor":
		doctor(configDir, jsonOut)
	case "build":
		rt, err := app.Load(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\\n", err)
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
				fmt.Fprintf(os.Stderr, "error building %s: %v\\n", p, err)
				os.Exit(1)
			}
			reports = append(reports, *rep)
		}
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "ok", "reports": reports})
		} else {
			for _, r := range reports {
				fmt.Printf("built %s: %d records from %d sources\\n", r.Profile, r.RecordsIngested, len(r.SourcesIngested))
				for _, s := range r.Skipped {
					fmt.Printf("  skipped: %s\\n", s)
				}
				for _, s := range r.SecretRejected {
					fmt.Printf("  secret-rejected: %s\\n", s)
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
			fmt.Fprintf(os.Stderr, "error: %v\\n", err)
			os.Exit(1)
		}
		capID := capability
		if capID == "" {
			capID = "cap_" + profile + "_default"
		}
		sess, err := rt.Serve(contracts.Profile(profile), capID, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\\n", err)
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
			fmt.Fprintf(os.Stderr, "error: %v\\n", err)
			os.Exit(3)
		}
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(pack)
			_ = trace
		} else {
			renderPackHuman(pack)
			fmt.Printf("\\ntrace steps: %d (use explain --json for full trace)\\n", len(trace))
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
			fmt.Fprintf(os.Stderr, "error: %v\\n", err)
			os.Exit(1)
		}
		store, err := storage.Open(rt.ProjectionPath(contracts.Profile(profile)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\\n", err)
			os.Exit(1)
		}
		defer store.Close()
		if err := store.Tombstone(key, reason); err != recErrNone(err) {
			fmt.Fprintf(os.Stderr, "error: %v\\n", err)
			os.Exit(1)
		}
		fmt.Printf("tombstoned %s (logical forget; derived purge happens on next rebuild)\\n", key)
	case "adapter":
		adapterCmd(args[1:])
	case "serve", "mcp":
		if transport != "stdio" {
			fmt.Fprintf(os.Stderr, "error: only stdio transport is supported in v1 (ADR-014)\\n")
			os.Exit(3)
		}
		runMCPServer(configDir, profile, capability, strconv.FormatBool(experimentalLearned))
	default:
		usage()
		os.Exit(2)
	}
}

func recErrNone(err error) error { return err }

func usage() {
	fmt.Fprintf(os.Stderr, `beme %s — personal execution-context runtime

Usage:
  beme status [--json] [--config DIR]
  beme doctor [--json] [--config DIR]
  beme build --profile personal|work-safe|all [--json] [--config DIR]
  beme preview --task TEXT [--workspace PATH] [--projection P] [--json]
  beme resolve  (alias of preview)
  beme forget [--profile P] <record-id|source:source-id> [reason]
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
	fmt.Printf("beme %s\\n", version)
	fmt.Printf("registered sources: %d\\n", len(srcs))
	for _, s := range srcs {
		fmt.Printf("  %-24s trust=%-14s profiles=%-12s sensitivity=%s\\n", s.SourceID, s.Trust, s.Profiles, s.Sensitivity)
	}
	fmt.Printf("registered workspaces: %d\\n", len(rt.Registry.Workspaces))
}

func doctor(configDir string, jsonOut bool) {
	rt, err := app.Load(configDir)
	findings := []string{}
	status := "healthy"
	if err != nil {
		status = "unavailable"
		findings = append(findings, "config load failed: "+err.Error())
	} else {
		if len(rt.Sources) == 0 {
			status = "degraded"
			findings = append(findings, "no registered sources; register sources under <config>/sources/")
		}
		for _, s := range rt.Sources {
			if _, err := os.Stat(s.Root); err != nil {
				status = "degraded"
				findings = append(findings, fmt.Sprintf("source %s root missing: %s", s.SourceID, s.Root))
			}
		}
		for _, p := range []string{"personal", "work-safe"} {
			if _, err := os.Stat(rt.ProjectionPath(contracts.Profile(p))); err != nil {
				findings = append(findings, fmt.Sprintf("projection %s not built yet (run: beme build --profile %s)", p, p))
			}
		}
	}
	if jsonOut {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"status": status, "findings": findings, "checked_at": time.Now().UTC().Format(time.RFC3339)})
		return
	}
	fmt.Printf("doctor: %s\\n", status)
	for _, f := range findings {
		fmt.Printf("  - %s\\n", f)
	}
}

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
	fmt.Printf("pack %s (profile=%s completeness=%s)\\n", p.PackID, p.Resolution.Profile, p.Resolution.Completeness)
	fmt.Printf("constraints: %d\\n", len(p.Constraints))
	for i, c := range p.Constraints {
		fmt.Printf("  %d. MUST: %s\\n", i+1, c.Text)
	}
	fmt.Printf("guidance: %d\\n", len(p.Guidance))
	for i, g := range p.Guidance {
		fmt.Printf("  %d. %s\\n", i+1, g.Text)
	}
	fmt.Printf("precedents: %d\\n", len(p.Precedents))
	for i, pr := range p.Precedents {
		fmt.Printf("  %d. %s\\n", i+1, pr.Text)
	}
	fmt.Printf("unknowns: %d\\n", len(p.Unknowns))
	for _, u := range p.Unknowns {
		fmt.Printf("  ? %s\\n", u.Question)
	}
	fmt.Printf("conflicts: %d\\n", len(p.Conflicts))
	for _, c := range p.Conflicts {
		fmt.Printf("  ! %s\\n", c.Description)
	}
	_ = strconv.Itoa
}
