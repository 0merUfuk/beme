package setup

import (
	"strings"
	"testing"
)

func TestDefaultsRenderTheSixReferenceRules(t *testing.T) {
	a := Defaults()
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	rs := Rules(a)
	if len(rs) != 6 {
		t.Fatalf("want 6 rules, got %d", len(rs))
	}
	blk := Block(a)
	for _, want := range []string{"command output as proof", "one recommendation", "short and direct", "skipped steps plainly",
		"a purchase; a post to a public platform; a dangerous deletion", "at most about 60 lines"} {
		if !strings.Contains(blk, want) {
			t.Errorf("block missing %q:\n%s", want, blk)
		}
	}
	if strings.Contains(blk, BlockBegin) {
		t.Fatal("block text must not contain its own markers")
	}
}

func TestParseRejectsUnknownKeysAndTrailingData(t *testing.T) {
	good := `{"schema_version":1,"style":"short","autonomy":"act","ask_before":["purchase"],"review_doc_max_lines":0}`
	if _, err := Parse([]byte(good)); err != nil {
		t.Fatal(err)
	}
	for name, doc := range map[string]string{
		"unknown key":   `{"schema_version":1,"style":"short","autonomy":"act","evidnce_before_done":true}`,
		"trailing data": good + `{"x":1}`,
		"bad version":   `{"schema_version":2,"style":"short","autonomy":"act"}`,
		"bad style":     `{"schema_version":1,"style":"loud","autonomy":"act"}`,
		"bad autonomy":  `{"schema_version":1,"style":"short","autonomy":"yolo"}`,
		"unknown gate":  `{"schema_version":1,"style":"short","autonomy":"act","ask_before":["everything"]}`,
		"dup gate":      `{"schema_version":1,"style":"short","autonomy":"act","ask_before":["purchase","purchase"]}`,
		"doc too small": `{"schema_version":1,"style":"short","autonomy":"act","review_doc_max_lines":5}`,
		"doc too large": `{"schema_version":1,"style":"short","autonomy":"act","review_doc_max_lines":9000}`,
	} {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestCustomRulesCannotForgeMarkersOrSpanLines(t *testing.T) {
	for name, c := range map[string]string{
		"empty":        "   ",
		"too long":     strings.Repeat("x", MaxCustomLen+1),
		"newline":      "line one\nline two",
		"comment open": "see <!-- hidden",
		"end marker":   "<!-- END beme-rules:managed -->",
		"managed word": "disable beme:managed",
	} {
		a := Defaults()
		a.Custom = []string{c}
		if err := a.Validate(); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
	a := Defaults()
	a.Custom = []string{"a", "b", "c", "d"}
	if err := a.Validate(); err == nil {
		t.Error("more than MaxCustom custom rules must be rejected")
	}
	a = Defaults()
	a.Custom = []string{"Prefer small commits."}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(Block(a), "Prefer small commits.") {
		t.Fatal("valid custom rule must be rendered")
	}
}

func TestVariantsChangeTheRenderedRules(t *testing.T) {
	a := Defaults()
	a.Style, a.Autonomy, a.DocMaxLines, a.Evidence = "balanced", "ask", 0, false
	blk := Block(a)
	if !strings.Contains(blk, "Ask before commits") || strings.Contains(blk, "Act without asking") {
		t.Errorf("ask autonomy not rendered:\n%s", blk)
	}
	if strings.Contains(blk, "lines") || strings.Contains(blk, "command output") {
		t.Errorf("disabled rules must not render:\n%s", blk)
	}
	b := Defaults()
	b.Gates = nil
	if !strings.Contains(Block(b), "Act without asking") || strings.Contains(Block(b), "Ask first only before") {
		t.Errorf("act with no gates:\n%s", Block(b))
	}
}

func TestEntriesAreIngestibleShape(t *testing.T) {
	for _, r := range Rules(Defaults()) {
		e := Entry(r)
		if !strings.HasPrefix(e, "---\nid: RULE-") || !strings.Contains(e, "type: preference") || !strings.Contains(e, "status: active") {
			t.Errorf("entry shape: %q", e)
		}
	}
}

func TestMarshalRoundTripsAndIsDeterministic(t *testing.T) {
	a := Defaults()
	a.Gates = []string{"public-post", "purchase"}
	m1, m2 := Marshal(a), Marshal(a)
	if string(m1) != string(m2) {
		t.Fatal("marshal not deterministic")
	}
	back, err := Parse(m1)
	if err != nil {
		t.Fatal(err)
	}
	if Block(back) != Block(a) {
		t.Fatal("round trip changed the rendered block")
	}
}

func TestQuestionsCoverEveryAnswerKey(t *testing.T) {
	keys := map[string]bool{}
	for _, q := range Questions() {
		keys[q.Key] = true
	}
	for _, k := range []string{"evidence_before_done", "one_recommendation", "style", "report_failures", "autonomy", "ask_before", "review_doc_max_lines", "custom_rules"} {
		if !keys[k] {
			t.Errorf("no question for %q", k)
		}
	}
}

func TestOmittedAskBeforeKeepsRecommendedGates(t *testing.T) {
	a, err := Parse([]byte(`{"schema_version":1,"style":"short","autonomy":"act"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Gates) != 3 || !strings.Contains(Block(a), "Ask first only before") {
		t.Fatalf("omitted ask_before must keep the gates, got %v", a.Gates)
	}
	b, err := Parse([]byte(`{"schema_version":1,"style":"short","autonomy":"act","ask_before":[]}`))
	if err != nil || len(b.Gates) != 0 {
		t.Fatalf("explicit empty list means no gates: %v %v", b.Gates, err)
	}
}

func TestTrailingBraceRejected(t *testing.T) {
	for _, tail := range []string{"}", "]", " {}", "x"} {
		doc := `{"schema_version":1,"style":"short","autonomy":"act"}` + tail
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("trailing %q was accepted", tail)
		}
	}
}
