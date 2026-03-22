package launcher

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
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
	Models    []string       `json:"models"` // available Ollama models
}

// CheckSQLite verifies the DB file is accessible.
// If it doesn't exist yet, that's OK — init will create it on first start.
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
	return HealthResult{
		Status:  StatusOK,
		Message: fmt.Sprintf("%s (%d KB)", dbPath, sizeKB),
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
func RunAll(dbPath, ollamaURL, anthropicKey string) HealthReport {
	ollamaResult, models := CheckOllama(ollamaURL)
	return HealthReport{
		SQLite:    CheckSQLite(dbPath),
		Ollama:    ollamaResult,
		Anthropic: CheckAnthropic(anthropicKey),
		Models:    models,
	}
}
