package launcher

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuaziBit/job-matcher-go/config"
)

func newTestLauncher(t *testing.T) (*Launcher, chan config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := config.Defaults()
	startCh := make(chan config.Config, 1)
	l := New(cfg, cfgPath, startCh)
	return l, startCh, cfgPath
}

func TestLauncherStartsAndServes(t *testing.T) {
	l, _, _ := newTestLauncher(t)
	launcherURL, err := l.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer l.Stop()

	resp, err := http.Get(launcherURL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestLauncherIndexContainsExpectedElements(t *testing.T) {
	l, _, _ := newTestLauncher(t)
	launcherURL, _ := l.Start()
	defer l.Stop()

	resp, err := http.Get(launcherURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)

	checks := []string{
		"Job Matcher",
		"Launcher",
		"start-btn",
		"ollama_model",
		"anthropic_api_key",
		"db_path",
		"port",
	}
	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("expected launcher page to contain %q", check)
		}
	}
}

func TestLauncherHealthEndpointReturnsJSON(t *testing.T) {
	l, _, _ := newTestLauncher(t)
	launcherURL, _ := l.Start()
	defer l.Stop()

	resp, err := http.Get(launcherURL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var report HealthReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("expected valid JSON health report: %v", err)
	}

	if report.SQLite.Status == "" {
		t.Error("SQLite status should not be empty")
	}
	if report.Ollama.Status == "" {
		t.Error("Ollama status should not be empty")
	}
	if report.Anthropic.Status == "" {
		t.Error("Anthropic status should not be empty")
	}
}

func TestLauncherHealthWithQueryParams(t *testing.T) {
	l, _, _ := newTestLauncher(t)
	launcherURL, _ := l.Start()
	defer l.Stop()

	params := url.Values{}
	params.Set("db_path", "/nonexistent/test.db")
	params.Set("ollama_url", "http://127.0.0.1:19998") // nothing listening
	params.Set("api_key", "")

	resp, err := http.Get(launcherURL + "/health?" + params.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var report HealthReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}

	if report.SQLite.Status == StatusOK {
		t.Error("expected warn/error for nonexistent DB path")
	}
	if report.Ollama.Status == StatusOK {
		t.Error("expected warn for unreachable Ollama")
	}
}

func TestLauncherStartRejectsGET(t *testing.T) {
	l, _, _ := newTestLauncher(t)
	launcherURL, _ := l.Start()
	defer l.Stop()

	resp, err := http.Get(launcherURL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /start, got %d", resp.StatusCode)
	}
}

func TestLauncherStartPostSendsConfig(t *testing.T) {
	l, startCh, cfgPath := newTestLauncher(t)
	launcherURL, _ := l.Start()
	defer l.Stop()

	formData := url.Values{}
	formData.Set("port", "9090")
	formData.Set("host", "127.0.0.1")
	formData.Set("db_path", "./test.db")
	formData.Set("anthropic_api_key", "sk-ant-testkey123")
	formData.Set("ollama_base_url", "http://localhost:11434")
	formData.Set("ollama_model", "llama3.1:8b")
	formData.Set("ollama_timeout", "300")

	resp, err := http.PostForm(launcherURL+"/start", formData)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("expected JSON response: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("expected ok:true, got %v", result["ok"])
	}
	if _, ok := result["url"].(string); !ok {
		t.Error("expected url field in response")
	}

	// Config should be sent on startCh
	select {
	case cfg := <-startCh:
		if cfg.Port != 9090 {
			t.Errorf("expected port 9090 in config, got %d", cfg.Port)
		}
		if cfg.AnthropicAPIKey != "sk-ant-testkey123" {
			t.Errorf("expected api key in config, got %s", cfg.AnthropicAPIKey)
		}
		if cfg.OllamaTimeoutSeconds != 300 {
			t.Errorf("expected timeout 300, got %d", cfg.OllamaTimeoutSeconds)
		}
	default:
		t.Error("expected config to be sent on startCh")
	}

	// Config should be saved to disk
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("expected config file to be saved")
	}
}

func TestLauncherStartInvalidPortIgnored(t *testing.T) {
	l, startCh, _ := newTestLauncher(t)
	launcherURL, _ := l.Start()
	defer l.Stop()

	formData := url.Values{}
	formData.Set("port", "99") // below 1024 — invalid, should use default
	formData.Set("host", "127.0.0.1")

	http.PostForm(launcherURL+"/start", formData)

	select {
	case cfg := <-startCh:
		if cfg.Port == 99 {
			t.Error("invalid port 99 should have been ignored")
		}
	default:
		// Config not sent is also acceptable for invalid input
	}
}

func TestLauncherUsesRandomPort(t *testing.T) {
	l1, _, _ := newTestLauncher(t)
	l2, _, _ := newTestLauncher(t)

	url1, err1 := l1.Start()
	url2, err2 := l2.Start()
	defer l1.Stop()
	defer l2.Stop()

	if err1 != nil || err2 != nil {
		t.Fatalf("Start failed: %v %v", err1, err2)
	}

	// Two launchers should get different ports
	if url1 == url2 {
		t.Error("expected two launchers to use different ports")
	}
}

func TestLauncherClearsAnthropicKeyWhenEmpty(t *testing.T) {
	l, startCh, _ := newTestLauncher(t)
	launcherURL, _ := l.Start()
	defer l.Stop()

	// First start with a key set
	formData := url.Values{}
	formData.Set("port", "9091")
	formData.Set("host", "127.0.0.1")
	formData.Set("db_path", "./test.db")
	formData.Set("anthropic_api_key", "sk-ant-original")
	formData.Set("ollama_base_url", "http://localhost:11434")
	formData.Set("ollama_model", "llama3.1:8b")
	formData.Set("ollama_timeout", "300")

	resp, err := http.PostForm(launcherURL+"/start", formData)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	<-startCh // drain startCh

	// Restart with empty key — should clear it
	formData.Set("anthropic_api_key", "")
	resp2, err := http.PostForm(launcherURL+"/restart", formData)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	// Restart signals on restartCh, not startCh
	select {
	case cfg := <-l.RestartCh():
		if cfg.AnthropicAPIKey != "" {
			t.Errorf("expected empty AnthropicAPIKey after clearing, got %q", cfg.AnthropicAPIKey)
		}
	default:
		t.Error("expected config on restartCh after restart")
	}
}

func TestLauncherOpenAIKeyCanBeCleared(t *testing.T) {
	l, startCh, _ := newTestLauncher(t)
	launcherURL, _ := l.Start()
	defer l.Stop()

	formData := url.Values{}
	formData.Set("port", "9092")
	formData.Set("host", "127.0.0.1")
	formData.Set("db_path", "./test.db")
	formData.Set("openai_api_key", "")
	formData.Set("ollama_base_url", "http://localhost:11434")
	formData.Set("ollama_model", "llama3.1:8b")
	formData.Set("ollama_timeout", "300")

	resp, err := http.PostForm(launcherURL+"/start", formData)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	select {
	case cfg := <-startCh:
		if cfg.OpenAIAPIKey != "" {
			t.Errorf("expected empty OpenAIAPIKey, got %q", cfg.OpenAIAPIKey)
		}
	default:
		t.Error("expected config on startCh")
	}
}
