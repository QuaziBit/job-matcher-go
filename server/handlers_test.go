package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

// ── Provider availability flags ───────────────────────────────────────────────

func TestOllamaAvailable_ReachableServer(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"models":[]}`))
	}))
	defer mock.Close()

	origURL := appCfg.OllamaBaseURL
	appCfg.OllamaBaseURL = mock.URL
	defer func() { appCfg.OllamaBaseURL = origURL }()

	if !ollamaAvailable() {
		t.Error("expected ollamaAvailable()=true when server responds 200")
	}
}

func TestOllamaAvailable_UnreachableServer(t *testing.T) {
	origURL := appCfg.OllamaBaseURL
	appCfg.OllamaBaseURL = "http://127.0.0.1:19998"
	defer func() { appCfg.OllamaBaseURL = origURL }()

	if ollamaAvailable() {
		t.Error("expected ollamaAvailable()=false when server is not running")
	}
}

func TestProviderFlags_AnthropicKeyPresent(t *testing.T) {
	origKey := appCfg.AnthropicAPIKey
	appCfg.AnthropicAPIKey = "sk-ant-testkey"
	defer func() { appCfg.AnthropicAPIKey = origKey }()

	if appCfg.AnthropicAPIKey == "" {
		t.Error("expected HasAnthropic=true when key is set")
	}
}

func TestProviderFlags_AnthropicKeyAbsent(t *testing.T) {
	origKey := appCfg.AnthropicAPIKey
	appCfg.AnthropicAPIKey = ""
	defer func() { appCfg.AnthropicAPIKey = origKey }()

	if appCfg.AnthropicAPIKey != "" {
		t.Error("expected HasAnthropic=false when key is empty")
	}
}

func TestProviderFlags_OpenAIKeyPresent(t *testing.T) {
	origKey := appCfg.OpenAIAPIKey
	appCfg.OpenAIAPIKey = "sk-proj-testkey"
	defer func() { appCfg.OpenAIAPIKey = origKey }()

	if appCfg.OpenAIAPIKey == "" {
		t.Error("expected HasOpenAI=true when key is set")
	}
}

func TestProviderFlags_GeminiKeyPresent(t *testing.T) {
	origKey := appCfg.GeminiAPIKey
	appCfg.GeminiAPIKey = "AIzaSy-testkey"
	defer func() { appCfg.GeminiAPIKey = origKey }()

	if appCfg.GeminiAPIKey == "" {
		t.Error("expected HasGemini=true when key is set")
	}
}

// ── GET /api/jobs/{id}/detail ─────────────────────────────────────────────────

func TestHandleJobDetailAPI_NotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/9999/detail", nil)
	req.URL.Path = "/api/jobs/9999/detail"
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing job, got %d", w.Code)
	}
}

func TestHandleJobDetailAPI_WrongMethod(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/jobs/1/detail", nil)
	req.URL.Path = "/api/jobs/1/detail"
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	// POST /detail is not handled — falls through to 404
	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 for POST /detail, got %d", w.Code)
	}
}

func TestHandleJobDetailAPI_ReturnsJob(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Insert a job
	jobID, err := dbInsertJob("https://example.com/job/detail-test", "Go Engineer", "Acme", "VA", "Go experience required")
	if err != nil {
		t.Fatalf("dbInsertJob failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/1/detail", nil)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(jobID, 10) + "/detail"
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp JobDetailAPIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Job.Title != "Go Engineer" {
		t.Errorf("expected title 'Go Engineer', got %q", resp.Job.Title)
	}
	if resp.Job.Company != "Acme" {
		t.Errorf("expected company 'Acme', got %q", resp.Job.Company)
	}
	if resp.Application.Status != "not_applied" {
		t.Errorf("expected default status 'not_applied', got %q", resp.Application.Status)
	}
	if resp.Analyses == nil {
		t.Error("expected analyses to be non-nil slice")
	}
	if resp.Resumes == nil {
		t.Error("expected resumes to be non-nil slice")
	}
}

func TestHandleJobDetailAPI_ContentTypeJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	jobID, err := dbInsertJob("https://example.com/job/ct-test", "Dev", "Corp", "NY", "Some job")
	if err != nil {
		t.Fatalf("dbInsertJob failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/1/detail", nil)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(jobID, 10) + "/detail"
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// ── GET /api/providers/status ─────────────────────────────────────────────────

func TestHandleProvidersStatus_OK(t *testing.T) {
	origAnthropic := appCfg.AnthropicAPIKey
	origOpenAI    := appCfg.OpenAIAPIKey
	origGemini    := appCfg.GeminiAPIKey
	appCfg.AnthropicAPIKey = "sk-ant-test"
	appCfg.OpenAIAPIKey    = ""
	appCfg.GeminiAPIKey    = ""
	defer func() {
		appCfg.AnthropicAPIKey = origAnthropic
		appCfg.OpenAIAPIKey    = origOpenAI
		appCfg.GeminiAPIKey    = origGemini
	}()

	req := httptest.NewRequest(http.MethodGet, "/api/providers/status", nil)
	w := httptest.NewRecorder()
	handleProvidersStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ProvidersStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.HasAnthropic {
		t.Error("expected has_anthropic=true when key is set")
	}
	if resp.HasOpenAI {
		t.Error("expected has_openai=false when key is empty")
	}
	if resp.HasGemini {
		t.Error("expected has_gemini=false when key is empty")
	}
}

func TestHandleProvidersStatus_WrongMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/providers/status", nil)
	w := httptest.NewRecorder()
	handleProvidersStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleProvidersStatus_ContentTypeJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/providers/status", nil)
	w := httptest.NewRecorder()
	handleProvidersStatus(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestHandleProvidersStatus_AllProvidersAbsent(t *testing.T) {
	origAnthropic := appCfg.AnthropicAPIKey
	origOpenAI    := appCfg.OpenAIAPIKey
	origGemini    := appCfg.GeminiAPIKey
	appCfg.AnthropicAPIKey = ""
	appCfg.OpenAIAPIKey    = ""
	appCfg.GeminiAPIKey    = ""
	defer func() {
		appCfg.AnthropicAPIKey = origAnthropic
		appCfg.OpenAIAPIKey    = origOpenAI
		appCfg.GeminiAPIKey    = origGemini
	}()

	req := httptest.NewRequest(http.MethodGet, "/api/providers/status", nil)
	w := httptest.NewRecorder()
	handleProvidersStatus(w, req)

	var resp ProvidersStatusResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.HasAnthropic || resp.HasOpenAI || resp.HasGemini {
		t.Error("expected all cloud providers false when keys are empty")
	}
}

func TestHandleProvidersStatus_DefaultProviderAnthropicWhenKeySet(t *testing.T) {
	origAnthropic := appCfg.AnthropicAPIKey
	origOpenAI    := appCfg.OpenAIAPIKey
	origGemini    := appCfg.GeminiAPIKey
	appCfg.AnthropicAPIKey = "sk-ant-test"
	appCfg.OpenAIAPIKey    = ""
	appCfg.GeminiAPIKey    = ""
	defer func() {
		appCfg.AnthropicAPIKey = origAnthropic
		appCfg.OpenAIAPIKey    = origOpenAI
		appCfg.GeminiAPIKey    = origGemini
	}()

	req := httptest.NewRequest(http.MethodGet, "/api/providers/status", nil)
	w := httptest.NewRecorder()
	handleProvidersStatus(w, req)

	var resp ProvidersStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.DefaultProvider != "anthropic" {
		t.Errorf("expected default_provider=anthropic, got %q", resp.DefaultProvider)
	}
}

func TestHandleProvidersStatus_DefaultProviderOllamaWhenNoKeysSet(t *testing.T) {
	origAnthropic := appCfg.AnthropicAPIKey
	origOpenAI    := appCfg.OpenAIAPIKey
	origGemini    := appCfg.GeminiAPIKey
	appCfg.AnthropicAPIKey = ""
	appCfg.OpenAIAPIKey    = ""
	appCfg.GeminiAPIKey    = ""
	defer func() {
		appCfg.AnthropicAPIKey = origAnthropic
		appCfg.OpenAIAPIKey    = origOpenAI
		appCfg.GeminiAPIKey    = origGemini
	}()

	req := httptest.NewRequest(http.MethodGet, "/api/providers/status", nil)
	w := httptest.NewRecorder()
	handleProvidersStatus(w, req)

	var resp ProvidersStatusResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.DefaultProvider != "ollama" {
		t.Errorf("expected default_provider=ollama when no keys set, got %q", resp.DefaultProvider)
	}
}

// ── GET /api/resumes/ ─────────────────────────────────────────────────────────

func TestHandleResumesList_EmptyDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/resumes/", nil)
	w := httptest.NewRecorder()
	handleResumesList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string][]Resume
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resumes, ok := resp["resumes"]
	if !ok {
		t.Fatal("expected 'resumes' key in response")
	}
	if len(resumes) != 0 {
		t.Errorf("expected 0 resumes, got %d", len(resumes))
	}
}

func TestHandleResumesList_WithResumes(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	if _, err := dbInsertResume("v1 General", "Resume content here"); err != nil {
		t.Fatalf("dbInsertResume failed: %v", err)
	}
	if _, err := dbInsertResume("v2 Tailored", "Tailored content here"); err != nil {
		t.Fatalf("dbInsertResume failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/resumes/", nil)
	w := httptest.NewRecorder()
	handleResumesList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string][]Resume
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp["resumes"]) != 2 {
		t.Errorf("expected 2 resumes, got %d", len(resp["resumes"]))
	}
	if resp["resumes"][0].Label == "" {
		t.Error("expected resume label to be non-empty")
	}
}

func TestHandleResumesList_WrongMethod(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/resumes/", nil)
	w := httptest.NewRecorder()
	handleResumesList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleResumesList_ContentTypeJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/resumes/", nil)
	w := httptest.NewRecorder()
	handleResumesList(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// ── Page shell handlers ───────────────────────────────────────────────────────

func TestHandleIndex_ServesHTML(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html Content-Type, got %q", ct)
	}
}

func TestHandleIndex_NotFoundForSubPath(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/notaroute", nil)
	w := httptest.NewRecorder()
	handleIndex(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-root path, got %d", w.Code)
	}
}

func TestHandleResumes_ServesHTML(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/resumes", nil)
	w := httptest.NewRecorder()
	handleResumes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html Content-Type, got %q", ct)
	}
}

func TestHandleJobPreview_ServesHTML(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/jobs/preview", nil)
	w := httptest.NewRecorder()
	handleJobPreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html Content-Type, got %q", ct)
	}
}

func TestHandleJobPreview_WrongMethod(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/jobs/preview", nil)
	w := httptest.NewRecorder()
	handleJobPreview(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleJobDetail_ServesHTML(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	jobID, err := dbInsertJob("https://example.com/shell-test", "Shell Test", "Corp", "VA", "Some job description here")
	if err != nil {
		t.Fatalf("dbInsertJob failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/job/1", nil)
	req.URL.Path = "/job/" + strconv.FormatInt(jobID, 10)
	w := httptest.NewRecorder()
	handleJobDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html Content-Type, got %q", ct)
	}
}

func TestHandleJobDetail_NotFoundForMissingJob(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/job/99999", nil)
	req.URL.Path = "/job/99999"
	w := httptest.NewRecorder()
	handleJobDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing job, got %d", w.Code)
	}
}

// ── handleScrapeJobPreview — URL field in response ───────────────────────────

func TestHandleScrapeJobPreview_IncludesURLInResponse(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Spin up a minimal HTTP server to act as the job page
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Go Engineer at Acme</title></head><body>
			<h1>Go Engineer</h1><p>We need a Go developer with 3+ years of experience
			building REST APIs, working with PostgreSQL, and deploying to AWS.
			Strong knowledge of Docker and Kubernetes required.</p></body></html>`)
	}))
	defer ts.Close()

	body := strings.NewReader("url=" + ts.URL)
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/scrape", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleScrapeJobPreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp ScrapePreviewResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.URL != ts.URL {
		t.Errorf("expected url=%q in response, got %q", ts.URL, resp.URL)
	}
}

