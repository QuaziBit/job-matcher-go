package server

import (
	"path/filepath"
	"testing"
)

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
		MatchedSkills: []MatchedSkill{{Skill: "Python", MatchType: "exact"}, {Skill: "Docker", MatchType: "exact"}},
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

	items, _, err := dbGetJobListItems(JobFilters{Page: 1, PerPage: 25})
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

// ── JobList filters ───────────────────────────────────────────────────────────

// helper: insert a job with an analysis in one call
func insertJobWithAnalysis(t *testing.T, url, title, company, provider string, score, adjusted int) (int64, int64) {
	t.Helper()
	rid, _ := dbInsertResume("test-resume", "resume content")
	jid, err := dbInsertJob(url, title, company, "DC", "job description")
	if err != nil {
		t.Fatalf("insertJobWithAnalysis: job insert failed: %v", err)
	}
	_, err = dbInsertAnalysis(Analysis{
		JobID: jid, ResumeID: rid,
		Score: score, AdjustedScore: adjusted,
		LLMProvider: provider, LLMModel: "test-model",
	})
	if err != nil {
		t.Fatalf("insertJobWithAnalysis: analysis insert failed: %v", err)
	}
	return jid, rid
}

func TestDBJobList_EmptyDBReturnsZero(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	items, total, err := dbGetJobListItems(JobFilters{Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestDBJobList_EmptySearchReturnsAll(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Go Engineer", "Acme", "DC", "desc")
	dbInsertJob("https://example.com/j2", "Python Dev", "Globex", "VA", "desc")
	dbInsertJob("https://example.com/j3", "DevSecOps", "Initech", "MD", "desc")

	items, total, err := dbGetJobListItems(JobFilters{Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestDBJobList_SearchByTitle(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Go Engineer", "Acme", "DC", "desc")
	dbInsertJob("https://example.com/j2", "Python Developer", "Globex", "VA", "desc")
	dbInsertJob("https://example.com/j3", "DevSecOps Engineer", "Initech", "MD", "desc")

	items, total, err := dbGetJobListItems(JobFilters{Search: "engineer", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 for 'engineer', got %d", total)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestDBJobList_SearchByTitleCaseInsensitive(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Go Engineer", "Acme", "DC", "desc")
	dbInsertJob("https://example.com/j2", "Python Developer", "Globex", "VA", "desc")

	items, total, err := dbGetJobListItems(JobFilters{Search: "ENGINEER", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1 for 'ENGINEER', got %d", total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestDBJobList_SearchByCompany(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Go Engineer", "Acme Corp", "DC", "desc")
	dbInsertJob("https://example.com/j2", "Python Dev", "Globex", "VA", "desc")
	dbInsertJob("https://example.com/j3", "DevSecOps", "Acme Federal", "MD", "desc")

	items, total, err := dbGetJobListItems(JobFilters{Search: "acme", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 results for company 'acme', got %d", total)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestDBJobList_SearchNoMatch(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Go Engineer", "Acme", "DC", "desc")

	items, total, err := dbGetJobListItems(JobFilters{Search: "zzznomatch", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 results, got %d", total)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestDBJobList_FilterByStatus_Applied(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	jid1, _ := dbInsertJob("https://example.com/j1", "Job A", "", "", "desc")
	jid2, _ := dbInsertJob("https://example.com/j2", "Job B", "", "", "desc")
	dbInsertJob("https://example.com/j3", "Job C", "", "", "desc")

	dbUpsertApplication(Application{JobID: jid1, Status: "applied"})
	dbUpsertApplication(Application{JobID: jid2, Status: "interviewing"})
	// j3 has no application — defaults to not_applied

	items, total, err := dbGetJobListItems(JobFilters{Status: "applied", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 applied job, got %d", total)
	}
	if len(items) != 1 || items[0].ID != jid1 {
		t.Errorf("expected job %d in results, got %v", jid1, items)
	}
}

func TestDBJobList_FilterByStatus_NotApplied(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	jid1, _ := dbInsertJob("https://example.com/j1", "Job A", "", "", "desc")
	jid2, _ := dbInsertJob("https://example.com/j2", "Job B", "", "", "desc")

	dbUpsertApplication(Application{JobID: jid1, Status: "applied"})
	// jid2 has no application — defaults to not_applied

	items, total, err := dbGetJobListItems(JobFilters{Status: "not_applied", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 not_applied job, got %d", total)
	}
	if len(items) != 1 || items[0].ID != jid2 {
		t.Errorf("expected job %d, got %v", jid2, items)
	}
}

func TestDBJobList_FilterByScore_MinScore(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	insertJobWithAnalysis(t, "https://example.com/j1", "Job A", "", "anthropic", 5, 5)
	insertJobWithAnalysis(t, "https://example.com/j2", "Job B", "", "anthropic", 4, 3)
	insertJobWithAnalysis(t, "https://example.com/j3", "Job C", "", "anthropic", 3, 2)
	insertJobWithAnalysis(t, "https://example.com/j4", "Job D", "", "anthropic", 2, 1)

	items, total, err := dbGetJobListItems(JobFilters{Score: "3", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 jobs with adjusted score >= 3, got %d", total)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestDBJobList_FilterByScore_Exact5(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	insertJobWithAnalysis(t, "https://example.com/j1", "Job A", "", "anthropic", 5, 5)
	insertJobWithAnalysis(t, "https://example.com/j2", "Job B", "", "anthropic", 5, 4)
	insertJobWithAnalysis(t, "https://example.com/j3", "Job C", "", "anthropic", 3, 3)

	items, total, err := dbGetJobListItems(JobFilters{Score: "5", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected exactly 1 job with score=5, got %d", total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestDBJobList_FilterByScore_NotScored(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "No Analysis", "", "", "desc")
	insertJobWithAnalysis(t, "https://example.com/j2", "Has Analysis", "", "anthropic", 4, 3)

	items, total, err := dbGetJobListItems(JobFilters{Score: "0", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 unscored job, got %d", total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestDBJobList_FilterByScore_Score1(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	insertJobWithAnalysis(t, "https://example.com/j1", "Job A", "", "anthropic", 4, 1)
	insertJobWithAnalysis(t, "https://example.com/j2", "Job B", "", "anthropic", 3, 2)
	dbInsertJob("https://example.com/j3", "No Score", "", "", "desc")

	items, total, err := dbGetJobListItems(JobFilters{Score: "1", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both scored jobs qualify (adjusted >= 1)
	if total != 2 {
		t.Errorf("expected 2 jobs with score >= 1, got %d", total)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestDBJobList_FilterByProvider_Anthropic(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	insertJobWithAnalysis(t, "https://example.com/j1", "Job A", "", "anthropic", 4, 3)
	insertJobWithAnalysis(t, "https://example.com/j2", "Job B", "", "ollama", 3, 2)

	items, total, err := dbGetJobListItems(JobFilters{Provider: "anthropic", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 anthropic job, got %d", total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestDBJobList_FilterByProvider_Ollama(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	insertJobWithAnalysis(t, "https://example.com/j1", "Job A", "", "anthropic", 4, 3)
	insertJobWithAnalysis(t, "https://example.com/j2", "Job B", "", "ollama", 3, 2)
	insertJobWithAnalysis(t, "https://example.com/j3", "Job C", "", "ollama", 5, 5)

	items, total, err := dbGetJobListItems(JobFilters{Provider: "ollama", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 ollama jobs, got %d", total)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestDBJobList_FilterByProvider_Manual(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("manual://abc123", "Pasted Job", "", "", "desc")
	dbInsertJob("https://example.com/j1", "Scraped Job", "", "", "desc")

	items, total, err := dbGetJobListItems(JobFilters{Provider: "manual", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 manual job, got %d", total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestDBJobList_MultipleFiltersAND(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// This one matches all filters: search=engineer, status=applied, score>=3, provider=anthropic
	jid1, _ := insertJobWithAnalysis(t, "https://example.com/j1", "Go Engineer", "Acme", "anthropic", 4, 3)
	dbUpsertApplication(Application{JobID: jid1, Status: "applied"})

	// Wrong status
	jid2, _ := insertJobWithAnalysis(t, "https://example.com/j2", "Python Engineer", "Acme", "anthropic", 4, 3)
	dbUpsertApplication(Application{JobID: jid2, Status: "rejected"})

	// Wrong provider
	insertJobWithAnalysis(t, "https://example.com/j3", "DevSecOps Engineer", "", "ollama", 4, 3)

	// Wrong score
	jid4, _ := insertJobWithAnalysis(t, "https://example.com/j4", "Java Engineer", "", "anthropic", 3, 2)
	dbUpsertApplication(Application{JobID: jid4, Status: "applied"})

	f := JobFilters{
		Search:   "engineer",
		Status:   "applied",
		Score:    "3",
		Provider: "anthropic",
		Page:     1,
		PerPage:  25,
	}
	items, total, err := dbGetJobListItems(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected exactly 1 match for all filters, got %d", total)
	}
	if len(items) != 1 || items[0].ID != jid1 {
		t.Errorf("expected job %d, got %v", jid1, items)
	}
}

func TestDBJobList_Pagination_LimitOffset(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 10; i++ {
		dbInsertJob("https://example.com/j"+string(rune('0'+i)), "Job", "", "", "desc")
	}

	// Page 1, 3 per page
	items, total, err := dbGetJobListItems(JobFilters{Page: 1, PerPage: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 10 {
		t.Errorf("expected total 10, got %d", total)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items on page 1, got %d", len(items))
	}

	// Page 2, 3 per page
	items2, total2, err := dbGetJobListItems(JobFilters{Page: 2, PerPage: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total2 != 10 {
		t.Errorf("expected total 10 on page 2, got %d", total2)
	}
	if len(items2) != 3 {
		t.Errorf("expected 3 items on page 2, got %d", len(items2))
	}

	// Pages must return different jobs
	if items[0].ID == items2[0].ID {
		t.Error("page 1 and page 2 returned the same first item")
	}
}

func TestDBJobList_Pagination_LastPagePartial(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 7; i++ {
		dbInsertJob("https://example.com/j"+string(rune('0'+i)), "Job", "", "", "desc")
	}

	// Page 3 of 3 with per_page=3 should return 1 item
	items, total, err := dbGetJobListItems(JobFilters{Page: 3, PerPage: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 7 {
		t.Errorf("expected total 7, got %d", total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item on last partial page, got %d", len(items))
	}
}

func TestDBJobList_Pagination_AllItems(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		dbInsertJob("https://example.com/j"+string(rune('0'+i)), "Job", "", "", "desc")
	}

	// PerPage=0 means return all
	items, total, err := dbGetJobListItems(JobFilters{Page: 1, PerPage: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(items) != 5 {
		t.Errorf("expected all 5 items, got %d", len(items))
	}
}

func TestDBJobList_Pagination_PageBeyondRange(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Job", "", "", "desc")

	// Page 999 with only 1 job — should return empty list, not error
	items, total, err := dbGetJobListItems(JobFilters{Page: 999, PerPage: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items on out-of-range page, got %d", len(items))
	}
}

// ── Task 1.4 — v2 matched/missing skills DB round-trip ────────────────────────

func TestDBAnalysis_InsertsV2MatchedSkills(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("v2test", "resume content")
	jid, _ := dbInsertJob("https://example.com/v2", "V2 Job", "Co", "DC", "desc")

	a := Analysis{
		JobID:    jid,
		ResumeID: rid,
		Score:    4, AdjustedScore: 4,
		MatchedSkills: []MatchedSkill{
			{Skill: "Go", MatchType: "exact", JDSnippet: "5+ years Go", ResumeSnippet: "Built in Go"},
		},
		MissingSkills: []MissingSkill{},
		Reasoning:     "ok", LLMProvider: "anthropic", LLMModel: "claude-opus-4-5",
	}
	_, err := dbInsertAnalysis(a)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	results, err := dbGetAnalysesByJobID(jid)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one analysis")
	}
	ms := results[0].MatchedSkills
	if len(ms) != 1 {
		t.Fatalf("expected 1 matched skill, got %d", len(ms))
	}
	if ms[0].JDSnippet != "5+ years Go" {
		t.Errorf("expected jd_snippet '5+ years Go', got %q", ms[0].JDSnippet)
	}
	if ms[0].ResumeSnippet != "Built in Go" {
		t.Errorf("expected resume_snippet 'Built in Go', got %q", ms[0].ResumeSnippet)
	}
}

func TestDBAnalysis_InsertsV2MissingSkills(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("v2miss", "resume content")
	jid, _ := dbInsertJob("https://example.com/v2m", "V2 Miss Job", "Co", "DC", "desc")

	a := Analysis{
		JobID:    jid,
		ResumeID: rid,
		Score:    3, AdjustedScore: 2,
		MatchedSkills: []MatchedSkill{},
		MissingSkills: []MissingSkill{
			{Skill: "Kubernetes", Severity: "major", RequirementType: "preferred", JDSnippet: "K8s orchestration preferred"},
		},
		Reasoning: "ok", LLMProvider: "anthropic", LLMModel: "claude-opus-4-5",
	}
	_, err := dbInsertAnalysis(a)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	results, err := dbGetAnalysesByJobID(jid)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	ms := results[0].MissingSkills
	if len(ms) != 1 {
		t.Fatalf("expected 1 missing skill, got %d", len(ms))
	}
	if ms[0].JDSnippet != "K8s orchestration preferred" {
		t.Errorf("expected jd_snippet, got %q", ms[0].JDSnippet)
	}
}

func TestDBAnalysis_FallsBackToV1ForOldRecords(t *testing.T) {
	// Insert directly with only v1 columns populated (simulates old DB record)
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("old", "resume")
	jid, _ := dbInsertJob("https://example.com/old", "Old Job", "Co", "DC", "desc")

	_, err := db.Exec(`
		INSERT INTO analyses (job_id, resume_id, score, adjusted_score, penalty_breakdown,
		matched_skills, missing_skills, reasoning, llm_provider, llm_model)
		VALUES (?, ?, 3, 3, '{}', '["Python","Docker"]', '[]', 'ok', 'anthropic', 'claude-opus-4-5')`,
		jid, rid)
	if err != nil {
		t.Fatalf("raw insert failed: %v", err)
	}

	results, err := dbGetAnalysesByJobID(jid)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(results[0].MatchedSkills) != 2 {
		t.Fatalf("expected 2 matched skills from v1 fallback, got %d", len(results[0].MatchedSkills))
	}
	if results[0].MatchedSkills[0].Skill != "Python" {
		t.Errorf("expected 'Python' from v1 fallback, got %q", results[0].MatchedSkills[0].Skill)
	}
}

// ── Task 3.1 — Suggestions DB round-trip ─────────────────────────────────────

func TestDBAnalysis_InsertAndFetchSuggestions(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("sugg", "resume")
	jid, _ := dbInsertJob("https://example.com/sugg", "Sugg Job", "Co", "DC", "desc")

	a := Analysis{
		JobID: jid, ResumeID: rid,
		Score: 3, AdjustedScore: 3,
		MatchedSkills: []MatchedSkill{},
		MissingSkills: []MissingSkill{},
		Reasoning:     "ok", LLMProvider: "anthropic", LLMModel: "m",
		Suggestions: []ResumeSuggestion{
			{Title: "Clarify AWS", Detail: "Add S3/EC2 details.", JobRequirement: "AWS required"},
		},
	}
	_, err := dbInsertAnalysis(a)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	results, err := dbGetAnalysesByJobID(jid)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(results[0].Suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(results[0].Suggestions))
	}
	if results[0].Suggestions[0].Title != "Clarify AWS" {
		t.Errorf("expected 'Clarify AWS', got %q", results[0].Suggestions[0].Title)
	}
}

func TestDBAnalysis_EmptySuggestionsHandled(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("nosugg", "resume")
	jid, _ := dbInsertJob("https://example.com/nosugg", "No Sugg Job", "Co", "DC", "desc")

	a := Analysis{
		JobID: jid, ResumeID: rid,
		Score: 4, AdjustedScore: 4,
		MatchedSkills: []MatchedSkill{},
		MissingSkills: []MissingSkill{},
		Reasoning:     "ok", LLMProvider: "anthropic", LLMModel: "m",
	}
	_, err := dbInsertAnalysis(a)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	results, _ := dbGetAnalysesByJobID(jid)
	// nil or empty slice are both fine
	if len(results[0].Suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %d", len(results[0].Suggestions))
	}
}
