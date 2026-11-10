// Package setup turns a short interview into a user's personal working rules.
//
// One answers file is the single source of truth: the always-on instruction
// block written into harness files and the Be Me rule entries are both
// rendered from it, so they cannot drift. The package is pure (no
// filesystem, no network); cmd/beme performs the guarded writes.
package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
)

const (
	SchemaVersion = 1
	MaxCustom     = 3
	MaxCustomLen  = 200
	MinDocLines   = 20
	MaxDocLines   = 300
	// BlockBegin/BlockEnd delimit the rules block in harness instruction
	// files. They are distinct from the bootstrap and brain markers so each
	// region installs, verifies and removes independently.
	BlockBegin = "<!-- BEGIN beme-rules:managed -->"
	BlockEnd   = "<!-- END beme-rules:managed -->"
)

// Gates are the actions an agent must confirm before taking.
var allGates = []string{"purchase", "public-post", "dangerous-deletion"}

var gateText = map[string]string{
	"purchase":           "a purchase",
	"public-post":        "a post to a public platform",
	"dangerous-deletion": "a dangerous deletion (database, instance)",
}

// Harnesses with a managed instruction file; others get printed text only.
var KnownHarnesses = []string{"claude-code", "codex", "cursor", "hermes"}

// Answers is the complete, validated interview result.
type Answers struct {
	SchemaVersion     int      `json:"schema_version"`
	Evidence          bool     `json:"evidence_before_done"`
	OneRecommendation bool     `json:"one_recommendation"`
	Style             string   `json:"style"`      // short | balanced
	Autonomy          string   `json:"autonomy"`   // act | ask
	Gates             []string `json:"ask_before"` // subset of allGates (autonomy=act)
	FailureReporting  bool     `json:"report_failures"`
	DocMaxLines       int      `json:"review_doc_max_lines"` // 0 = no limit
	Custom            []string `json:"custom_rules"`
}

// Defaults returns the recommended answers (the rule set Be Me's own author
// measured and ships as its reference).
func Defaults() Answers {
	return Answers{
		SchemaVersion: SchemaVersion, Evidence: true, OneRecommendation: true,
		Style: "short", Autonomy: "act", Gates: append([]string(nil), allGates...),
		FailureReporting: true, DocMaxLines: 60,
	}
}

// Parse decodes and validates an answers document. Unknown keys are rejected
// so a typo cannot silently drop a rule.
func Parse(data []byte) (Answers, error) {
	// Omitted keys keep their recommended defaults, including ask_before: a
	// missing key must never silently remove the confirmation gates.
	a := Defaults()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return Answers{}, fmt.Errorf("answers: %w", err)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return Answers{}, fmt.Errorf("answers: trailing data after the JSON document")
	}
	if err := a.Validate(); err != nil {
		return Answers{}, err
	}
	return a, nil
}

// Validate enforces closed vocabularies and bounds. Custom rule text becomes
// instruction text in the user's own agent context, so it is constrained to
// one short printable line that cannot forge a managed-region marker.
func (a *Answers) Validate() error {
	if a.SchemaVersion != SchemaVersion {
		return fmt.Errorf("answers: schema_version must be %d", SchemaVersion)
	}
	if a.Style != "short" && a.Style != "balanced" {
		return fmt.Errorf("answers: style must be short or balanced")
	}
	if a.Autonomy != "act" && a.Autonomy != "ask" {
		return fmt.Errorf("answers: autonomy must be act or ask")
	}
	if err := ValidateGates(a.Gates); err != nil {
		return err
	}
	if err := ValidateDocLines(a.DocMaxLines); err != nil {
		return err
	}
	if len(a.Custom) > MaxCustom {
		return fmt.Errorf("answers: at most %d custom rules", MaxCustom)
	}
	for i, c := range a.Custom {
		if err := validCustom(c); err != nil {
			return fmt.Errorf("answers: custom rule %d: %w", i+1, err)
		}
	}
	return nil
}

// ValidateGates checks the confirmation-gate list against the closed vocabulary.
func ValidateGates(gates []string) error {
	seen := map[string]bool{}
	for _, g := range gates {
		if _, ok := gateText[g]; !ok {
			return fmt.Errorf("answers: unknown gate %q (allowed: %s)", g, strings.Join(allGates, ", "))
		}
		if seen[g] {
			return fmt.Errorf("answers: duplicate gate %q", g)
		}
		seen[g] = true
	}
	return nil
}

// ValidateDocLines checks the review-document length limit (0 = no limit).
func ValidateDocLines(n int) error {
	if n != 0 && (n < MinDocLines || n > MaxDocLines) {
		return fmt.Errorf("answers: review_doc_max_lines must be 0 or %d..%d", MinDocLines, MaxDocLines)
	}
	return nil
}

// ValidateCustom checks one custom rule line.
func ValidateCustom(s string) error { return validCustom(s) }

func validCustom(s string) error {
	t := strings.TrimSpace(s)
	if t == "" {
		return fmt.Errorf("empty")
	}
	if len([]rune(t)) > MaxCustomLen {
		return fmt.Errorf("longer than %d characters", MaxCustomLen)
	}
	for _, r := range t {
		if unicode.IsControl(r) {
			return fmt.Errorf("control characters are not allowed (one line only)")
		}
	}
	low := strings.ToLower(t)
	if strings.Contains(low, "<!--") || strings.Contains(low, "-->") || strings.Contains(low, "beme:managed") ||
		strings.Contains(low, "beme-rules") || strings.Contains(low, "beme-brain") {
		return fmt.Errorf("must not contain comment or managed-region markers")
	}
	return nil
}

