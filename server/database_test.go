package server

import (
	"os"
	"path/filepath"
	"testing"
)

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

// ── Schema ────────────────────────────────────────────────────────────────────

func TestInitDB_CreatesAllTables(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rows, err := db.Query(
		"SELECT name FROM sqlite_master WHERE type='table' AND name IN ('jobs','resumes','analyses','applications')",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
	}
	if len(tables) != 4 {
		t.Errorf("expected 4 tables, got %d: %v", len(tables), tables)
	}
}

func TestInitDB_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	if err := initDB(dbPath); err != nil {
		t.Fatalf("first initDB failed: %v", err)
	}
	defer db.Close()
	// Second call should not error
	if err := createSchema(); err != nil {
		t.Fatalf("second createSchema failed: %v", err)
	}
}

// ── Resumes ───────────────────────────────────────────────────────────────────

func TestDBResume_InsertAndFetch(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, err := dbInsertResume("DevSecOps v1", "resume content here")
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero id")
	}

	resume, err := dbGetResumeByID(id)
	if err != nil || resume == nil {
		t.Fatalf("get failed: %v", err)
	}
	if resume.Label != "DevSecOps v1" {
		t.Errorf("expected label 'DevSecOps v1', got %q", resume.Label)
	}
	if resume.Content != "resume content here" {
		t.Errorf("expected content 'resume content here', got %q", resume.Content)
	}
}

func TestDBResume_GetAll(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertResume("Label A", "content a")
	dbInsertResume("Label B", "content b")

	resumes, err := dbGetResumes()
	if err != nil {
		t.Fatal(err)
	}
	if len(resumes) != 2 {
		t.Errorf("expected 2 resumes, got %d", len(resumes))
	}
}

func TestDBResume_Delete(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertResume("ToDelete", "content")
	if err := dbDeleteResume(id); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	resume, _ := dbGetResumeByID(id)
	if resume != nil {
		t.Error("expected resume to be deleted")
	}
}

func TestDBResume_GetNonExistentReturnsNil(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	resume, err := dbGetResumeByID(9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resume != nil {
		t.Error("expected nil for non-existent resume")
	}
}

// ── Jobs ──────────────────────────────────────────────────────────────────────

func TestDBJob_InsertAndFetch(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, err := dbInsertJob("https://example.com/job/1", "DevSecOps Engineer", "Acme", "Arlington, VA", "job description here")
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	job, err := dbGetJobByID(id)
	if err != nil || job == nil {
		t.Fatalf("get failed: %v", err)
	}
	if job.Title != "DevSecOps Engineer" {
		t.Errorf("expected title 'DevSecOps Engineer', got %q", job.Title)
	}
	if job.Company != "Acme" {
		t.Errorf("expected company 'Acme', got %q", job.Company)
	}
}

func TestDBJob_DuplicateURLFails(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/job/1", "Job A", "", "", "desc")
	_, err := dbInsertJob("https://example.com/job/1", "Job B", "", "", "desc")
	if err == nil {
		t.Error("expected UNIQUE constraint error for duplicate URL")
	}
}

func TestDBJob_GetByURL(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/job/1", "My Job", "Co", "DC", "desc")
	job, err := dbGetJobByURL("https://example.com/job/1")
	if err != nil || job == nil {
		t.Fatalf("get by URL failed: %v", err)
	}
	if job.Title != "My Job" {
		t.Errorf("expected 'My Job', got %q", job.Title)
	}
}

func TestDBJob_GetByURLNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	job, err := dbGetJobByURL("https://notfound.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job != nil {
		t.Error("expected nil for non-existent URL")
	}
}

func TestDBJob_Delete(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/del", "Del Job", "", "", "desc")
	if err := dbDeleteJob(id); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	job, _ := dbGetJobByID(id)
	if job != nil {
		t.Error("expected job to be deleted")
	}
}

// ── Analyses ──────────────────────────────────────────────────────────────────

func TestDBAnalysis_InsertAndFetch(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("v1", "resume")
	jid, _ := dbInsertJob("https://example.com/j1", "Job", "Co", "DC", "desc")

	a := Analysis{
		JobID:         jid,
		ResumeID:      rid,
		Score:         4,
		AdjustedScore: 2,
		PenaltyBreakdown: PenaltyBreakdown{
			Blockers: 1, BlockerPenalty: 2, TotalPenalty: 2,
		},
		MatchedSkills: []string{"Python", "Docker"},
		MissingSkills: []MissingSkill{
			{Skill: "TS/SCI Clearance", Severity: "blocker"},
		},
		Reasoning:   "Good match but missing clearance.",
		LLMProvider: "anthropic",
		LLMModel:    "claude-opus-4-5",
	}

	id, err := dbInsertAnalysis(a)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero id")
	}

	analyses, err := dbGetAnalysesByJobID(jid)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if len(analyses) != 1 {
		t.Fatalf("expected 1 analysis, got %d", len(analyses))
	}

	got := analyses[0]
	if got.Score != 4 {
		t.Errorf("expected score 4, got %d", got.Score)
	}
	if got.AdjustedScore != 2 {
		t.Errorf("expected adjusted 2, got %d", got.AdjustedScore)
	}
	if len(got.MatchedSkills) != 2 {
		t.Errorf("expected 2 matched skills, got %d", len(got.MatchedSkills))
	}
	if len(got.MissingSkills) != 1 {
		t.Errorf("expected 1 missing skill, got %d", len(got.MissingSkills))
	}
	if got.MissingSkills[0].Severity != "blocker" {
		t.Errorf("expected severity blocker, got %s", got.MissingSkills[0].Severity)
	}
	if got.LLMModel != "claude-opus-4-5" {
		t.Errorf("expected model claude-opus-4-5, got %s", got.LLMModel)
	}
}

