package evalrunner

import (
	"strings"
)

// MockProvider is a fully deterministic model stand-in. It "answers" by
// composing text from the pack content it receives under each arm, so:
//   - B0 (no pack) can only echo the task → weak, often unacceptable.
//   - B2/B4 (pack present) surface acceptable decisions + mandatory
//     conclusions from pack items → strong answers.
//
// This makes the whole pipeline (resolve → generate → grade → package)
// provable end-to-end with zero paid calls and identical outputs per seed.
type MockProvider struct {
	name string
}

func NewMockProvider(name string) MockProvider {
	if name == "" {
		name = "mock_deterministic_v1"
	}
	return MockProvider{name: name}
}

func (m MockProvider) Name() string { return m.name }

func (m MockProvider) Generate(req GenerationRequest) (GenerationResult, error) {
	var sb strings.Builder
	// the mock "thinks" using pack text; each item contributes its text
	items := 0
	for _, c := range req.Pack.Constraints {
		sb.WriteString(c.Text)
		sb.WriteString(" ")
		items++
	}
	for _, g := range req.Pack.Guidance {
		sb.WriteString(g.Text)
		sb.WriteString(" ")
		items++
	}
	for _, p := range req.Pack.Precedents {
		sb.WriteString(p.Text)
		sb.WriteString(" ")
		items++
	}
	for _, u := range req.Pack.Unknowns {
		sb.WriteString("unknown: ")
		sb.WriteString(u.Question)
		sb.WriteString(". ")
	}
	if items == 0 {
		// no pack content: echo the task (plain-agent behavior)
		sb.WriteString(req.Task)
	}
	return GenerationResult{
		Text:       sb.String(),
		TokensUsed: len(sb.String()) / 4,
		Meta: map[string]string{
			"provider":      m.Name(),
			"arm":           string(req.Arm),
			"repeat":        strings.TrimSpace(intToString(req.Repeat)),
			"deterministic": "true",
		},
	}, nil
}

func intToString(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}
