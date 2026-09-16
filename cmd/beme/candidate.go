package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
)

// candidateCmd implements the §12.2 review surface:
//
//	beme candidate list [--status quarantined] [--json]
//	beme candidate inspect <id> [--json]
//	beme candidate review <id> --action <approve|edit|merge|reject|defer|situational|scope_limit|counterexample> [--note TEXT] [--json]
//
// Promotion note (FR-051, ADR-010): `approve` records the decision and
// marks the observation approved; the canonical knowledge write itself is
// the separate trusted proposal path in the owning repository (canonical
// knowledge entries / project docs). Be Me never writes canonical knowledge.
func candidateCmd(rt *app.Runtime, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, `usage: beme candidate list [--status S] [--json]
       beme candidate inspect <id> [--json]
       beme candidate review <id> --action <approve|edit|merge|reject|defer|situational|scope_limit|counterexample> [--note TEXT] [--json]`)
		os.Exit(2)
	}
	verb := args[0]
	rest := args[1:]

	jsonOut, status, action, note := false, "", "", ""
	id := ""
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--json":
			jsonOut = true
		case "--status":
			if i+1 < len(rest) {
				i++
				status = rest[i]
			}
		case "--action":
			if i+1 < len(rest) {
				i++
				action = rest[i]
			}
		case "--note":
			if i+1 < len(rest) {
				i++
				note = rest[i]
			}
		default:
			if id == "" && !strings.HasPrefix(rest[i], "--") {
				id = rest[i]
			}
		}
	}

	store, err := rt.OpenLearning()
	if errors.Is(err, app.ErrLedgerUnusable) {
		fmt.Fprintf(os.Stderr, "policy blocked: %v\n", err)
		os.Exit(3)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch verb {
	case "list":
		if status == "" {
			status = "quarantined"
		}
		obs := store.List(status)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]any{"status_filter": status, "count": len(obs), "observations": obs})
			return
		}
		if len(obs) == 0 {
			fmt.Printf("no observations with status %q\n", status)
			return
		}
		for _, o := range obs {
			marker := " "
			if o.FamilyCount > 1 {
				marker = fmt.Sprintf("×%d", o.FamilyCount)
			}
			fmt.Printf("%s %-24s %-6s %-40s family=%s sensitivity=%s\n", marker, o.ObservationID, o.Kind, truncateRunes(o.Hypothesis, 38), o.EvidenceFamily, o.InheritedSensitivity)
		}
		fmt.Printf("\nreview with: beme candidate review <id> --action <approve|edit|merge|reject|defer|situational|scope_limit|counterexample> [--note TEXT]\n")
	case "inspect":
		if id == "" {
			fmt.Fprintln(os.Stderr, "error: observation id required")
			os.Exit(2)
		}
		o, err := store.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(4)
		}
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(o)
			return
		}
		b, _ := json.MarshalIndent(o, "", "  ")
		fmt.Println(string(b))
	case "review":
		if id == "" || action == "" {
			fmt.Fprintln(os.Stderr, "error: review requires <id> and --action")
			os.Exit(2)
		}
		o, err := store.Review(id, action, "operator", note)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(o)
			return
		}
		fmt.Printf("observation %s → %s\n", o.ObservationID, o.Status)
		if action == "approve" {
			fmt.Println("note: approval recorded; the canonical knowledge write belongs to the")
			fmt.Println("owning repository's proposal path (user-owned). Be Me does not write canonical knowledge.")
		}
		if action == "reject" && o.ReviewOutcome != nil {
			fmt.Printf("tombstone: %s (equivalent re-proposals will be refused)\n", o.ReviewOutcome.Fingerprint)
		}
	default:
		fmt.Fprintf(os.Stderr, "error: unknown candidate verb %q\n", verb)
		os.Exit(2)
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// observationSensitivityFor derives the inherited sensitivity for feedback
// captured under a capability (FR-054: work-restricted feedback never
// silently becomes global personal knowledge).
func observationSensitivityFor(profile contracts.Profile) string {
	switch profile {
	case contracts.ProfileWorkSafe:
		return "public_general" // work-safe feedback saw only declassified content
	default:
		return "personal_private"
	}
}
