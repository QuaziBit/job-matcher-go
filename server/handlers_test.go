package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// getJobsList is a test helper that fires a GET against /api/jobs/list
// with the given query string and returns the decoded response + status code.
func getJobsList(t *testing.T, query string) (JobsListResponse, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/list?"+query, nil)
	w := httptest.NewRecorder()
	handleJobsList(w, req)
	resp := w.Result()
	var data JobsListResponse
	json.NewDecoder(resp.Body).Decode(&data)
	return data, resp.StatusCode
}

// getJobsListError fires a GET and returns the raw APIError + status code.
func getJobsListError(t *testing.T, query string) (APIError, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/list?"+query, nil)
	w := httptest.NewRecorder()
	handleJobsList(w, req)
	resp := w.Result()
	var data APIError
	json.NewDecoder(resp.Body).Decode(&data)
	return data, resp.StatusCode
}

// ── Method validation ─────────────────────────────────────────────────────────

func TestHandlerJobsList_WrongMethod_POST(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/jobs/list", nil)
	w := httptest.NewRecorder()
	handleJobsList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandlerJobsList_WrongMethod_PUT(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/api/jobs/list", nil)
	w := httptest.NewRecorder()
	handleJobsList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ── Default params ────────────────────────────────────────────────────────────

func TestHandlerJobsList_DefaultParams_EmptyDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	data, status := getJobsList(t, "")
	if status != http.StatusOK {
		t.Errorf("expected 200, got %d", status)
	}
	if data.Total != 0 {
		t.Errorf("expected total 0, got %d", data.Total)
	}
	if data.Page != 1 {
		t.Errorf("expected page 1, got %d", data.Page)
	}
	if data.PerPage != 25 {
		t.Errorf("expected per_page 25, got %d", data.PerPage)
	}
	if data.Jobs == nil {
		t.Error("expected jobs to be non-nil empty slice, got nil")
	}
}

func TestHandlerJobsList_DefaultParams_WithJobs(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Go Engineer", "Acme", "DC", "desc")
	dbInsertJob("https://example.com/j2", "Python Dev", "Globex", "VA", "desc")

	data, status := getJobsList(t, "")
	if status != http.StatusOK {
		t.Errorf("expected 200, got %d", status)
	}
	if data.Total != 2 {
		t.Errorf("expected total 2, got %d", data.Total)
	}
	if len(data.Jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(data.Jobs))
	}
	if data.TotalPages != 1 {
		t.Errorf("expected 1 page, got %d", data.TotalPages)
	}
}

// ── Response shape ────────────────────────────────────────────────────────────

func TestHandlerJobsList_ResponseShape(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rid, _ := dbInsertResume("v1", "resume")
	jid, _ := dbInsertJob("https://example.com/j1", "Go Engineer", "Acme", "DC", "desc")
	dbInsertAnalysis(Analysis{
		JobID: jid, ResumeID: rid,
		Score: 4, AdjustedScore: 3,
		LLMProvider: "anthropic", LLMModel: "claude-opus-4-5",
	})

	data, status := getJobsList(t, "")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(data.Jobs) == 0 {
		t.Fatal("expected at least 1 job")
	}

	job := data.Jobs[0]
	if job.ID == 0 {
		t.Error("expected non-zero job ID")
	}
	if job.Title != "Go Engineer" {
		t.Errorf("expected title 'Go Engineer', got %q", job.Title)
	}
	if job.AdjustedScore == nil || *job.AdjustedScore != 3 {
		t.Errorf("expected adjusted_score 3, got %v", job.AdjustedScore)
	}
	if job.BestScore == nil || *job.BestScore != 4 {
		t.Errorf("expected best_score 4, got %v", job.BestScore)
	}
	if job.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", job.Provider)
	}
}

func TestHandlerJobsList_IsManualFlag(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("manual://abc123", "Pasted Job", "", "", "desc")
	dbInsertJob("https://example.com/j1", "Scraped Job", "", "", "desc")

	data, status := getJobsList(t, "")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(data.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(data.Jobs))
	}

	var manualCount int
	for _, j := range data.Jobs {
		if j.IsManual {
			manualCount++
		}
	}
	if manualCount != 1 {
		t.Errorf("expected 1 manual job, got %d", manualCount)
	}
}

// ── Parameter validation ──────────────────────────────────────────────────────

