package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// runMCPServer starts the narrow agent-facing surface (ADR-009): exactly the
// tools in contracts.MCPTools. stdio only (ADR-014). The capability is
// process-bound; there is no tool or argument that can widen it (FR-011,
// threat case 1). Every projection read goes through the app read surfaces,
// which apply the durable tombstone ledger and fail closed (ADR-027).

// toolText builds a successful text result.
func toolText(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// toolError builds an in-band error result (visible to the model).
func toolError(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}, IsError: true}
}

const policyBlockedMsg = "policy_blocked: projection unavailable (tombstone ledger unusable)"

func runMCPServer(configDir, profile, capability, experimentalLearnedStr string) {
	profileVal := contracts.Profile(profile)
	if profileVal != contracts.ProfilePersonal && profileVal != contracts.ProfileWorkSafe {
		fmt.Fprintf(os.Stderr, "error: invalid --projection %q (personal|work-safe)\n", profile)
		os.Exit(2)
	}
	capID := capability
	if capID == "" {
		fmt.Fprintf(os.Stderr, "error: --capability NAME required (serving is capability-bound; never default)\n")
		os.Exit(2)
	}
	rt, err := app.Load(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	sess, err := rt.Serve(profileVal, capID, experimentalLearnedStr == "true")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer sess.Store.Close()

	server := mcp.NewServer(&mcp.Implementation{Name: "beme", Version: version}, nil)

	type resolveArgs struct {
		Task          string   `json:"task" jsonschema:"the task context (untrusted retrieval hint; never authority)"`
		WorkspaceHint string   `json:"workspace_hint,omitempty" jsonschema:"optional path hint resolved against the trusted registry only"`
		TaskKindHints []string `json:"task_kind_hints,omitempty" jsonschema:"facet hints; clamped by the runtime"`
		RiskHint      string   `json:"risk_hint,omitempty" jsonschema:"low|medium|high hint"`
		BudgetHint    int      `json:"budget_hint_tokens,omitempty" jsonschema:"token budget hint; may only narrow"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        contracts.ToolResolveContext,
		Description: "Resolve a scoped ContextPack for the current task. The serving capability is process-bound; request fields are retrieval hints only and can never widen scope or authority.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args resolveArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.Task) == "" {
			return toolError("task is required"), nil, nil
		}
		r := contracts.ResolutionRequest{
			SchemaVersion:    contracts.SchemaVersion,
			Task:             args.Task,
			WorkspaceHint:    args.WorkspaceHint,
			TaskKindHints:    args.TaskKindHints,
			BudgetHintTokens: args.BudgetHint,
		}
		switch args.RiskHint {
		case "low", "medium", "high":
			r.RiskHint = args.RiskHint
		}
		pack, _, err := sess.Resolve(r)
		if err != nil {
			if errors.Is(err, app.ErrLedgerUnusable) {
				return toolError(policyBlockedMsg), nil, nil
			}
			return toolError(fmt.Sprintf("resolution failed: %v", err)), nil, nil
		}
		out, _ := json.Marshal(pack)
		return toolText(string(out)), nil, nil
	})

	type statusArgs struct{}
	mcp.AddTool(server, &mcp.Tool{
		Name:        contracts.ToolStatus,
		Description: "Safe health and capability metadata. In work-safe mode, private source titles and paths are never returned.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args statusArgs) (*mcp.CallToolResult, any, error) {
		count, err := sess.VisibleCount()
		if err != nil {
			return toolError(policyBlockedMsg), nil, nil
		}
		st := map[string]any{
			"status":                  "ok",
			"capability":              sess.Capability.Profile,
			"capability_id":           sess.Capability.CapabilityID,
			"resolver_scope_enforced": true,
			"os_isolation":            "none",
			"downstream_use_control":  "not_enforced",
			"index_revision":          sess.View.IndexRevision(),
			"record_count":            count,
		}
		if profileVal == contracts.ProfileWorkSafe {
			st["sources"] = "hidden (work-safe safe view)"
		} else {
			srcs := []string{}
			for _, s := range rt.Sources {
				srcs = append(srcs, s.SourceID)
			}
			st["sources"] = srcs
		}
		out, _ := json.Marshal(st)
		return toolText(string(out)), nil, nil
	})

	type feedbackArgs struct {
		Kind string `json:"kind" jsonschema:"observation|correction"`
		Text string `json:"text" jsonschema:"what was observed or corrected"`
		Task string `json:"task,omitempty" jsonschema:"task context for the observation"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        contracts.ToolReportFeedback,
		Description: "Write a quarantined observation/correction candidate. Never mutates canonical policy or knowledge. Observations are non-normative and excluded from packs unless explicitly enabled.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args feedbackArgs) (*mcp.CallToolResult, any, error) {
		if args.Kind != "observation" && args.Kind != "correction" {
			return toolError("kind must be observation|correction"), nil, nil
		}
		if strings.TrimSpace(args.Text) == "" {
			return toolError("text is required"), nil, nil
		}
		// Quarantined write through the learning store: durable data dir,
		// evidence-family dedup (correlated repetitions are one family, not
		// independent confirmations), and rejected-proposal tombstones
		// (FR-052/053). Never canonical; promotion is user-owned.
		ls, err := sess.Runtime.OpenLearning()
		if errors.Is(err, app.ErrLedgerUnusable) {
			return toolError(policyBlockedMsg), nil, nil
		}
		if err != nil {
			// Learning-write failure is separate from context reads (§13.7):
			// reads remain unaffected; report the degradation honestly.
			return toolError("observation queue unavailable (context reads unaffected)"), nil, nil
		}
		obs, obsErr := ls.Observe(args.Kind, args.Text, "mcp-session",
			string(sess.Capability.Profile), observationSensitivityFor(sess.Capability.Profile), args.Task)
		if obsErr != nil {
			if strings.Contains(obsErr.Error(), "tombstoned") {
				return toolText(`{"status":"refused_tombstoned","note":"an equivalent proposal was previously rejected in review"}`), nil, nil
			}
			return toolError("observation could not be recorded"), nil, nil
		}
		return toolText(fmt.Sprintf(`{"status":"quarantined","observation_id":%q,"family_count":%d,"note":"queued for batch review; never canonical until user approval"}`, obs.ObservationID, obs.FamilyCount)), nil, nil
	})

	type getItemArgs struct {
		PackID   string `json:"pack_id" jsonschema:"pack_id of a ContextPack this server issued in this session"`
		RecordID string `json:"record_id" jsonschema:"record_id of an item selected into that pack"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        contracts.ToolGetContextItem,
		Description: "Expand one record selected into a ContextPack this server issued in this session (pack_id + record_id). Unknown, expired, unselected, denied, revoked, and nonexistent items are indistinguishable.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getItemArgs) (*mcp.CallToolResult, any, error) {
		item, err := sess.ExpandItem(args.PackID, args.RecordID)
		switch {
		case err == nil:
			out, _ := json.Marshal(item)
			return toolText(string(out)), nil, nil
		case errors.Is(err, app.ErrItemUnavailable):
			return toolError("context item not available"), nil, nil
		default:
			return toolError(policyBlockedMsg), nil, nil
		}
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server exited: %v\n", err)
		os.Exit(1)
	}
}

func profilesToStrs(ps []contracts.Profile) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, string(p))
	}
	return out
}