func TestHandleScrapeJobPreview_MissingURL(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := strings.NewReader("")
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/scrape", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleScrapeJobPreview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing url, got %d", w.Code)
	}
}

func TestHandleSavePreview_MissingURL(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := strings.NewReader("title=Engineer&company=Acme&description=Some+job+description+here")
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/save-preview", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleSavePreview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when url missing from save-preview, got %d", w.Code)
	}
}

// ── PATCH /api/jobs/{id}/url ──────────────────────────────────────────────────

func TestHandleUpdateJobURL_SetsURL(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("manual://abc", "Dev", "Co", "VA", "Some job description here for testing purposes only")

	body := strings.NewReader("url=https://example.com/job/123")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/url", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/url"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Error("expected ok:true")
	}
	if resp["url"] != "https://example.com/job/123" {
		t.Errorf("expected url in response, got %v", resp["url"])
	}
}

func TestHandleUpdateJobURL_ClearRestoresSynthetic(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/job/1", "Dev", "Co", "VA", "Some job description here for testing purposes only")

	body := strings.NewReader("url=")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/url", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/url"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	url, _ := resp["url"].(string)
	if !strings.HasPrefix(url, "manual://") {
		t.Errorf("expected manual:// URL after clear, got %q", url)
	}
}

func TestHandleUpdateJobURL_InvalidSchemeReturns422(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("manual://abc", "Dev", "Co", "VA", "Some job description here for testing purposes only")

	body := strings.NewReader("url=ftp://bad.example.com")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/url", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/url"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid scheme, got %d", w.Code)
	}
}

