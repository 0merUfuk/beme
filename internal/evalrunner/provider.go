package evalrunner

// Provider is a model backend. A deterministic mock implements the same
// interface as a live or command-backed provider, so the runner is proven
// without paid calls.
type Provider interface {
	// Name identifies the provider.
	Name() string
	// Settings reports the model settings this provider generates with. The
	// runner records them verbatim in every run manifest (§18.7), so a
	// provider must report what it actually uses — never defaults it cannot
	// vouch for.
	Settings() ModelSettings
	// Generate produces the agent's answer for one case under one arm.
	Generate(req GenerationRequest) (GenerationResult, error)
}

// ModelSettings are the provider-reported generation settings.
type ModelSettings struct {
	Provider        string  `json:"model_provider"`
	ModelID         string  `json:"model_id"`
	ModelVersion    string  `json:"model_version"`
	Seed            int     `json:"seed"`
	Temperature     float64 `json:"temperature"`
	TopP            float64 `json:"top_p"`
	MaxOutputTokens int     `json:"max_output_tokens"`
	ReasoningBudget *int    `json:"reasoning_budget"`
}

// GenerationRequest is what a provider sees for one case run.
type GenerationRequest struct {
	RunID  string
	CaseID string
	// Arm is for in-process bookkeeping only. It never appears in Prompt,
	// and the command provider never passes it to the external command.
	Arm    Arm
	Repeat int
	// Task is the case task text.
	Task string
	// Bootstrap is the real managed bootstrap text ("" for B0).
	Bootstrap string
	// Context is this arm's personal context, deep-copied per request.
	Context ArmContext
	// Prompt is the deterministic rendering of Task + Bootstrap + Context
	// (RenderPrompt) — the exact text a model is given.
	Prompt string
}

// GenerationResult is the raw model output for grading.
type GenerationResult struct {
	Text       string            `json:"text"`
	TokensUsed int               `json:"tokens_used"`
	Meta       map[string]string `json:"meta,omitempty"`
}
