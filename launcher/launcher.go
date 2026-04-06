package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/QuaziBit/job-matcher-go/config"
)

// Launcher manages the pre-start configuration UI.
type Launcher struct {
	cfg      config.Config
	cfgPath  string
	srv      *http.Server
	port     int
	startCh  chan config.Config // signals main to start the app server
	stopCh   chan struct{}      // signals main to stop the app server
	restartCh chan config.Config // signals main to restart with new config
	mu       sync.Mutex
}

// New creates a Launcher.
func New(cfg config.Config, cfgPath string, startCh chan config.Config) *Launcher {
	return &Launcher{
		cfg:      cfg,
		cfgPath:  cfgPath,
		startCh:  startCh,
		stopCh:   make(chan struct{}, 1),
		restartCh: make(chan config.Config, 1),
	}
}

// StopCh returns the channel that signals app server stop.
func (l *Launcher) StopCh() <-chan struct{} { return l.stopCh }

// RestartCh returns the channel that signals app server restart.
func (l *Launcher) RestartCh() <-chan config.Config { return l.restartCh }

// Start finds a free port, starts the launcher HTTP server, and returns the URL.
func (l *Launcher) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("could not find free port: %w", err)
	}
	l.port = ln.Addr().(*net.TCPAddr).Port
	log.Printf("→ Launcher listening on port %d", l.port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", l.handleIndex)
	mux.HandleFunc("/health", l.handleHealth)
	mux.HandleFunc("/start", l.handleStart)
	mux.HandleFunc("/stop", l.handleStop)
	mux.HandleFunc("/restart", l.handleRestart)

	l.srv = &http.Server{Handler: mux}
	go func() {
		if err := l.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("✗ Launcher server error: %v", err)
		}
	}()
	return fmt.Sprintf("http://127.0.0.1:%d", l.port), nil
}

// Stop shuts down the launcher server.
func (l *Launcher) Stop() {
	log.Printf("→ Stopping launcher")
	if l.srv != nil {
		if err := l.srv.Shutdown(context.Background()); err != nil {
			log.Printf("✗ Launcher shutdown error: %v", err)
		}
	}
	log.Printf("✓ Launcher stopped")
}

// handleIndex serves the launcher HTML page.
func (l *Launcher) handleIndex(w http.ResponseWriter, r *http.Request) {
	log.Printf("→ Launcher GET /")
	l.mu.Lock()
	cfg := l.cfg
	l.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := fmt.Fprint(w, renderLauncherPage(cfg)); err != nil {
		log.Printf("✗ Launcher index write error: %v", err)
	}
}

// handleHealth returns a JSON health report.
func (l *Launcher) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbPath       := r.URL.Query().Get("db_path")
	ollamaURL    := r.URL.Query().Get("ollama_url")
	apiKey       := r.URL.Query().Get("api_key")
	openaiKey    := r.URL.Query().Get("openai_key")
	geminiKey    := r.URL.Query().Get("gemini_key")

	if dbPath == "" {
		l.mu.Lock()
		dbPath    = l.cfg.DBPath
		ollamaURL = l.cfg.OllamaBaseURL
		apiKey    = l.cfg.AnthropicAPIKey
		openaiKey = l.cfg.OpenAIAPIKey
		geminiKey = l.cfg.GeminiAPIKey
		l.mu.Unlock()
	}
	log.Printf("→ Launcher health check: db=%s ollama=%s anthropic_set=%v openai_set=%v gemini_set=%v",
		dbPath, ollamaURL, apiKey != "", openaiKey != "", geminiKey != "")

	report := RunAll(dbPath, ollamaURL, apiKey, openaiKey, geminiKey)
	log.Printf("✓ Health: sqlite=%s ollama=%s anthropic=%s openai=%s gemini=%s models=%d",
		report.SQLite.Status, report.Ollama.Status, report.Anthropic.Status,
		report.OpenAI.Status, report.Gemini.Status, len(report.Models))

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(report); err != nil {
		log.Printf("✗ Launcher health encode error: %v", err)
	}
}

