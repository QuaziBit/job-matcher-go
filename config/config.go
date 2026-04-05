package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const DefaultConfigFile = "config.json"

// Config holds all runtime settings. Saved as config.json next to the binary.
type Config struct {
	// Server
	Port int    `json:"port"`
	Host string `json:"host"`

	// Database
	DBPath string `json:"db_path"`

	// Anthropic
	AnthropicAPIKey   string `json:"anthropic_api_key"`
	AnthropicModel    string `json:"anthropic_model"` // e.g. claude-opus-4-5, claude-sonnet-4-6

	// Ollama
	OllamaBaseURL        string `json:"ollama_base_url"`
	OllamaModel          string `json:"ollama_model"`
	OllamaTimeoutSeconds int    `json:"ollama_timeout_seconds"`

	// Analysis
	AnalysisMode string `json:"analysis_mode"` // fast | standard | detailed
	ShowMoreLogs bool   `json:"show_more_logs"`
}

// Defaults returns a Config populated with sensible defaults.
func Defaults() Config {
	return Config{
		Port:                 8000,
		Host:                 "127.0.0.1",
		DBPath:               "job_matcher.db",
		AnthropicAPIKey:      "",
		AnthropicModel:       "claude-opus-4-5",
		OllamaBaseURL:        "http://localhost:11434",
		OllamaModel:          "llama3.1:8b",
		OllamaTimeoutSeconds: 600,
		AnalysisMode:         "standard",
	}
}

// Load reads config.json from the given path.
// If the file does not exist, returns Defaults() and saves the file.
func Load(path string) (Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// First run — save defaults so user has a template to edit
		if saveErr := Save(cfg, path); saveErr != nil {
			return cfg, saveErr
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	// Fill in any missing fields added in newer versions
	if cfg.Port == 0 {
		cfg.Port = 8000
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.OllamaBaseURL == "" {
		cfg.OllamaBaseURL = "http://localhost:11434"
	}
	if cfg.OllamaModel == "" {
		cfg.OllamaModel = "llama3.1:8b"
	}
	if cfg.AnthropicModel == "" {
		cfg.AnthropicModel = "claude-opus-4-5"
	}
	if cfg.OllamaTimeoutSeconds == 0 {
		cfg.OllamaTimeoutSeconds = 600
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "job_matcher.db"
	}
	if cfg.AnalysisMode == "" {
		cfg.AnalysisMode = "standard"
	}
	switch cfg.AnalysisMode {
	case "fast", "standard", "detailed":
		// valid
	default:
		cfg.AnalysisMode = "standard"
	}

	return cfg, nil
}

// Save writes the config to path as pretty-printed JSON.
func Save(cfg Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ConfigPath returns the path to config.json inside a cfg/ subfolder
// relative to the current working directory. This works correctly for both
// "go run ." (working dir = project root) and a built binary run from its folder.
// Uses "cfg/" to avoid conflict with the "config/" Go package directory.
func ConfigPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return DefaultConfigFile
	}
	dir := filepath.Join(wd, "cfg")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return filepath.Join(wd, DefaultConfigFile)
	}
	return filepath.Join(dir, DefaultConfigFile)
}
