package server

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/QuaziBit/job-matcher-go/config"
)

//go:embed embedded/templates embedded/static
var embeddedFS embed.FS

// Server wraps the main application HTTP server.
type Server struct {
	cfg config.Config
	srv *http.Server
}

// New creates a new Server with the given config.
func New(cfg config.Config) *Server {
	return &Server{cfg: cfg}
}

// parseTemplate parses base.html + the given page template into a fresh set.
// This prevents define block conflicts across pages (e.g. multiple "title" defines).
func parseTemplate(name string) (*template.Template, error) {
	return template.New("").Funcs(templateFuncs()).ParseFS(
		embeddedFS,
		"embedded/templates/base.html",
		"embedded/templates/"+name,
	)
}

// Start initializes DB, parses templates, registers routes, and begins serving.
func (s *Server) Start() error {
	log.Printf("→ Initializing database: %s", s.cfg.DBPath)
	if err := initDB(s.cfg.DBPath); err != nil {
		return fmt.Errorf("database init failed: %w", err)
	}

	mux := http.NewServeMux()
	registerRoutes(mux, s.cfg)

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	s.srv = &http.Server{
		Addr:    addr,
		Handler: &loggedMux{mux: mux},
	}

	go func() {
		log.Printf("→ HTTP server listening on %s", addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("✗ Server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	log.Printf("→ Stopping server...")
	if s.srv != nil {
		if err := s.srv.Shutdown(context.Background()); err != nil {
			log.Printf("✗ Server shutdown error: %v", err)
		}
	}
	if db != nil {
		if err := db.Close(); err != nil {
			log.Printf("✗ DB close error: %v", err)
		}
	}
	log.Printf("✓ Server stopped")
}

// templateFuncs returns custom template functions.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"hasPrefix":          strings.HasPrefix,
		"upper":              strings.ToUpper,
		"groupMatchedSkills": groupMatchedSkills,
		"buildClusterLines":  buildClusterLines,
		"formatDuration": func(s int) string {
			if s == 0 {
				return ""
			}
			m := s / 60
			sec := s % 60
			if m > 0 {
				return fmt.Sprintf("%d:%02d", m, sec)
			}
			return fmt.Sprintf("%ds", sec)
		},
		// "empty" checks if a string value is empty — use instead of "not" for strings
		// since Go's built-in "not" only works on booleans
		"empty": func(v interface{}) bool {
			if v == nil {
				return true
			}
			switch val := v.(type) {
			case string:
				return val == ""
			case *int:
				return val == nil
			case bool:
				return !val
			}
			return false
		},
		"sub": func(a, b int) int { return a - b },
		"gt":  func(a, b int) bool { return a > b },
		"slice": func(s string, i, j int) string {
			if i >= len(s) {
				return ""
			}
			if j > len(s) {
				j = len(s)
			}
			return s[i:j]
		},
		"seq": func(vals ...string) []string { return vals },
	}
}