// Rule is one rendered rule.
type Rule struct {
	ID    string // RULE-001...
	Title string
	Text  string
}

// Rules renders the ordered rule list from the answers.
func Rules(a Answers) []Rule {
	var texts []string
	if a.Evidence {
		texts = append(texts, "Never say done, fixed or passing without this turn's command output as proof.")
	}
	if a.OneRecommendation {
		texts = append(texts, "For a delegated decision, give one recommendation with reasoning, not a menu.")
	}
	if a.Style == "short" {
		texts = append(texts, "Be short and direct. No preamble, no recap. Give full detail only when asked.")
	} else {
		texts = append(texts, "Be clear and well organised, and keep each answer as short as its content allows.")
	}
	if a.FailureReporting {
		texts = append(texts, "Report failed or skipped steps plainly, with their output. Never present them as done.")
	}
	if a.Autonomy == "act" {
		g := orderedGates(a.Gates)
		if len(g) == 0 {
			texts = append(texts, "Act without asking. Edits, ordinary deletes, tests, commits, pushes and pull requests on my own repositories need no permission question.")
		} else {
			texts = append(texts, "Act without asking. Edits, ordinary deletes, tests, commits, pushes and pull requests on my own repositories need no permission question. Ask first only before: "+strings.Join(g, "; ")+".")
		}
	} else {
		texts = append(texts, "Ask before commits, pushes, pull requests and deletions. Routine edits and tests need no question.")
	}
	if a.DocMaxLines > 0 {
		texts = append(texts, fmt.Sprintf("Anything shown for review or approval (README, plan, report): at most about %d lines, answer first, details linked rather than inlined.", a.DocMaxLines))
	}
	for _, c := range a.Custom {
		texts = append(texts, strings.TrimSpace(c))
	}
	out := make([]Rule, len(texts))
	for i, t := range texts {
		out[i] = Rule{ID: fmt.Sprintf("RULE-%03d", i+1), Title: title(t), Text: t}
	}
	return out
}

func orderedGates(g []string) []string {
	have := map[string]bool{}
	for _, x := range g {
		have[x] = true
	}
	var out []string
	for _, x := range allGates {
		if have[x] {
			out = append(out, gateText[x])
		}
	}
	return out
}

func title(t string) string {
	if i := strings.IndexAny(t, ".:"); i > 0 && i < 60 {
		t = t[:i]
	}
	if len(t) > 60 {
		t = t[:60]
	}
	return strings.TrimSpace(t)
}

// Block renders the always-on instruction text (without markers).
func Block(a Answers) string {
	var b strings.Builder
	b.WriteString("## Working rules (set by `beme setup`; a project's own AGENTS.md/CLAUDE.md outrank this list)\n\n")
	for i, r := range Rules(a) {
		fmt.Fprintf(&b, "%d. %s\n", i+1, r.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Entry renders one Be Me source entry (Markdown with frontmatter). The type
// maps to a guidance record; entries are advisory, the block is the
// always-on path.
func Entry(r Rule) string {
	return fmt.Sprintf("---\nid: %s\ntitle: %q\ntype: preference\nstatus: active\n---\n\n%s\n", r.ID, r.Title, r.Text)
}

// Marshal renders the answers file deterministically.
func Marshal(a Answers) []byte {
	a.Gates = append([]string(nil), a.Gates...)
	sort.Strings(a.Gates)
	if a.Gates == nil {
		a.Gates = []string{}
	}
	if a.Custom == nil {
		a.Custom = []string{}
	}
	data, _ := json.MarshalIndent(a, "", "  ")
	return append(data, '\n')
}

// Question describes one interview step so any agent can ask it.
type Question struct {
	Key     string   `json:"key"`
	Prompt  string   `json:"prompt"`
	Kind    string   `json:"kind"` // bool | choice | multi | int | text-list
	Choices []string `json:"choices,omitempty"`
	Default any      `json:"default"`
}

// Questions returns the interview, in order, with recommended defaults.
func Questions() []Question {
	d := Defaults()
	return []Question{
		{"evidence_before_done", "Must agents show command output before saying a task is done?", "bool", nil, d.Evidence},
		{"one_recommendation", "For decisions you delegate, should agents give one recommendation instead of a menu?", "bool", nil, d.OneRecommendation},
		{"style", "Answer style: short and direct, or balanced?", "choice", []string{"short", "balanced"}, d.Style},
		{"report_failures", "Must agents report failed or skipped steps plainly?", "bool", nil, d.FailureReporting},
		{"autonomy", "Should agents act without asking (act) or ask before commits and deletions (ask)?", "choice", []string{"act", "ask"}, d.Autonomy},
		{"ask_before", "Which actions must agents always confirm first? (autonomy=act)", "multi", append([]string(nil), allGates...), d.Gates},
		{"review_doc_max_lines", "Maximum length of documents shown for review (0 = no limit)", "int", nil, d.DocMaxLines},
		{"custom_rules", fmt.Sprintf("Up to %d extra rules of your own, one line each (optional)", MaxCustom), "text-list", nil, []string{}},
	}
}
