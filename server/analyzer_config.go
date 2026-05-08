package server

import (
	"log"
	"regexp"
	"strings"

	"github.com/QuaziBit/job-matcher-go/config"
)

var blockerKeywords = []string{
	"clearance", "ts/sci", "top secret", "secret", "polygraph",
	"citizenship", "citizen only", "usc only",
}

var yearPattern = regexp.MustCompile(`(\d+)\+?\s*years?\s*(of\s*)?(\w+\s*)*?(experience|exp)`)

// ModeConfig holds per-mode analysis settings.
type ModeConfig struct {
	SnippetLen  int
	MaxMatched  int
	MaxMissing  int
	Suggestions bool
	MaxTokens   int
}

var modeConfigs = map[string]ModeConfig{
	"fast":     {SnippetLen: 40,  MaxMatched: 5,  MaxMissing: 4,  Suggestions: false, MaxTokens: 800},
	"standard": {SnippetLen: 70,  MaxMatched: 8,  MaxMissing: 6,  Suggestions: false, MaxTokens: 1800},
	"detailed": {SnippetLen: 100, MaxMatched: 15, MaxMissing: 10, Suggestions: true,  MaxTokens: 4096},
}

// modelMaxMode maps known Ollama model names to the highest analysis mode
// they can reliably produce well-formed JSON for.
var modelMaxMode = map[string]string{
	// fast-only (small models, limited instruction following)
	"phi3.5":        "fast",
	"phi3.5:3.8b":   "fast",
	"llama3.2:1b":   "fast",
	"gemma3:1b":     "fast",
	"nemotron-nano": "fast",
	// standard-capable
	"llama3.2:3b":    "standard",
	"gemma3:4b":      "standard",
	"gemma3:12b":     "standard",
	"mistral:7b":     "standard",
	"phi4:14b":       "standard",
	"deepseek-r1:7b": "standard",
	"qwen2.5:7b":     "standard",
	// detailed-capable
	"llama3.1:8b":     "detailed",
	"llama3.3:70b":    "detailed",
	"gemma3:27b":      "detailed",
	"gemma3n:e4b":     "detailed",
	"gemma4:e2b":      "detailed",
	"gemma4:e4b":      "detailed",
	"gemma4:26b":      "detailed",
	"gemma4:31b":      "detailed",
	"mixtral:8x7b":    "detailed",
	"deepseek-r1:14b": "detailed",
	"qwen2.5:14b":     "detailed",
}

// thinkingModels is the set of Ollama models that use a built-in reasoning
// phase before producing output. These require the two-call thinking path
// instead of the standard chunked path.
var thinkingModels = map[string]bool{
	"gemma4:e2b":        true,
	"gemma4:e4b":        true,
	"gemma4:26b":        true,
	"gemma4:31b":        true,
	"gemma4:latest":     true,
	"deepseek-r1:7b":    true,
	"deepseek-r1:14b":   true,
	"deepseek-r1:32b":   true,
	"deepseek-r1:70b":   true,
	"deepseek-r1:671b":  true,
	"qwq:32b":           true,
}

// isThinkingModel returns true if the model uses a built-in thinking/reasoning
// phase. Checks exact match first, then prefix match on base name before ":".
func isThinkingModel(modelName string) bool {
	if modelName == "" {
		return false
	}
	if thinkingModels[modelName] {
		return true
	}
	base := strings.ToLower(strings.SplitN(modelName, ":", 2)[0])
	for k := range thinkingModels {
		if strings.ToLower(strings.SplitN(k, ":", 2)[0]) == base {
			return true
		}
	}
	return false
}

var modeOrder = map[string]int{"fast": 0, "standard": 1, "detailed": 2}

// getModelMaxMode returns the highest reliable analysis mode for a given model.
// Tries exact match first, then longest prefix match. Unknown models default to "detailed".
func getModelMaxMode(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if mode, ok := modelMaxMode[model]; ok {
		return mode
	}
	// Prefix match — longest prefix wins so "llama3.1:8b-instruct" matches "llama3.1:8b"
	best, bestMode := "", ""
	for prefix, mode := range modelMaxMode {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(best) {
			best, bestMode = prefix, mode
		}
	}
	if bestMode != "" {
		return bestMode
	}
	return "detailed" // unknown models assumed capable
}

// capModeForModel caps the requested mode to the model's known capability.
// Logs a warning when a downgrade occurs.
func capModeForModel(requested, model string) string {
	maxMode := getModelMaxMode(model)
	if modeOrder[requested] <= modeOrder[maxMode] {
		return requested
	}
	log.Printf("⚠ %s max mode is '%s' — downgrading from '%s' to '%s'", model, maxMode, requested, maxMode)
	return maxMode
}

// getModeConfigFor returns the ModeConfig for the given mode name.
func getModeConfigFor(mode string) ModeConfig {
	if m, ok := modeConfigs[mode]; ok {
		return m
	}
	return modeConfigs["standard"]
}

func getModeConfig(cfg config.Config) ModeConfig {
	if m, ok := modeConfigs[cfg.AnalysisMode]; ok {
		return m
	}
	return modeConfigs["standard"]
}
