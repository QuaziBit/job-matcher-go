package server

// KnownModel is a cloud provider model entry shown in the UI dropdown.
type KnownModel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// knownModels lists static model choices per cloud provider.
// Ollama models are fetched dynamically from /api/ollama/models.
// Update model IDs here when providers release new versions.
var knownModels = map[string][]KnownModel{
	"anthropic": {
		{ID: "claude-haiku-4-5-20251001", Label: "Claude Haiku 4.5 · fastest · cheapest"},
		{ID: "claude-sonnet-4-6",         Label: "Claude Sonnet 4.6 · balanced"},
		{ID: "claude-opus-4-5",           Label: "Claude Opus 4.5 · capable"},
		{ID: "claude-opus-4-6",           Label: "Claude Opus 4.6 · most capable"},
	},
	"openai": {},
	"gemini": {},
}