// parseConfig reads and validates form values into a Config.
func (l *Launcher) parseConfig(r *http.Request) config.Config {
	l.mu.Lock()
	cfg := l.cfg
	l.mu.Unlock()

	if v := r.FormValue("port"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 1023 && p < 65536 {
			cfg.Port = p
		} else {
			log.Printf("✗ Invalid port %q: %v", v, err)
		}
	}
	if v := r.FormValue("host"); v != "" {
		cfg.Host = v
	}
	if v := r.FormValue("db_path"); v != "" {
		cfg.DBPath = v
	}
	cfg.AnthropicAPIKey = r.FormValue("anthropic_api_key")
	cfg.OpenAIAPIKey = r.FormValue("openai_api_key")
	cfg.GeminiAPIKey = r.FormValue("gemini_api_key")
	if v := r.FormValue("ollama_base_url"); v != "" {
		cfg.OllamaBaseURL = v
	}
	if v := r.FormValue("ollama_model"); v != "" {
		cfg.OllamaModel = v
	}
	if v := r.FormValue("ollama_timeout"); v != "" {
		if t, err := strconv.Atoi(v); err == nil && t > 0 {
			cfg.OllamaTimeoutSeconds = t
		} else {
			log.Printf("✗ Invalid timeout %q: %v", v, err)
		}
	}
	if v := r.FormValue("analysis_mode"); v != "" {
		switch v {
		case "fast", "standard", "detailed":
			cfg.AnalysisMode = v
		default:
			log.Printf("✗ Invalid analysis_mode %q", v)
		}
	}
	// Checkbox: absent from POST means unchecked (false).
	cfg.ShowMoreLogs = r.FormValue("show_more_logs") != ""
	return cfg
}

// handleStart saves config and signals the main app to start.
func (l *Launcher) handleStart(w http.ResponseWriter, r *http.Request) {
	log.Printf("→ Launcher POST /start")
	if r.Method != http.MethodPost {
		log.Printf("✗ Launcher /start: wrong method %s", r.Method)
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		log.Printf("→ Launcher /start: not multipart, trying urlencoded")
		if err2 := r.ParseForm(); err2 != nil {
			log.Printf("✗ Launcher /start: failed to parse form: %v", err2)
			http.Error(w, "bad form data", http.StatusBadRequest)
			return
		}
	}

	cfg := l.parseConfig(r)

	finalKey := "not set"
	if cfg.AnthropicAPIKey != "" {
		finalKey = cfg.AnthropicAPIKey[:min(12, len(cfg.AnthropicAPIKey))] + "..."
	}
	log.Printf("✓ Launcher /start config: port=%d model=%s key=%s db=%s",
		cfg.Port, cfg.OllamaModel, finalKey, cfg.DBPath)

	l.mu.Lock()
	l.cfg = cfg
	l.mu.Unlock()

	if err := config.Save(cfg, l.cfgPath); err != nil {
		log.Printf("✗ Launcher /start: failed to save config: %v", err)
	} else {
		log.Printf("✓ Config saved to: %s", l.cfgPath)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":  true,
		"url": fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port),
	})

	go func() {
		log.Printf("→ Launcher signaling app start")
		l.startCh <- cfg
	}()
}

// handleStop signals the main app server to stop.
func (l *Launcher) handleStop(w http.ResponseWriter, r *http.Request) {
	log.Printf("→ Launcher POST /stop")
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	go func() {
		log.Printf("→ Launcher signaling app stop")
		l.stopCh <- struct{}{}
	}()
}

// handleRestart stops the current server and starts a new one with updated config.
func (l *Launcher) handleRestart(w http.ResponseWriter, r *http.Request) {
	log.Printf("→ Launcher POST /restart")
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if err2 := r.ParseForm(); err2 != nil {
			log.Printf("✗ Launcher /restart: failed to parse form: %v", err2)
			http.Error(w, "bad form data", http.StatusBadRequest)
			return
		}
	}

	cfg := l.parseConfig(r)

	finalKey := "not set"
	if cfg.AnthropicAPIKey != "" {
		finalKey = cfg.AnthropicAPIKey[:min(12, len(cfg.AnthropicAPIKey))] + "..."
	}
	log.Printf("✓ Launcher /restart config: port=%d model=%s key=%s db=%s",
		cfg.Port, cfg.OllamaModel, finalKey, cfg.DBPath)

	l.mu.Lock()
	l.cfg = cfg
	l.mu.Unlock()

	if err := config.Save(cfg, l.cfgPath); err != nil {
		log.Printf("✗ Launcher /restart: failed to save config: %v", err)
	} else {
		log.Printf("✓ Config saved to: %s", l.cfgPath)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":            true,
		"url":           fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port),
		"analysis_mode": cfg.AnalysisMode,
	})

	go func() {
		log.Printf("→ Launcher signaling app restart")
		l.restartCh <- cfg
	}()
}
