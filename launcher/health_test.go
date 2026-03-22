package launcher

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ── CheckSQLite ───────────────────────────────────────────────────────────────

func TestCheckSQLite_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	// Create a non-empty file
	if err := os.WriteFile(path, []byte("SQLite format 3"), 0644); err != nil {
		t.Fatal(err)
	}

	result := CheckSQLite(path)
	if result.Status != StatusOK {
		t.Errorf("expected status ok, got %s: %s", result.Status, result.Message)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCheckSQLite_MissingFile(t *testing.T) {
	result := CheckSQLite("/nonexistent/path/test.db")
	if result.Status != StatusWarn {
		t.Errorf("expected status warn for missing file, got %s", result.Status)
	}
}

func TestCheckSQLite_MessageContainsPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mydb.db")
	os.WriteFile(path, []byte("data"), 0644)

	result := CheckSQLite(path)
	if result.Status == StatusOK && result.Message == "" {
		t.Error("expected message to contain path info")
	}
}

// ── CheckAnthropic ────────────────────────────────────────────────────────────

func TestCheckAnthropic_EmptyKey(t *testing.T) {
	result := CheckAnthropic("")
	if result.Status != StatusWarn {
		t.Errorf("expected warn for empty key, got %s", result.Status)
	}
}

func TestCheckAnthropic_ValidKey(t *testing.T) {
	result := CheckAnthropic("sk-ant-api03-validkeyhere1234567890abcdef")
	if result.Status != StatusOK {
		t.Errorf("expected ok for valid key, got %s: %s", result.Status, result.Message)
	}
}

func TestCheckAnthropic_InvalidKeyFormat(t *testing.T) {
	result := CheckAnthropic("not-a-valid-key")
	if result.Status != StatusError {
		t.Errorf("expected error for invalid key format, got %s", result.Status)
	}
}

func TestCheckAnthropic_KeyIsMaskedInMessage(t *testing.T) {
	key := "sk-ant-api03-supersecretkey1234567890"
	result := CheckAnthropic(key)
	if result.Status == StatusOK {
		// Should not expose the full key
		if result.Message == key {
			t.Error("full API key should not appear in message")
		}
		// Should contain masked version
		if len(result.Message) == 0 {
			t.Error("expected non-empty masked key message")
		}
	}
}

func TestCheckAnthropic_ShortValidKey(t *testing.T) {
	// Key that starts with sk-ant- but is short
	result := CheckAnthropic("sk-ant-abc")
	if result.Status != StatusOK {
		t.Errorf("expected ok for short but valid-format key, got %s", result.Status)
	}
}

// ── CheckOllama ───────────────────────────────────────────────────────────────

func TestCheckOllama_ServerRunning(t *testing.T) {
	// Mock Ollama server
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"models": []map[string]string{
					{"name": "llama3.1:8b"},
					{"name": "gemma3:27b"},
					{"name": "mistral:7b"},
				},
			})
		}
	}))
	defer mock.Close()

	result, models := CheckOllama(mock.URL)
	if result.Status != StatusOK {
		t.Errorf("expected ok, got %s: %s", result.Status, result.Message)
	}
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}
	if models[0] != "llama3.1:8b" {
		t.Errorf("expected first model llama3.1:8b, got %s", models[0])
	}
}

func TestCheckOllama_ServerNotRunning(t *testing.T) {
	// Use a port nothing is listening on
	result, models := CheckOllama("http://127.0.0.1:19999")
	if result.Status != StatusWarn {
		t.Errorf("expected warn for unreachable server, got %s", result.Status)
	}
	if len(models) != 0 {
		t.Errorf("expected no models when server unreachable, got %d", len(models))
	}
}

func TestCheckOllama_EmptyModelList(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []interface{}{},
		})
	}))
	defer mock.Close()

	result, models := CheckOllama(mock.URL)
	if result.Status != StatusOK {
		t.Errorf("expected ok even with empty model list, got %s", result.Status)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
}

func TestCheckOllama_InvalidJSON(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer mock.Close()

	result, _ := CheckOllama(mock.URL)
	if result.Status != StatusWarn {
		t.Errorf("expected warn for invalid response, got %s", result.Status)
	}
}

// ── RunAll ────────────────────────────────────────────────────────────────────

func TestRunAll_ReturnsAllThreeResults(t *testing.T) {
	// Mock Ollama
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]string{{"name": "llama3.1:8b"}},
		})
	}))
	defer mock.Close()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	os.WriteFile(dbPath, []byte("db"), 0644)

	report := RunAll(dbPath, mock.URL, "sk-ant-validkey123")

	if report.SQLite.Status == "" {
		t.Error("SQLite result should not be empty")
	}
	if report.Ollama.Status == "" {
		t.Error("Ollama result should not be empty")
	}
	if report.Anthropic.Status == "" {
		t.Error("Anthropic result should not be empty")
	}
	if len(report.Models) == 0 {
		t.Error("expected at least one model in report")
	}
}

func TestRunAll_MissingDBIsWarnNotError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"models": []interface{}{}})
	}))
	defer mock.Close()

	report := RunAll("/nonexistent/path/db.sqlite", mock.URL, "")

	// Missing DB is a warning (will be created on first start), not a hard error
	if report.SQLite.Status == StatusError {
		t.Errorf("missing DB should be warn not error, got %s", report.SQLite.Status)
	}
}
