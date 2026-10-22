// Package brain is the V2 second-brain layer (ADR-033): a plain-markdown
// work-memory vault that carries decisions, open work, work summaries, and
// learnings across agent sessions.
//
// Invariants encoded here:
//   - The vault stores distilled durable outcomes only — never code, secrets,
//     transcripts, or mid-task chatter (the gate rejects them; Doctor lints
//     them). Code belongs in project repositories.
//   - Agents write PROPOSALS to inbox/; only Doctor promotes them, after
//     deterministic gates (selective persistence, no memory poisoning).
//   - Notes are informational data. They carry no authority and can never
//     override user instructions, project policy, or canonical knowledge.
//   - Lifecycle is physical: live notes under projects/, retired notes under
//     archive/. Nothing is destroyed except expired rejected proposals.
//   - No os/exec, no network (FR-021, FR-060): git integration lives in the
//     CLI layer.
package brain

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Type is the closed set of note kinds. Order is the display order.
type Type string

const (
	TypeDecision Type = "decision" // architectural decision + why
	TypeOpen     Type = "open"     // unfinished work / next step
	TypeSummary  Type = "summary"  // work summary: latest state of a project
	TypeLearning Type = "learning" // reusable experience / lesson
)

// idRe keeps note IDs filename- and markdown-safe (they are echoed into
// briefings and used as file names).
var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,90}$`)

var allTypes = []Type{TypeDecision, TypeOpen, TypeSummary, TypeLearning}

// Valid reports whether t is a known note type.
func (t Type) Valid() bool {
	for _, k := range allTypes {
		if t == k {
			return true
		}
	}
	return false
}

func (t Type) dir() string {
	switch t {
	case TypeDecision:
		return "decisions"
	case TypeOpen:
		return "open"
	case TypeSummary:
		return "summaries"
	default:
		return "learnings"
	}
}

func (t Type) prefix() string {
	switch t {
	case TypeDecision:
		return "dec"
	case TypeOpen:
		return "opn"
	case TypeSummary:
		return "sum"
	default:
		return "lrn"
	}
}

// liveStatus is the status of a promoted note of this type.
func (t Type) liveStatus() string {
	if t == TypeOpen {
		return StatusOpen
	}
	return StatusActive
}

// Lifecycle statuses.
const (
	StatusProposed   = "proposed" // inbox only
	StatusActive     = "active"
	StatusOpen       = "open"
	StatusDone       = "done"       // archived open item
	StatusSuperseded = "superseded" // archived: replaced by another note
	StatusMerged     = "merged"     // archived: duplicate folded into another note
	StatusArchived   = "archived"   // archived by operator/agent request
	StatusRejected   = "rejected"   // inbox/rejected only: refused by the gate
)

// Confidence reuses the V1 vocabulary. Agent captures are observed; only a
// human edit (or future confirm path) makes a note validated.
const (
	ConfObserved  = "observed"
	ConfValidated = "validated"
)

// GlobalProject holds cross-project learnings and decisions. It always exists
// and needs no registered root.
const GlobalProject = "global"

func unsupportedScope(n *Note) bool {
	return n.Project == GlobalProject && (n.Type == TypeOpen || n.Type == TypeSummary)
}

// Note is one vault document. The yaml-tagged fields are the frontmatter;
// the text fields are the markdown body sections.
type Note struct {
	ID           string   `yaml:"id"`
	Type         Type     `yaml:"type"`
	Project      string   `yaml:"project"`
	Title        string   `yaml:"title"`
	Status       string   `yaml:"status"`
	Confidence   string   `yaml:"confidence,omitempty"`
	Topic        string   `yaml:"topic,omitempty"`
	Source       string   `yaml:"source,omitempty"`
	Created      string   `yaml:"created"`
	Updated      string   `yaml:"updated,omitempty"`
	LastCaptured string   `yaml:"last_captured,omitempty"` // semantic evidence, not maintenance time
	Confirmed    int      `yaml:"confirmed,omitempty"`     // recapture count, not independent evidence
	RecapturedBy []string `yaml:"recaptured_by,omitempty"` // only unresolved retry IDs; completed receipts live in archive
	MergeApplied bool     `yaml:"merge_applied,omitempty"` // archive receipt: target update committed
	Supersedes   []string `yaml:"supersedes,omitempty"`
	SupersededBy string   `yaml:"superseded_by,omitempty"`
	MergedInto   string   `yaml:"merged_into,omitempty"`
	Reason       string   `yaml:"reason,omitempty"` // why a note was rejected or archived

	Statement string `yaml:"-"` // first paragraph: the decision / open item / summary / lesson
	Why       string `yaml:"-"` // rationale (required for decisions)
	Next      string `yaml:"-"` // concrete next step (open items)
	Extra     string `yaml:"-"` // any other human-authored sections, preserved verbatim

	Path              string `yaml:"-"` // absolute path when loaded from disk
	AccountingWarning string `yaml:"-"` // acceptance succeeded; bookkeeping may be unknown
}

// Marshal renders the note as frontmatter + markdown.
func (n *Note) Marshal() ([]byte, error) {
	fm, err := yaml.Marshal(n)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(fm)
	b.WriteString("---\n\n")
	b.WriteString(n.body())
	return b.Bytes(), nil
}

func (n *Note) body() string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(n.Statement))
	b.WriteString("\n")
	if s := strings.TrimSpace(n.Why); s != "" {
		b.WriteString("\n## Why\n\n" + s + "\n")
	}
	if s := strings.TrimSpace(n.Next); s != "" {
		b.WriteString("\n## Next\n\n" + s + "\n")
	}
	if s := strings.TrimSpace(n.Extra); s != "" {
		b.WriteString("\n" + s + "\n")
	}
	return b.String()
}

// ParseNote reads a vault document. Frontmatter is data only (ADR-006).
func ParseNote(data []byte) (*Note, error) {
	txt := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(txt, "---\n") {
		return nil, fmt.Errorf("missing frontmatter")
	}
	rest := txt[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, fmt.Errorf("unterminated frontmatter")
	}
	var n Note
	if err := yaml.Unmarshal([]byte(rest[:end]), &n); err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}
	if !idRe.MatchString(n.ID) {
		return nil, fmt.Errorf("frontmatter: id must match %s", idRe)
	}
	if !n.Type.Valid() {
		return nil, fmt.Errorf("frontmatter: unknown type %q", n.Type)
	}
	if n.Project == "" {
		return nil, fmt.Errorf("frontmatter: project required")
	}
	body := rest[end+4:]
	body = strings.TrimPrefix(body, "\n")
	n.parseBody(body)
	return &n, nil
}

// parseBody splits the body into statement / Why / Next / Extra. Unknown
// "## " sections are kept verbatim in Extra so hand edits survive rewrites.
func (n *Note) parseBody(body string) {
	section := "statement"
	var stmt, why, next, extra []string
	for _, line := range strings.Split(body, "\n") {
		switch strings.TrimSpace(line) {
		case "## Why":
			section = "why"
			continue
		case "## Next":
			section = "next"
			continue
		}
		if strings.HasPrefix(line, "## ") {
			section = "extra"
		}
		switch section {
		case "statement":
			stmt = append(stmt, line)
		case "why":
			why = append(why, line)
		case "next":
			next = append(next, line)
		default:
			extra = append(extra, line)
		}
	}
	n.Statement = strings.TrimSpace(strings.Join(stmt, "\n"))
	n.Why = strings.TrimSpace(strings.Join(why, "\n"))
	n.Next = strings.TrimSpace(strings.Join(next, "\n"))
	n.Extra = strings.TrimSpace(strings.Join(extra, "\n"))
}

// fullText is everything the gate and similarity see.
func (n *Note) fullText() string {
	return strings.Join([]string{n.Title, n.Statement, n.Why, n.Next, n.Extra}, "\n")
}

// slugify makes an ASCII, filename-safe slug (Turkish letters folded).
func slugify(s string, max int) string {
	fold := strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u",
		"Ç", "c", "Ğ", "g", "İ", "i", "Ö", "o", "Ş", "s", "Ü", "u")
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(fold.Replace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > max {
		out = strings.Trim(out[:max], "-")
	}
	if out == "" {
		return "note"
	}
	return out
}

var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true, "this": true,
	"are": true, "was": true, "will": true, "from": true, "not": true, "use": true,
	"bir": true, "ile": true, "için": true, "icin": true, "olarak": true, "gibi": true,
}

// tokens is the deterministic lexical fingerprint used for similarity.
func tokens(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if stopwords[f] {
			continue
		}
		out[f] = struct{}{}
	}
	return out
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}

// similarity compares two notes as the mean of title overlap and statement
// overlap: a claim is "the same" only when both its label and its content
// agree. Rationale wording is ignored so a reworded duplicate is still caught.
func similarity(a, b *Note) float64 {
	return 0.5*jaccard(tokens(a.Title), tokens(b.Title)) + 0.5*jaccard(tokens(a.Statement), tokens(b.Statement))
}

// negationRe marks statements that reverse or retire something. Two notes
// that share nearly every word but differ in negation ("use X" / "do not use
// X") are opposites, not duplicates.
var negationRe = regexp.MustCompile(`(?i)\b(not|never|no|without|cannot|isn't|don't|doesn't|won't|shouldn't|stop|avoid|drop|instead|deprecated|removed)\b|değil|asla|olmadan|yerine`)

func negated(n *Note) bool { return negationRe.MatchString(n.Title + " " + n.Statement) }

// contradicts is true for high-overlap notes whose negation differs.
func contradicts(a, b *Note) bool { return negated(a) != negated(b) }

// sameClaim is deliberately conservative: lexical overlap cannot establish
// equivalence. Keep scalars, case-sensitive identifiers, order and rationale.
// Only surrounding whitespace is immaterial; similarity is review-only.
func sameClaim(a, b *Note) bool {
	return a.Type == b.Type && a.Project == b.Project && a.Topic == b.Topic &&
		strings.TrimSpace(a.Title) == strings.TrimSpace(b.Title) &&
		strings.TrimSpace(a.Statement) == strings.TrimSpace(b.Statement) &&
		strings.TrimSpace(a.Why) == strings.TrimSpace(b.Why) &&
		strings.TrimSpace(a.Next) == strings.TrimSpace(b.Next) &&
		strings.TrimSpace(a.Extra) == strings.TrimSpace(b.Extra)
}
