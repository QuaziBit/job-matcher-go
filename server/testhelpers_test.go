package server

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestDB initialises a fresh in-memory DB and returns a cleanup func.
// Used by database_test.go, handlers_test.go, and any other server tests
// that need a real DB without touching the global state after the test.
func setupTestDB(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	if err := initDB(dbPath); err != nil {
		t.Fatalf("initDB failed: %v", err)
	}
	return func() {
		if db != nil {
			db.Close()
		}
		os.Remove(dbPath)
	}
}
