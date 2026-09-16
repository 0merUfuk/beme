package evalrunner

import (
	"strconv"
	"strings"
)

// MockProvider is a fully deterministic model stand-in. It "answers" by
// composing text from the context items it receives under each arm, so:
//   - B0/B1 (no context) can only echo the task → weak, often unacceptable.
//   - B2/B4 (context present) surface acceptable decisions + mandatory
//     conclusions from context items → strong answers.
//
// This makes the whole pipeline (resolve → generate → grade → package)
// provable end-to-end with zero paid calls and identical outputs per input.
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

// Settings reports the mock's actual behavior: no sampling (temperature 0,
// top_p 1), no seed consumed (0), no output cap (max_output_tokens 0), no
// reasoning budget.
func (m MockProvider) Settings() ModelSettings {
	return ModelSettings{
		Provider:        m.name,
		ModelID:         "context-echo",
		ModelVersion:    "evalrunner-mock-1",
		Seed:            0,
		Temperature:     0,
		TopP:            1,
		MaxOutputTokens: 0,
		ReasoningBudget: nil,
	}
}

func (m MockProvider) Generate(req GenerationRequest) (GenerationResult, error) {
	var sb strings.Builder
	// the mock "thinks" using context text; each item contributes its text
	items := req.Context.Items()
	for _, it := range items {
		sb.WriteString(it.Text)
		sb.WriteString(" ")
	}
	if req.Context.Pack != nil {
		for _, u := range req.Context.Pack.Unknowns {
			sb.WriteString("unknown: ")
			sb.WriteString(u.Question)
			sb.WriteString(". ")
		}
	}
	if len(items) == 0 {
		// no context: echo the task (plain-agent behavior)
		sb.WriteString(req.Task)
	}
	return GenerationResult{
		Text:       sb.String(),
		TokensUsed: len(sb.String()) / 4,
		Meta: map[string]string{
			"provider":      m.Name(),
			"arm":           string(req.Arm),
			"repeat":        strconv.Itoa(req.Repeat),
			"deterministic": "true",
		},
	}, nil
}
