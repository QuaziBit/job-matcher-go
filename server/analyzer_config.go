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
	"mixtral:8x7b":    "detailed",
	"deepseek-r1:14b": "detailed",
	"qwen2.5:14b":     "detailed",
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