func TestHandlerJobsList_InvalidPage_Letters(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	errData, status := getJobsListError(t, "page=abc")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if errData.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandlerJobsList_InvalidPage_Negative(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	errData, status := getJobsListError(t, "page=-1")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if errData.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandlerJobsList_InvalidPage_Zero(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	errData, status := getJobsListError(t, "page=0")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if errData.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandlerJobsList_InvalidPerPage_Negative(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	errData, status := getJobsListError(t, "per_page=-5")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if errData.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandlerJobsList_InvalidPerPage_Letters(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	errData, status := getJobsListError(t, "per_page=xyz")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if errData.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandlerJobsList_InvalidStatus(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	errData, status := getJobsListError(t, "status=unknown_status")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if errData.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandlerJobsList_InvalidScore(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	errData, status := getJobsListError(t, "score=9")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if errData.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandlerJobsList_InvalidProvider(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	errData, status := getJobsListError(t, "provider=badprovider")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if errData.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandlerJobsList_OpenAIProviderIsValid(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	_, status := getJobsListError(t, "provider=openai")
	if status == http.StatusBadRequest {
		t.Error("expected openai to be a valid provider, got 400")
	}
}

func TestHandlerJobsList_GeminiProviderIsValid(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	_, status := getJobsListError(t, "provider=gemini")
	if status == http.StatusBadRequest {
		t.Error("expected gemini to be a valid provider, got 400")
	}
}

// ── Valid filter params ───────────────────────────────────────────────────────

func TestHandlerJobsList_ValidStatus_AllValues(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	statuses := []string{"not_applied", "applied", "interviewing", "offered", "rejected"}
	for _, s := range statuses {
		_, status := getJobsList(t, "status="+s)
		if status != http.StatusOK {
			t.Errorf("expected 200 for status=%q, got %d", s, status)
		}
	}
}

func TestHandlerJobsList_ValidScore_AllValues(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	scores := []string{"0", "1", "2", "3", "4", "5"}
	for _, s := range scores {
		_, status := getJobsList(t, "score="+s)
		if status != http.StatusOK {
			t.Errorf("expected 200 for score=%q, got %d", s, status)
		}
	}
}

func TestHandlerJobsList_ValidProvider_AllValues(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	providers := []string{"anthropic", "ollama", "manual"}
	for _, p := range providers {
		_, status := getJobsList(t, "provider="+p)
		if status != http.StatusOK {
			t.Errorf("expected 200 for provider=%q, got %d", p, status)
		}
	}
}

// ── Pagination ────────────────────────────────────────────────────────────────

func TestHandlerJobsList_Pagination_Page1(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 30; i++ {
		dbInsertJob("https://example.com/j"+string(rune('a'+i%26))+string(rune('0'+i/26)), "Job", "", "", "desc")
	}

	data, status := getJobsList(t, "page=1&per_page=10")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if data.Total != 30 {
		t.Errorf("expected total 30, got %d", data.Total)
	}
	if len(data.Jobs) != 10 {
		t.Errorf("expected 10 jobs on page 1, got %d", len(data.Jobs))
	}
	if data.TotalPages != 3 {
		t.Errorf("expected 3 total pages, got %d", data.TotalPages)
	}
	if data.Page != 1 {
		t.Errorf("expected page 1, got %d", data.Page)
	}
}

func TestHandlerJobsList_Pagination_Page2(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 30; i++ {
		dbInsertJob("https://example.com/j"+string(rune('a'+i%26))+string(rune('0'+i/26)), "Job", "", "", "desc")
	}

	data1, _ := getJobsList(t, "page=1&per_page=10")
	data2, status := getJobsList(t, "page=2&per_page=10")

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(data2.Jobs) != 10 {
		t.Errorf("expected 10 jobs on page 2, got %d", len(data2.Jobs))
	}
	if len(data1.Jobs) > 0 && len(data2.Jobs) > 0 && data1.Jobs[0].ID == data2.Jobs[0].ID {
		t.Error("page 1 and page 2 returned the same first job")
	}
}

func TestHandlerJobsList_Pagination_PerPage0ReturnsAll(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		dbInsertJob("https://example.com/j"+string(rune('0'+i)), "Job", "", "", "desc")
	}

	data, status := getJobsList(t, "per_page=0")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(data.Jobs) != 5 {
		t.Errorf("expected all 5 jobs with per_page=0, got %d", len(data.Jobs))
	}
}

func TestHandlerJobsList_Pagination_PageBeyondRange(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Job", "", "", "desc")

	data, status := getJobsList(t, "page=999&per_page=25")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if data.Total != 1 {
		t.Errorf("expected total 1, got %d", data.Total)
	}
	if len(data.Jobs) != 0 {
		t.Errorf("expected 0 jobs on out-of-range page, got %d", len(data.Jobs))
	}
}

// ── Search ────────────────────────────────────────────────────────────────────

func TestHandlerJobsList_SearchParam(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Go Engineer", "Acme", "DC", "desc")
	dbInsertJob("https://example.com/j2", "Python Developer", "Globex", "VA", "desc")

	data, status := getJobsList(t, "search=engineer")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if data.Total != 1 {
		t.Errorf("expected 1 result for search=engineer, got %d", data.Total)
	}
}

func TestHandlerJobsList_SearchParam_NoMatch(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/j1", "Go Engineer", "Acme", "DC", "desc")

	data, status := getJobsList(t, "search=zzznomatch")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if data.Total != 0 {
		t.Errorf("expected 0 results, got %d", data.Total)
	}
	if len(data.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(data.Jobs))
	}
}

// ── Task 3.2 — Resume Comparison ──────────────────────────────────────────────

func TestBuildComparison_NilWhenLessThanTwoResumes(t *testing.T) {
	if buildComparison(nil) != nil {
		t.Error("expected nil for empty analyses")
	}
	single := []Analysis{{ResumeID: 1, ResumeLabel: "A", AdjustedScore: 4}}
	if buildComparison(single) != nil {
		t.Error("expected nil for single analysis")
	}
}

func TestBuildComparison_NilWhenSameResume(t *testing.T) {
	analyses := []Analysis{
		{ResumeID: 1, ResumeLabel: "A", AdjustedScore: 4},
		{ResumeID: 1, ResumeLabel: "A", AdjustedScore: 3},
	}
	if buildComparison(analyses) != nil {
		t.Error("expected nil when both analyses use same resume")
	}
}

func TestBuildComparison_ReturnsTwoMostRecent(t *testing.T) {
	analyses := []Analysis{
		{ResumeID: 1, ResumeLabel: "Resume A", AdjustedScore: 4},
		{ResumeID: 2, ResumeLabel: "Resume B", AdjustedScore: 3},
	}
	cmp := buildComparison(analyses)
	if cmp == nil {
		t.Fatal("expected non-nil comparison")
	}
	if cmp.BetterFit == "" {
		t.Error("expected a BetterFit verdict")
	}
}

func TestDetermineBetterFit_BlockerWins(t *testing.T) {
	a := Analysis{ResumeLabel: "A", AdjustedScore: 4,
		MissingSkills: []MissingSkill{{Skill: "Clearance", Severity: "blocker"}}}
	b := Analysis{ResumeLabel: "B", AdjustedScore: 2, MissingSkills: []MissingSkill{}}
	winner, _ := determineBetterFit(a, b)
	if winner != "B" {
		t.Errorf("expected B to win (no blocker), got %q", winner)
	}
}

func TestDetermineBetterFit_HigherScoreWins(t *testing.T) {
	a := Analysis{ResumeLabel: "A", AdjustedScore: 5}
	b := Analysis{ResumeLabel: "B", AdjustedScore: 3}
	winner, _ := determineBetterFit(a, b)
	if winner != "A" {
		t.Errorf("expected A to win (higher score), got %q", winner)
	}
}

func TestDetermineBetterFit_TieCase(t *testing.T) {
	a := Analysis{ResumeLabel: "A", AdjustedScore: 4}
	b := Analysis{ResumeLabel: "B", AdjustedScore: 4}
	winner, _ := determineBetterFit(a, b)
	if winner != "Tie" {
		t.Errorf("expected 'Tie', got %q", winner)
	}
}

func TestHasBlocker_TrueWhenBlockerPresent(t *testing.T) {
	skills := []MissingSkill{{Skill: "Clearance", Severity: "blocker"}}
	if !hasBlocker(skills) {
		t.Error("expected hasBlocker to return true")
	}
}

func TestHasBlocker_FalseWhenNoBlocker(t *testing.T) {
	skills := []MissingSkill{{Skill: "AWS", Severity: "major"}}
	if hasBlocker(skills) {
		t.Error("expected hasBlocker to return false")
	}
}