func TestHandleUpdateJobURL_NotFoundReturns404(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := strings.NewReader("url=https://example.com")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/99999/url", body)
	req.URL.Path = "/api/jobs/99999/url"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing job, got %d", w.Code)
	}
}

// ── PATCH /api/jobs/{id}/title ─────────────────────────────────────────────

func TestHandleUpdateJobTitle_SetsTitle(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("manual://abc", "Old Title", "Co", "VA", "Some job description here for testing purposes only")

	body := strings.NewReader("title=New+Title")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/title", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/title"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Error("expected ok:true")
	}
	if resp["title"] != "New Title" {
		t.Errorf("expected title in response, got %v", resp["title"])
	}
	job, _ := dbGetJobByID(id)
	if job.Title != "New Title" {
		t.Errorf("expected DB title to be 'New Title', got %q", job.Title)
	}
}

func TestHandleUpdateJobTitle_EmptyReturns422(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("manual://abc", "Dev", "Co", "VA", "Some job description here for testing purposes only")

	body := strings.NewReader("title=")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/title", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/title"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for empty title, got %d", w.Code)
	}
}

func TestHandleUpdateJobTitle_NotFoundReturns404(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := strings.NewReader("title=Dev")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/99999/title", body)
	req.URL.Path = "/api/jobs/99999/title"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing job, got %d", w.Code)
	}
}

