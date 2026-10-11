package brain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/0merUfuk/beme/internal/ingestion"
)

// Violation is one gate finding. Safety violations (secret, code, injection,
// transcript) mean the content must never enter or be injected from the
// vault; shape violations (size, missing fields) are quality warnings for
// live notes but still block proposals.
type Violation struct {
	Code   string
	Detail string
	Safety bool
}

func (v Violation) String() string { return v.Code + ": " + v.Detail }

// GateError is returned when a proposal is refused.
type GateError struct{ Violations []Violation }

func (e *GateError) Error() string {
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, v.String())
	}
	return "rejected by brain gate — " + strings.Join(parts, "; ")
}

const (
	maxTitle     = 120
	minStatement = 20
	maxStatement = 600
	minWhy       = 10
	maxWhy       = 600
	maxNext      = 300
	maxTotal     = 1800
)

var (
	topicRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,60}$`)

	fenceRe   = regexp.MustCompile("(?m)^\\s*(```|~~~)")
	diffRe    = regexp.MustCompile(`(?m)^(diff --git |@@ [-+0-9, ]+@@|\+\+\+ |--- a/|index [0-9a-f]{6,}\.\.)`)
	codeLine  = regexp.MustCompile(`^\s*(func|def|class|import|package|const|let|var|return|public|private|static|#include|SELECT|INSERT|UPDATE|CREATE TABLE)\b|[;{}]\s*$`)
	stackRe   = regexp.MustCompile(`(?m)(^Traceback \(most recent call last\)|^goroutine \d+ \[|^\s+at .+\(.+:\d+(:\d+)?\)$|^panic: )`)
	shellRe   = regexp.MustCompile(`^\s*(\$|>>>) \S`)
	chatRe    = regexp.MustCompile(`(?im)^\s*(user|human|assistant|claude|ai):\s`)
	injectRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(ignore|disregard|forget|override)\s+(all\s+|any\s+|the\s+|your\s+)?(previous|prior|above|earlier|system|these)\s+(instructions?|rules?|prompts?|polic(y|ies)|guidelines?)`),
		regexp.MustCompile(`(?i)\b(you are now|act as if you have no|new instructions?:)`),
		regexp.MustCompile(`(?i)</?\s*(system|assistant|user|brain-memory|system-reminder)\s*>`),
		regexp.MustCompile(`(?i)do not (tell|inform|show) the user`),
	}
)

// Check runs the deterministic gate over one note. It is the single source of
// truth for "what may live in the brain": Capture runs it synchronously,
// Doctor re-runs it on every proposal and lints live notes with it.
func Check(n *Note) []Violation {
	var vs []Violation
	add := func(code, detail string, safety bool) {
		vs = append(vs, Violation{Code: code, Detail: detail, Safety: safety})
	}

	// --- shape
	if !n.Type.Valid() {
		add("type", fmt.Sprintf("unknown type %q (decision|open|summary|learning)", n.Type), false)
	}
	title := strings.TrimSpace(n.Title)
	if len([]rune(title)) < 3 || len([]rune(title)) > maxTitle || strings.ContainsAny(title, "\n\r") {
		add("title", fmt.Sprintf("title must be 3-%d characters on one line", maxTitle), false)
	}
	st := len([]rune(strings.TrimSpace(n.Statement)))
	if st < minStatement {
		add("statement", fmt.Sprintf("body too short (%d < %d chars): state the outcome, not a label", st, minStatement), false)
	}
	if st > maxStatement {
		add("statement", fmt.Sprintf("body too long (%d > %d chars): distill it — the brain keeps outcomes, not narrative", st, maxStatement), false)
	}
	if n.Type == TypeDecision {
		w := len([]rune(strings.TrimSpace(n.Why)))
		if w < minWhy {
			add("why", "a decision requires --why (the rationale is the durable part)", false)
		}
		if w > maxWhy {
			add("why", fmt.Sprintf("why too long (%d > %d chars)", w, maxWhy), false)
		}
	}
	if len([]rune(n.Next)) > maxNext {
		add("next", fmt.Sprintf("next too long (> %d chars)", maxNext), false)
	}
	if len([]rune(n.fullText())) > maxTotal {
		add("size", fmt.Sprintf("note exceeds %d chars in total: distill it", maxTotal), false)
	}
	if n.Topic != "" && !topicRe.MatchString(n.Topic) {
		add("topic", "topic must match [a-z0-9][a-z0-9-]{1,60}", false)
	}

	text := n.fullText()

	// --- safety: secrets (reuse the V1 ingestion scanner: one definition)
	if v := secretViolation(text); v != nil {
		vs = append(vs, *v)
	}

	// --- safety: domain separation — no code in the second brain
	if fenceRe.MatchString(text) {
		add("code", "fenced code blocks are not allowed: code belongs in the project repository (inline `identifiers` are fine)", true)
	} else if diffRe.MatchString(text) {
		add("code", "diff/patch content is not allowed: describe the change, keep the patch in the repository", true)
	} else if stackRe.MatchString(text) {
		add("code", "stack traces are not allowed: record the conclusion, not the trace", true)
	} else {
		codeish, shell := 0, 0
		for _, l := range strings.Split(text, "\n") {
			if codeLine.MatchString(l) {
				codeish++
			}
			if shellRe.MatchString(l) {
				shell++
			}
		}
		if codeish >= 3 || shell >= 2 {
			add("code", "content reads as source code or a shell session: code belongs in the project repository", true)
		}
	}

	// --- safety: transcripts / chat logs are not durable knowledge
	if len(chatRe.FindAllString(text, -1)) >= 2 {
		add("transcript", "conversation transcripts are not allowed: distill the decision instead", true)
	}

	// --- safety: memory is re-injected into every future session, so an
	// instruction-shaped note is a persistence vector (ADR-006)
	if v := injectionViolation(text); v != nil {
		vs = append(vs, *v)
	}
	return vs
}

// bareBearerRe is the word "bearer" NOT followed by a token-shaped string.
var bareBearerRe = regexp.MustCompile(`(?i)\bbearer\b(\s+[A-Za-z0-9._~+/=-]{20,})?`)

// secretViolation reuses the V1 scanner, but the prose word "Bearer" (e.g.
// "services authenticate with Bearer tokens") is not a credential: only a
// token-shaped string after it is, and those remain refused.
func secretViolation(text string) *Violation {
	neutral := bareBearerRe.ReplaceAllStringFunc(text, func(m string) string {
		if strings.TrimSpace(strings.TrimPrefix(strings.ToLower(m), "bearer")) == "" {
			return "b-e-a-r-e-r"
		}
		return m
	})
	if err := ingestion.SecretScan([]byte(neutral)); err != nil {
		return &Violation{Code: "secret", Detail: err.Error(), Safety: true}
	}
	return nil
}

// injectionViolation flags instruction-shaped text: memory is re-injected into
// every future session, so it is a persistence vector (ADR-006).
func injectionViolation(text string) *Violation {
	for _, re := range injectRes {
		if re.MatchString(text) {
			return &Violation{Code: "injection", Detail: "instruction-like content is not allowed in memory (notes are data, never directives)", Safety: true}
		}
	}
	return nil
}

// unsafe reports whether any violation is a safety violation.
func unsafe(vs []Violation) bool {
	for _, v := range vs {
		if v.Safety {
			return true
		}
	}
	return false
}
