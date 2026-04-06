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
	"openai": {
		{ID: "gpt-4o-mini",  Label: "gpt-4o-mini  · cheapest"},
		{ID: "gpt-4o",       Label: "gpt-4o       · balanced"},
		{ID: "gpt-4-turbo",  Label: "gpt-4-turbo  · powerful"},
		{ID: "o1-mini",      Label: "o1-mini      · reasoning · cheap"},
		{ID: "o1",           Label: "o1           · reasoning · expensive"},
	},
	"gemini": {
		{ID: "gemini-2.5-flash",      Label: "gemini-2.5-flash      · cheapest"},
		{ID: "gemini-2.5-flash-lite", Label: "gemini-2.5-flash-lite · cheapest · fastest"},
		{ID: "gemini-2.5-pro",        Label: "gemini-2.5-pro        · best · expensive"},
		{ID: "gemini-2.0-flash",      Label: "gemini-2.0-flash      · fast · cheap"},
	},
}
