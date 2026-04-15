package launcher

import (
	"html"
	"log"
	"strconv"
	"strings"

	"github.com/QuaziBit/job-matcher-go/assets"
	"github.com/QuaziBit/job-matcher-go/config"
)

// checkedIf returns "checked" if condition is true, empty string otherwise.
func checkedIf(condition bool) string {
	if condition {
		return "checked"
	}
	return ""
}

// loadLauncherAssets reads launcher HTML, CSS, and JS from embedded FS.
func loadLauncherAssets() (string, string, string) {
	htmlBytes, err := assets.UI.ReadFile("ui/launcher/launcher.html")
	if err != nil {
		log.Fatalf("failed to load launcher.html: %v", err)
	}

	cssBytes, err := assets.UI.ReadFile("ui/launcher/launcher.css")
	if err != nil {
		log.Fatalf("failed to load launcher.css: %v", err)
	}

	jsBytes, err := assets.UI.ReadFile("ui/launcher/launcher.js")
	if err != nil {
		log.Fatalf("failed to load launcher.js: %v", err)
	}

	return string(htmlBytes), string(cssBytes), string(jsBytes)
}

// renderLauncherPage builds the launcher HTML by substituting config values
// into the embedded template.
func renderLauncherPage(cfg config.Config) string {
	htmlStr, cssStr, jsStr := loadLauncherAssets()

	r := strings.NewReplacer(
		"{css}", cssStr,
		"{js}", jsStr,
		"{port}", strconv.Itoa(cfg.Port),
		"{host}", html.EscapeString(cfg.Host),
		"{db_path}", html.EscapeString(cfg.DBPath),
		"{api_key}", html.EscapeString(cfg.AnthropicAPIKey),
		"{openai_key}", html.EscapeString(cfg.OpenAIAPIKey),
		"{gemini_key}", html.EscapeString(cfg.GeminiAPIKey),
		"{ollama_url}", html.EscapeString(cfg.OllamaBaseURL),
		"{ollama_model}", html.EscapeString(cfg.OllamaModel),
		"{ollama_timeout}", strconv.Itoa(cfg.OllamaTimeoutSeconds),
		"{checked_fast}", checkedIf(cfg.AnalysisMode == "fast"),
		"{checked_standard}", checkedIf(cfg.AnalysisMode == "standard" || cfg.AnalysisMode == ""),
		"{checked_detailed}", checkedIf(cfg.AnalysisMode == "detailed"),
		"{checked_show_more_logs}", checkedIf(cfg.ShowMoreLogs),
	)

	return r.Replace(htmlStr)
}
