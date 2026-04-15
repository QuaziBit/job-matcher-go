package server

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/QuaziBit/job-matcher-go/config"
)

var uiFS embed.FS

// Server wraps the main application HTTP server.
type Server struct {
	cfg config.Config
	srv *http.Server
}

// New creates a new Server with the given config.
func New(cfg config.Config, fs embed.FS) *Server {
	uiFS = fs // global injection once
	return &Server{cfg: cfg}
}

// serveUIFile reads a static HTML shell from the embedded ui/ directory
// and writes it as an HTML response.
func serveUIFile(w http.ResponseWriter, name string) {
	data, err := uiFS.ReadFile("ui/" + name)

	// fallback for tests (VERY IMPORTANT)
	if err != nil {
		data, err = os.ReadFile("../assets/ui/" + name)
	}

	if err != nil {
		log.Printf("✗ UI file not found: %s — %v", name, err)
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// Start initializes DB, registers routes, and begins serving.
func (s *Server) Start() error {
	// Propagate ShowMoreLogs to env so showMoreLogs() helper in llm.go can read it.
	if s.cfg.ShowMoreLogs {
		os.Setenv("SHOW_MORE_LOGS", "true")
	} else {
		os.Setenv("SHOW_MORE_LOGS", "false")
	}
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

// formatWithCommas formats an integer with thousands separators: 140000 → "140,000".
func formatWithCommas(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
