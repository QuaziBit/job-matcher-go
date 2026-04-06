package launcher

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Status levels
const (
	StatusOK      = "ok"
	StatusWarn    = "warn"
	StatusError   = "error"
)

type HealthResult struct {
	Status  string `json:"status"`  // ok | warn | error
	Message string `json:"message"`
}

type HealthReport struct {
	SQLite    HealthResult   `json:"sqlite"`
	Ollama    HealthResult   `json:"ollama"`
	Anthropic HealthResult   `json:"anthropic"`
	OpenAI    HealthResult   `json:"openai"`
	Gemini    HealthResult   `json:"gemini"`
	Models    []string       `json:"models"` // available Ollama models
}

// requiredTables lists the four core tables the app needs.
var requiredTables = []string{"jobs", "resumes", "analyses", "applications"}

// CheckSQLite verifies the DB file is accessible and the 4 core tables exist.
// If the file doesn't exist yet, that's OK — InitDB will create it on first start.
func CheckSQLite(dbPath string) HealthResult {
	info, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		return HealthResult{
			Status:  StatusWarn,
			Message: fmt.Sprintf("Will be created at: %s", dbPath),
		}
	}
	if err != nil {
		return HealthResult{
			Status:  StatusError,
			Message: fmt.Sprintf("Cannot access: %v", err),
		}
	}
	sizeKB := info.Size() / 1024

	// Verify the 4 required tables exist.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return HealthResult{
			Status:  StatusWarn,
			Message: fmt.Sprintf("%s (%d KB) — could not open to verify tables: %v", dbPath, sizeKB, err),
		}
	}
	defer db.Close()

	var missing []string
	for _, tbl := range requiredTables {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			missing = append(missing, tbl)
		}
	}
	if len(missing) > 0 {
		return HealthResult{
			Status:  StatusWarn,
			Message: fmt.Sprintf("%s (%d KB) — missing tables: %s", dbPath, sizeKB, strings.Join(missing, ", ")),
		}
	}
	return HealthResult{
		Status:  StatusOK,
		Message: fmt.Sprintf("%s (%d KB, %d tables verified)", dbPath, sizeKB, len(requiredTables)),
	}
}

// CheckOllama pings the Ollama API and returns available models.
func CheckOllama(baseURL string) (HealthResult, []string) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/api/tags")
	if err != nil {
		return HealthResult{
			Status:  StatusWarn,
			Message: "Not reachable — run: ollama serve",
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return HealthResult{
			Status:  StatusWarn,
			Message: "Could not read response",
		}, nil
	}

	var data struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return HealthResult{
			Status:  StatusWarn,
			Message: "Unexpected response format",
		}, nil
	}

	models := make([]string, 0, len(data.Models))
	for _, m := range data.Models {
		models = append(models, m.Name)
	}

	msg := fmt.Sprintf("Running — %d model(s) available", len(models))
	return HealthResult{Status: StatusOK, Message: msg}, models
}

// CheckAnthropic validates the API key format (no actual API call).
func CheckAnthropic(apiKey string) HealthResult {
	if apiKey == "" {
		return HealthResult{
			Status:  StatusWarn,
			Message: "No key set — Anthropic provider unavailable",
		}
	}
	if !strings.HasPrefix(apiKey, "sk-ant-") {
		return HealthResult{
			Status:  StatusError,
			Message: "Key format invalid (must start with sk-ant-)",
		}
	}
	// Mask for display
	masked := apiKey
	if len(apiKey) > 16 {
		masked = apiKey[:12] + "..." + apiKey[len(apiKey)-4:]
	}
	return HealthResult{
		Status:  StatusOK,
		Message: fmt.Sprintf("Key present (%s)", masked),
	}
}

// RunAll runs all health checks and returns the full report.
func RunAll(dbPath, ollamaURL, anthropicKey, openaiKey, geminiKey string) HealthReport {
	ollamaResult, models := CheckOllama(ollamaURL)
	return HealthReport{
		SQLite:    CheckSQLite(dbPath),
		Ollama:    ollamaResult,
		Anthropic: CheckAnthropic(anthropicKey),
		OpenAI:    CheckOpenAI(openaiKey),
		Gemini:    CheckGemini(geminiKey),
		Models:    models,
	}
}

func CheckOpenAI(apiKey string) HealthResult {
	if apiKey == "" {
		return HealthResult{Status: StatusWarn, Message: "API key not set — OpenAI provider disabled"}
	}
	if len(apiKey) < 20 || !strings.HasPrefix(apiKey, "sk-") {
		return HealthResult{Status: StatusError, Message: "API key format invalid — expected sk-..."}
	}
	masked := apiKey[:7] + "..." + apiKey[len(apiKey)-4:]
	return HealthResult{Status: StatusOK, Message: fmt.Sprintf("Key present (%s)", masked)}
}

func CheckGemini(apiKey string) HealthResult {
	if apiKey == "" {
		return HealthResult{Status: StatusWarn, Message: "API key not set — Gemini provider disabled"}
	}
	if len(apiKey) < 10 {
		return HealthResult{Status: StatusError, Message: "API key format invalid"}
	}
	masked := apiKey[:6] + "..." + apiKey[len(apiKey)-4:]
	return HealthResult{Status: StatusOK, Message: fmt.Sprintf("Key present (%s)", masked)}
}
