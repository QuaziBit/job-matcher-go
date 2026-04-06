package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Port != 8000 {
		t.Errorf("expected port 8000, got %d", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.DBPath != "job_matcher.db" {
		t.Errorf("expected db_path job_matcher.db, got %s", cfg.DBPath)
	}
	if cfg.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("expected ollama URL http://localhost:11434, got %s", cfg.OllamaBaseURL)
	}
	if cfg.OllamaModel != "llama3.1:8b" {
		t.Errorf("expected ollama model llama3.1:8b, got %s", cfg.OllamaModel)
	}
	if cfg.OllamaTimeoutSeconds != 600 {
		t.Errorf("expected timeout 600, got %d", cfg.OllamaTimeoutSeconds)
	}
	if cfg.OpenAIModel != "gpt-4o-mini" {
		t.Errorf("expected openai model gpt-4o-mini, got %s", cfg.OpenAIModel)
	}
	if cfg.GeminiModel != "gemini-2.5-flash" {
		t.Errorf("expected gemini model gemini-2.5-flash, got %s", cfg.GeminiModel)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := Defaults()
	cfg.Port = 9090
	cfg.AnthropicAPIKey = "sk-ant-test123"
	cfg.OllamaModel = "gemma3:27b"

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Port != 9090 {
		t.Errorf("expected port 9090, got %d", loaded.Port)
	}
	if loaded.AnthropicAPIKey != "sk-ant-test123" {
		t.Errorf("expected api key sk-ant-test123, got %s", loaded.AnthropicAPIKey)
	}
	if loaded.OllamaModel != "gemma3:27b" {
		t.Errorf("expected model gemma3:27b, got %s", loaded.OllamaModel)
	}
}

func TestLoadCreatesDefaultsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should return defaults
	if cfg.Port != 8000 {
		t.Errorf("expected default port 8000, got %d", cfg.Port)
	}

	// Should have created the file
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected config file to be created, but it does not exist")
	}
}

func TestLoadCreatedFileIsValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	_, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read created config: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Errorf("created config is not valid JSON: %v", err)
	}
}

func TestLoadFillsMissingOpenAIGeminiDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write config without openai/gemini model fields
	minimal := `{"port": 8000, "host": "127.0.0.1", "db_path": "job_matcher.db"}`
	if err := os.WriteFile(path, []byte(minimal), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.OpenAIModel != "gpt-4o-mini" {
		t.Errorf("expected openai default gpt-4o-mini, got %s", cfg.OpenAIModel)
	}
	if cfg.GeminiModel != "gemini-2.5-flash" {
		t.Errorf("expected gemini default gemini-2.5-flash, got %s", cfg.GeminiModel)
	}
}

func TestLoadFillsMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write a partial config missing several fields
	partial := `{"port": 8080, "anthropic_api_key": "sk-ant-abc"}`
	if err := os.WriteFile(path, []byte(partial), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected default host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("expected default ollama URL, got %s", cfg.OllamaBaseURL)
	}
	if cfg.OllamaTimeoutSeconds != 600 {
		t.Errorf("expected default timeout 600, got %d", cfg.OllamaTimeoutSeconds)
	}
}

func TestLoadInvalidJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	if err := os.WriteFile(path, []byte("not json {{{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestSaveProducesPrettyJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := Defaults()
	if err := Save(cfg, path); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	// Pretty JSON should contain newlines and indentation
	content := string(data)
	if len(content) == 0 {
		t.Error("saved config is empty")
	}
	// Should have newline (pretty-printed)
	found := false
	for _, c := range content {
		if c == '\n' {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pretty-printed JSON with newlines")
	}
}

func TestPortValidRange(t *testing.T) {
	cfg := Defaults()
	if cfg.Port < 1024 || cfg.Port > 65535 {
		t.Errorf("default port %d is outside valid range 1024-65535", cfg.Port)
	}
}