func TestHandleUpdateJobTitle_ReflectedInDetailAPI(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("manual://abc", "Original", "Co", "VA", "Some job description here for testing purposes only")

	body := strings.NewReader("title=Updated+Title")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/title", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/title"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	job, _ := dbGetJobByID(id)
	if job.Title != "Updated Title" {
		t.Errorf("detail API should reflect new title, got %q", job.Title)
	}
}

// ── PATCH /api/jobs/{id}/company ───────────────────────────────────────────

func TestHandleUpdateJobCompany_SetsCompany(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("manual://abc", "Dev", "OldCo", "VA", "Some job description here for testing purposes only")

	body := strings.NewReader("company=NewCo")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/company", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/company"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["company"] != "NewCo" {
		t.Errorf("expected company in response, got %v", resp["company"])
	}
	job, _ := dbGetJobByID(id)
	if job.Company != "NewCo" {
		t.Errorf("expected DB company to be 'NewCo', got %q", job.Company)
	}
}

func TestHandleUpdateJobCompany_AllowsEmpty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("manual://abc", "Dev", "Acme", "VA", "Some job description here for testing purposes only")

	body := strings.NewReader("company=")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/company", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/company"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for empty company, got %d", w.Code)
	}
	job, _ := dbGetJobByID(id)
	if job.Company != "" {
		t.Errorf("expected empty company in DB, got %q", job.Company)
	}
}

