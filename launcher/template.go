package launcher

import (
	_ "embed"
	"html"
	"strconv"
	"strings"

	"github.com/QuaziBit/job-matcher-go/config"
)

//go:embed embedded/launcher.html
var launcherHTML string

//go:embed embedded/launcher.css
var launcherCSS string

//go:embed embedded/launcher.js
var launcherJS string

// checkedIf returns "checked" if condition is true, empty string otherwise.
func checkedIf(condition bool) string {
	if condition {
		return "checked"
	}
	return ""
}

// renderLauncherPage builds the launcher HTML by substituting config values
// into the embedded template. Uses strings.Replacer (single-pass, no %% escaping needed).
func renderLauncherPage(cfg config.Config) string {
	r := strings.NewReplacer(
		"{css}",                launcherCSS,
		"{js}",                 launcherJS,
		"{port}",               strconv.Itoa(cfg.Port),
		"{host}",               html.EscapeString(cfg.Host),
		"{db_path}",            html.EscapeString(cfg.DBPath),
		"{anthropic_api_key}",  html.EscapeString(cfg.AnthropicAPIKey),
		"{anthropic_model}",    html.EscapeString(cfg.AnthropicModel),
		"{ollama_base_url}",    html.EscapeString(cfg.OllamaBaseURL),
		"{ollama_model}",       html.EscapeString(cfg.OllamaModel),
		"{ollama_timeout}",     strconv.Itoa(cfg.OllamaTimeoutSeconds),
		"{checked_fast}",       checkedIf(cfg.AnalysisMode == "fast"),
		"{checked_standard}",   checkedIf(cfg.AnalysisMode == "standard" || cfg.AnalysisMode == ""),
		"{checked_detailed}",   checkedIf(cfg.AnalysisMode == "detailed"),
		"{checked_show_more_logs}", checkedIf(cfg.ShowMoreLogs),
	)
	return r.Replace(launcherHTML)
}
