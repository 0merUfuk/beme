package evalrunner

// Prompt rendering. The prompt is the exact text a model is given, rendered
// deterministically from the arm's inputs:
//
//	=== BOOTSTRAP ===        (omitted for B0)
//	<bootstrap text>
//	=== END BOOTSTRAP ===
//
//	=== CONTEXT ===          (omitted for B0 and B1)
//	<indented JSON context view>
//	=== END CONTEXT ===
//
//	=== TASK ===
//	<task>
//	=== END TASK ===
//
// The context view is the arm's context minus per-issuance identifiers
// (pack_id, generated_at, trace_ref, expand_ref), so identical inputs render
// byte-identical prompts. The arm label never appears.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/0merUfuk/beme/internal/resolver"
)

// RenderPrompt renders the model prompt for one generation.
func RenderPrompt(task, bootstrapText string, ctx ArmContext) (string, error) {
	blocks := []string{}
	if bootstrapText != "" {
		blocks = append(blocks, promptBlock("BOOTSTRAP", bootstrapText))
	}
	view, err := RenderContext(ctx)
	if err != nil {
		return "", err
	}
	if view != "" {
		blocks = append(blocks, promptBlock("CONTEXT", view))
	}
	blocks = append(blocks, promptBlock("TASK", task))
	return strings.Join(blocks, "\n\n") + "\n", nil
}

// RenderContext renders the deterministic context view ("" for ShapeNone).
func RenderContext(ctx ArmContext) (string, error) {
	var v any
	switch ctx.Shape {
	case ShapeRecords:
		v = struct {
			Records []promptItem `json:"records"`
		}{promptItems(ctx.Records)}
	case ShapePack:
		if ctx.Pack == nil {
			return "", nil
		}
		v = promptPackView(*ctx.Pack)
	default:
		return "", nil
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func promptBlock(name, body string) string {
	return "=== " + name + " ===\n" + strings.TrimRight(body, "\n") + "\n=== END " + name + " ==="
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(h[:])
}

type promptItem struct {
	RecordID        string   `json:"record_id"`
	Kind            string   `json:"kind"`
	Text            string   `json:"text"`
	Force           string   `json:"force,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	Authority       string   `json:"authority,omitempty"`
	Sensitivity     string   `json:"sensitivity,omitempty"`
	SelectionReason []string `json:"selection_reason,omitempty"`
	ProvenanceRefs  []string `json:"provenance_refs,omitempty"`
}

type promptKnowledge struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Locator        *string  `json:"locator,omitempty"`
	ProvenanceRefs []string `json:"provenance_refs,omitempty"`
}

type promptPack struct {
	Profile              string                     `json:"profile"`
	Completeness         string                     `json:"completeness"`
	PolicyDigest         string                     `json:"policy_digest"`
	IndexRevision        string                     `json:"index_revision"`
	SourceRevisionDigest *string                    `json:"source_revision_digest,omitempty"`
	TaskKinds            []string                   `json:"task_kinds"`
	Constraints          []promptItem               `json:"constraints"`
	Guidance             []promptItem               `json:"guidance"`
	Precedents           []promptItem               `json:"precedents"`
	LearnedExperimental  []promptItem               `json:"learned_experimental,omitempty"`
	Knowledge            []promptKnowledge          `json:"knowledge,omitempty"`
	Unknowns             []resolver.Unknown         `json:"unknowns"`
	Conflicts            []resolver.PackConflict    `json:"conflicts"`
	Assumptions          []string                   `json:"assumptions"`
	Degradations         []resolver.PackDegradation `json:"degradations"`
	Provenance           []resolver.PackProvenance  `json:"provenance,omitempty"`
	Budget               resolver.PackBudget        `json:"budget"`
}

func promptItems(items []resolver.ContextItem) []promptItem {
	out := make([]promptItem, 0, len(items))
	for _, it := range items {
		out = append(out, promptItem{
			RecordID: it.RecordID, Kind: it.Kind, Text: it.Text, Force: it.Force,
			Confidence: it.Confidence, Authority: it.Authority, Sensitivity: it.Sensitivity,
			SelectionReason: it.SelectionReason, ProvenanceRefs: it.ProvenanceRefs,
		})
	}
	return out
}

func promptPackView(p resolver.Pack) promptPack {
	v := promptPack{
		Profile:              p.Resolution.Profile,
		Completeness:         p.Resolution.Completeness,
		PolicyDigest:         p.Resolution.PolicyDigest,
		IndexRevision:        p.Resolution.IndexRevision,
		SourceRevisionDigest: p.Resolution.SourceRevisionDigest,
		TaskKinds:            nonNil(p.Request.TaskKinds),
		Constraints:          promptItems(p.Constraints),
		Guidance:             promptItems(p.Guidance),
		Precedents:           promptItems(p.Precedents),
		Unknowns:             nonNil(p.Unknowns),
		Conflicts:            nonNil(p.Conflicts),
		Assumptions:          nonNil(p.Assumptions),
		Degradations:         nonNil(p.Degradations),
		Budget:               p.Budget,
	}
	if len(p.LearnedExperimental) > 0 {
		v.LearnedExperimental = promptItems(p.LearnedExperimental)
	}
	for _, k := range p.Knowledge {
		v.Knowledge = append(v.Knowledge, promptKnowledge{Title: k.Title, Description: k.Description, Locator: k.Locator, ProvenanceRefs: k.ProvenanceRefs})
	}
	if len(p.Provenance) > 0 {
		prov := append([]resolver.PackProvenance{}, p.Provenance...)
		// the resolver orders provenance by role only; ties need a total order
		sort.SliceStable(prov, func(i, j int) bool {
			if prov[i].SourceRole != prov[j].SourceRole {
				return prov[i].SourceRole < prov[j].SourceRole
			}
			if a, b := deref(prov[i].SourceID), deref(prov[j].SourceID); a != b {
				return a < b
			}
			return prov[i].RecordCount < prov[j].RecordCount
		})
		v.Provenance = prov
	}
	return v
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