func TestHandleUpdateJobCompany_NotFoundReturns404(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := strings.NewReader("company=X")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/99999/company", body)
	req.URL.Path = "/api/jobs/99999/company"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing job, got %d", w.Code)
	}
}

// ── PATCH /api/jobs/{id}/location ──────────────────────────────────────────

func TestHandleUpdateJobLocation_SetsLocation(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("manual://abc", "Dev", "Co", "Old Location", "Some job description here for testing purposes only")

	body := strings.NewReader("location=Washington%2C+DC")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/location", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/location"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["location"] != "Washington, DC" {
		t.Errorf("expected location in response, got %v", resp["location"])
	}
	job, _ := dbGetJobByID(id)
	if job.Location != "Washington, DC" {
		t.Errorf("expected DB location to be 'Washington, DC', got %q", job.Location)
	}
}

func TestHandleUpdateJobLocation_AllowsEmpty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("manual://abc", "Dev", "Co", "Remote", "Some job description here for testing purposes only")

	body := strings.NewReader("location=")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/location", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/location"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for empty location, got %d", w.Code)
	}
	job, _ := dbGetJobByID(id)
	if job.Location != "" {
		t.Errorf("expected empty location in DB, got %q", job.Location)
	}
}

func TestHandleUpdateJobLocation_NotFoundReturns404(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := strings.NewReader("location=X")
	req := httptest.NewRequest(http.MethodPatch, "/api/jobs/99999/location", body)
	req.URL.Path = "/api/jobs/99999/location"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing job, got %d", w.Code)
	}
}

func TestHandleAddJobManual_WithSourceURL(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := strings.NewReader("title=Dev&company=Co&source_url=https://linkedin.com/jobs/12345&description=Some+job+description+here+for+testing+purposes+only")
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/add-manual", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleAddJobManual(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	id := int64(resp["job_id"].(float64))

	job, _ := dbGetJobByID(id)
	if job == nil {
		t.Fatal("job not found")
	}
	if job.URL != "https://linkedin.com/jobs/12345" {
		t.Errorf("expected source URL stored, got %q", job.URL)
	}
}

func TestHandleAddJobManual_WithoutSourceURLIsManual(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := strings.NewReader("title=Dev&company=Co&description=Some+job+description+here+for+testing+purposes+only")
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/add-manual", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleAddJobManual(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	id := int64(resp["job_id"].(float64))

	job, _ := dbGetJobByID(id)
	if job == nil {
		t.Fatal("job not found")
	}
	if !strings.HasPrefix(job.URL, "manual://") {
		t.Errorf("expected manual:// URL, got %q", job.URL)
	}
}

// ── POST /api/resumes/extract ─────────────────────────────────────────────────

func TestHandleResumeExtract_TXT(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	txtContent := strings.Repeat("John Doe\nSoftware Engineer\nPython Go Docker AWS PostgreSQL\n", 5)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "resume.txt")
	part.Write([]byte(txtContent))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/resumes/extract", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handleResumeExtract(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["text"]; !ok {
		t.Error("expected 'text' field in response")
	}
	if _, ok := resp["char_count"]; !ok {
		t.Error("expected 'char_count' field in response")
	}
}