func TestDBAnalysis_CascadeDeleteWithJob(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("v1", "resume")
	jid, _ := dbInsertJob("https://example.com/cascade", "Job", "", "", "desc")

	dbInsertAnalysis(Analysis{
		JobID: jid, ResumeID: rid, Score: 3, AdjustedScore: 3,
		LLMProvider: "anthropic", LLMModel: "claude-opus-4-5",
	})

	// Delete job — analysis should cascade
	dbDeleteJob(jid)

	analyses, err := dbGetAnalysesByJobID(jid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(analyses) != 0 {
		t.Errorf("expected 0 analyses after cascade delete, got %d", len(analyses))
	}
}

func TestDBAnalysis_Delete(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("v1", "resume")
	jid, _ := dbInsertJob("https://example.com/j2", "Job", "", "", "desc")

	dbInsertAnalysis(Analysis{
		JobID: jid, ResumeID: rid, Score: 4, AdjustedScore: 4,
		LLMProvider: "ollama", LLMModel: "llama3.1:8b",
	})

	analyses, _ := dbGetAnalysesByJobID(jid)
	if len(analyses) == 0 {
		t.Fatal("expected analysis to exist before delete")
	}

	found, err := dbDeleteAnalysis(analyses[0].ID)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !found {
		t.Error("expected found=true for existing analysis")
	}

	analyses2, _ := dbGetAnalysesByJobID(jid)
	if len(analyses2) != 0 {
		t.Error("expected 0 analyses after delete")
	}
}

func TestDBAnalysis_DeleteNonExistent(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	found, err := dbDeleteAnalysis(9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for non-existent analysis")
	}
}

// ── Applications ──────────────────────────────────────────────────────────────

func TestDBApplication_UpsertAndFetch(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	jid, _ := dbInsertJob("https://example.com/app1", "Job", "", "", "desc")

	app := Application{
		JobID:          jid,
		Status:         "applied",
		RecruiterName:  "Jane Smith",
		RecruiterEmail: "jane@company.com",
		RecruiterPhone: "555-1234",
		Notes:          "Great role",
	}
	if err := dbUpsertApplication(app); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	got, err := dbGetApplicationByJobID(jid)
	if err != nil || got == nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Status != "applied" {
		t.Errorf("expected status 'applied', got %q", got.Status)
	}
	if got.RecruiterName != "Jane Smith" {
		t.Errorf("expected recruiter 'Jane Smith', got %q", got.RecruiterName)
	}
}

func TestDBApplication_UpsertUpdatesExisting(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	jid, _ := dbInsertJob("https://example.com/app2", "Job", "", "", "desc")

	dbUpsertApplication(Application{JobID: jid, Status: "applied", RecruiterName: "Jane"})
	dbUpsertApplication(Application{JobID: jid, Status: "interviewing", RecruiterName: "Jane"})

	got, _ := dbGetApplicationByJobID(jid)
	if got.Status != "interviewing" {
		t.Errorf("expected status 'interviewing', got %q", got.Status)
	}
}

func TestDBApplication_GetNonExistentReturnsNil(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	app, err := dbGetApplicationByJobID(9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app != nil {
		t.Error("expected nil for non-existent application")
	}
}

func TestDBJobList_ReturnsAdjustedScore(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("v1", "resume")
	jid, _ := dbInsertJob("https://example.com/list1", "List Job", "Co", "DC", "desc")

	dbInsertAnalysis(Analysis{
		JobID: jid, ResumeID: rid,
		Score: 4, AdjustedScore: 2,
		LLMProvider: "anthropic", LLMModel: "claude-opus-4-5",
	})

	items, err := dbGetJobListItems()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one job")
	}

	var found bool
	for _, item := range items {
		if item.ID == jid {
			found = true
			if item.AdjustedScore == nil || *item.AdjustedScore != 2 {
				t.Errorf("expected adjusted score 2, got %v", item.AdjustedScore)
			}
		}
	}
	if !found {
		t.Error("expected job to appear in list")
	}
}