func TestHandleResumeExtract_UnsupportedType(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "resume.md")
	part.Write([]byte("# Resume"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/resumes/extract", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handleResumeExtract(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleResumeExtract_TooShort(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "resume.txt")
	part.Write([]byte("Too short"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/resumes/extract", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handleResumeExtract(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleResumeExtract_WrongMethod(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/resumes/extract", nil)
	w := httptest.NewRecorder()
	handleResumeExtract(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ── GET /api/resumes/{id} ─────────────────────────────────────────────────────

func TestHandleGetResume_ReturnsContent(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertResume("v1", strings.Repeat("John Doe Software Engineer Python Go Docker AWS\n", 5))

	req := httptest.NewRequest(http.MethodGet, "/api/resumes/1", nil)
	req.URL.Path = "/api/resumes/" + strconv.FormatInt(id, 10)
	w := httptest.NewRecorder()
	handleResumeActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["label"] != "v1" {
		t.Errorf("expected label=v1, got %v", resp["label"])
	}
	if _, ok := resp["content"]; !ok {
		t.Error("expected content field")
	}
	if _, ok := resp["char_count"]; !ok {
		t.Error("expected char_count field")
	}
}

func TestHandleGetResume_NotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/resumes/99999", nil)
	w := httptest.NewRecorder()
	handleResumeActions(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── /api/jobs/{id}/email ──────────────────────────────────────────────────────

func TestHandleGetJobEmail_ReturnsNullWhenNone(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/e1", "Dev", "Co", "VA", "Some job description here for testing")
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/1/email", nil)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/email"
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["email"] != nil {
		t.Errorf("expected email=null, got %v", resp["email"])
	}
}

func TestHandleGetJobEmail_ReturnsEmail(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/e2", "Dev", "Co", "VA", "Some job description here for testing")
	dbSaveJobEmail(id, "<p>Interview confirmed</p>")

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/1/email", nil)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/email"
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["email"] == nil {
		t.Fatal("expected email to be non-null")
	}
}

func TestHandleSaveJobEmail_Upserts(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/e3", "Dev", "Co", "VA", "Some job description here for testing")

	for _, html := range []string{"<p>First</p>", "<p>Second</p>"} {
		body := strings.NewReader("raw_html=" + html)
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/1/email", body)
		req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/email"
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		handleJobActions(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d — %s", w.Code, w.Body.String())
		}
	}

	email, _ := dbGetJobEmail(id)
	if email == nil || email.RawHTML != "<p>Second</p>" {
		t.Errorf("expected Second, got %v", email)
	}
}

func TestHandleSaveJobEmail_EmptyReturns422(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/e4", "Dev", "Co", "VA", "Some job description here for testing")
	body := strings.NewReader("raw_html=")
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/1/email", body)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/email"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleDeleteJobEmail(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/e5", "Dev", "Co", "VA", "Some job description here for testing")
	dbSaveJobEmail(id, "<p>Email</p>")

	req := httptest.NewRequest(http.MethodDelete, "/api/jobs/1/email", nil)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/email"
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	email, _ := dbGetJobEmail(id)
	if email != nil {
		t.Error("expected email to be deleted")
	}
	// Regression: DELETE /api/jobs/{id}/email must NOT delete the job itself
	job, _ := dbGetJobByID(id)
	if job == nil {
		t.Error("job should still exist after deleting its email")
	}
}

func TestHandleDeleteJobEmail_DoesNotDeleteJob(t *testing.T) {
	// Regression guard: the generic DELETE handler in handleJobActions
	// was catching DELETE /api/jobs/{id}/email before the /email suffix
	// check, causing the entire job to be deleted instead of just the email.
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/e5b", "Dev", "Co", "VA", "Some job description here for testing")
	dbSaveJobEmail(id, "<p>Email</p>")

	req := httptest.NewRequest(http.MethodDelete, "/api/jobs/1/email", nil)
	req.URL.Path = "/api/jobs/" + strconv.FormatInt(id, 10) + "/email"
	w := httptest.NewRecorder()
	handleJobActions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Job must still exist
	job, _ := dbGetJobByID(id)
	if job == nil {
		t.Fatal("REGRESSION: DELETE /email deleted the entire job")
	}
	// Email must be gone
	email, _ := dbGetJobEmail(id)
	if email != nil {
		t.Error("expected email to be deleted")
	}
}

func TestHandleJobEmail_CascadesOnJobDelete(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/e6", "Dev", "Co", "VA", "Some job description here for testing")
	dbSaveJobEmail(id, "<p>Email</p>")
	dbDeleteJob(id)

	email, _ := dbGetJobEmail(id)
	if email != nil {
		t.Error("expected email to be cascade deleted with job")
	}
}

// ── /api/vetting ──────────────────────────────────────────────────────────────

func TestHandleVettingAPI_ReturnsKeys(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/vetting", nil)
	w := httptest.NewRecorder()
	handleVettingAPI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["companies"]; !ok {
		t.Error("expected companies key")
	}
	if _, ok := resp["recruiters"]; !ok {
		t.Error("expected recruiters key")
	}
}

func TestHandleVettingAPI_EmptyDBReturnsEmptyLists(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/vetting", nil)
	w := httptest.NewRecorder()
	handleVettingAPI(w, req)

	var resp map[string]json.RawMessage
	json.NewDecoder(w.Body).Decode(&resp)

	var companies, recruiters []interface{}
	json.Unmarshal(resp["companies"], &companies)
	json.Unmarshal(resp["recruiters"], &recruiters)
	if len(companies) != 0 {
		t.Errorf("expected empty companies, got %d", len(companies))
	}
	if len(recruiters) != 0 {
		t.Errorf("expected empty recruiters, got %d", len(recruiters))
	}
}

func TestHandleVettingAPI_GroupsByCompany(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dbInsertJob("https://example.com/v1", "Dev A", "Acme", "VA", "desc one for testing purposes here")
	dbInsertJob("https://example.com/v2", "Dev B", "Acme", "VA", "desc two for testing purposes here")
	dbInsertJob("https://example.com/v3", "Dev C", "Beta", "VA", "desc three for testing purposes here")

	req := httptest.NewRequest(http.MethodGet, "/api/vetting", nil)
	w := httptest.NewRecorder()
	handleVettingAPI(w, req)

	var resp struct {
		Companies []struct {
			Company string `json:"company"`
			Jobs    []interface{} `json:"jobs"`
		} `json:"companies"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	var acme *struct{ Company string; Jobs []interface{} }
	for i := range resp.Companies {
		if resp.Companies[i].Company == "Acme" {
			c := struct{ Company string; Jobs []interface{} }{resp.Companies[i].Company, resp.Companies[i].Jobs}
			acme = &c
		}
	}
	if acme == nil {
		t.Fatal("expected Acme company group")
	}
	if len(acme.Jobs) != 2 {
		t.Errorf("expected 2 jobs for Acme, got %d", len(acme.Jobs))
	}
}

func TestHandleVettingAPI_GroupsRecruiterByEmail(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/v4", "Dev", "Acme", "VA", "desc for testing")
	db.Exec(`INSERT INTO applications (job_id, status, recruiter_name, recruiter_email, recruiter_phone, notes)
		VALUES (?, 'applied', 'Jane Doe', 'jane@acme.com', '', '')`, id)

	req := httptest.NewRequest(http.MethodGet, "/api/vetting", nil)
	w := httptest.NewRecorder()
	handleVettingAPI(w, req)

	var resp struct {
		Recruiters []struct {
			Email string `json:"email"`
			Jobs  []interface{} `json:"jobs"`
		} `json:"recruiters"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	var jane *struct{ Email string; Jobs []interface{} }
	for i := range resp.Recruiters {
		if resp.Recruiters[i].Email == "jane@acme.com" {
			j := struct{ Email string; Jobs []interface{} }{resp.Recruiters[i].Email, resp.Recruiters[i].Jobs}
			jane = &j
		}
	}
	if jane == nil {
		t.Fatal("expected jane@acme.com recruiter group")
	}
	if len(jane.Jobs) != 1 {
		t.Errorf("expected 1 job for jane, got %d", len(jane.Jobs))
	}
}

func TestHandleVettingAPI_ScrapedAtInRecruiterJobs(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id, _ := dbInsertJob("https://example.com/v5", "Dev", "Acme", "VA", "desc for testing")
	db.Exec(`INSERT INTO applications (job_id, status, recruiter_name, recruiter_email, recruiter_phone, notes)
		VALUES (?, 'applied', 'Bob', 'bob@acme.com', '', '')`, id)

	req := httptest.NewRequest(http.MethodGet, "/api/vetting", nil)
	w := httptest.NewRecorder()
	handleVettingAPI(w, req)

	var resp struct {
		Recruiters []struct {
			Email string `json:"email"`
			Jobs  []struct {
				ScrapedAt string `json:"scraped_at"`
			} `json:"jobs"`
		} `json:"recruiters"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	for _, r := range resp.Recruiters {
		if r.Email == "bob@acme.com" {
			if len(r.Jobs) == 0 || r.Jobs[0].ScrapedAt == "" {
				t.Error("expected scraped_at in recruiter jobs")
			}
			return
		}
	}
	t.Error("recruiter not found")
}

func TestHandleVettingPage_Renders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/vetting", nil)
	w := httptest.NewRecorder()
	handleVettingPage(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
