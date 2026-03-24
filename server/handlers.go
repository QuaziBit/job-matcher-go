package server

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuaziBit/job-matcher-go/config"
)

var appCfg config.Config

// ── Request logging middleware ────────────────────────────────────────────────

type loggedMux struct{ mux *http.ServeMux }

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (lm *loggedMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := &statusRecorder{ResponseWriter: w, status: 200}
	lm.mux.ServeHTTP(rec, r)
	if !strings.HasPrefix(r.URL.Path, "/static/") {
		sym := "→"
		if rec.status >= 400 {
			sym = "✗"
		}
		log.Printf("%s %s %s %d", sym, r.Method, r.URL.Path, rec.status)
	}
}

// ── Route registration ────────────────────────────────────────────────────────

func registerRoutes(mux *http.ServeMux, cfg config.Config) {
	appCfg = cfg

	// Static files — bypass http.FileServer to set correct MIME types
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		fsPath := "embedded" + r.URL.Path
		data, err := embeddedFS.ReadFile(fsPath)
		if err != nil {
			log.Printf("✗ Static file not found: %s (embedded path: %s)", r.URL.Path, fsPath)
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.HasSuffix(fsPath, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(fsPath, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		default:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/job/", handleJobDetail)
	mux.HandleFunc("/resumes", handleResumes)

	mux.HandleFunc("/api/jobs/add-manual", handleAddJobManual)
	mux.HandleFunc("/api/jobs/add", handleAddJob)
	mux.HandleFunc("/api/jobs/list", handleJobsList)
	mux.HandleFunc("/api/jobs/", handleJobActions)
	mux.HandleFunc("/api/analyses/", handleAnalysisActions)
	mux.HandleFunc("/api/resumes/add", handleAddResume)
	mux.HandleFunc("/api/resumes/", handleResumeActions)
	mux.HandleFunc("/shutdown", handleShutdown)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// parseAnyForm handles both multipart/form-data (from fetch FormData)
// and application/x-www-form-urlencoded (from native HTML forms)
func parseAnyForm(r *http.Request) error {
	if err := r.ParseMultipartForm(10 << 20); err == nil {
		return nil
	}
	return r.ParseForm()
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("✗ Failed to encode JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	log.Printf("✗ ERROR %d: %s", status, msg)
	writeJSON(w, status, APIError{Error: msg})
}

func parseIDFromPath(path, prefix string) (int64, error) {
	idStr := strings.TrimPrefix(path, prefix)
	idStr = strings.Split(idStr, "/")[0]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		log.Printf("✗ Failed to parse ID from path %q (prefix %q): %v", path, prefix, err)
	}
	return id, err
}

func renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	tmpl, err := parseTemplate(name)
	if err != nil {
		log.Printf("✗ Template parse error [%s]: %v", name, err)
		http.Error(w, fmt.Sprintf("template parse error: %v", err), http.StatusInternalServerError)
		return
	}
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		log.Printf("✗ Template execute error [%s]: %v", name, err)
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := fmt.Fprint(w, buf.String()); err != nil {
		log.Printf("✗ Failed to write template response [%s]: %v", name, err)
	}
}

// ── Page handlers ─────────────────────────────────────────────────────────────

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	resumes, err := dbGetResumes()
	if err != nil {
		log.Printf("✗ dbGetResumes error (index): %v", err)
		http.Error(w, "failed to load resumes", http.StatusInternalServerError)
		return
	}
	// Jobs list is loaded client-side via /api/jobs/list on page load
	renderTemplate(w, "index.html", IndexView{Jobs: nil, Resumes: resumes})
}

func handleJobDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/job/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	job, err := dbGetJobByID(id)
	if err != nil {
		log.Printf("✗ dbGetJobByID(%d) error: %v", id, err)
		http.Error(w, "failed to load job", http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.NotFound(w, r)
		return
	}
	app, err := dbGetApplicationByJobID(id)
	if err != nil {
		log.Printf("✗ dbGetApplicationByJobID(%d) error: %v", id, err)
	}
	if app == nil {
		app = &Application{JobID: id, Status: "not_applied"}
	}
	analyses, err := dbGetAnalysesByJobID(id)
	if err != nil {
		log.Printf("✗ dbGetAnalysesByJobID(%d) error: %v", id, err)
	}
	resumes, err := dbGetResumes()
	if err != nil {
		log.Printf("✗ dbGetResumes error (job detail): %v", err)
	}
	renderTemplate(w, "job_detail.html", JobDetailView{
		Job:         *job,
		Application: *app,
		Analyses:    analyses,
		Resumes:     resumes,
		OllamaModel: appCfg.OllamaModel,
	})
}

func handleResumes(w http.ResponseWriter, r *http.Request) {
	resumes, err := dbGetResumes()
	if err != nil {
		log.Printf("✗ dbGetResumes error (resumes page): %v", err)
		http.Error(w, "failed to load resumes", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, "resumes.html", ResumesView{Resumes: resumes})
}

// ── Job list API (search + filter + pagination) ───────────────────────────────

func handleJobsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	q := r.URL.Query()

	// Validate and parse page
	page := 1
	if raw := q.Get("page"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil || p < 1 {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("invalid page %q — must be a positive integer", raw))
			return
		}
		page = p
	}

	// Validate and parse per_page
	perPage := 25
	if raw := q.Get("per_page"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil || p < 0 {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("invalid per_page %q — must be 0 (all) or a positive integer", raw))
			return
		}
		perPage = p
	}

	// Validate status
	status := q.Get("status")
	validStatuses := map[string]bool{
		"": true, "not_applied": true, "applied": true,
		"interviewing": true, "offered": true, "rejected": true,
	}
	if !validStatuses[status] {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("invalid status %q — must be one of: not_applied, applied, interviewing, offered, rejected", status))
		return
	}

	// Validate score
	score := q.Get("score")
	validScores := map[string]bool{"": true, "0": true, "1": true, "2": true, "3": true, "4": true, "5": true}
	if !validScores[score] {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("invalid score %q — must be one of: 0, 2, 3, 4, 5", score))
		return
	}

	// Validate provider
	provider := q.Get("provider")
	validProviders := map[string]bool{"": true, "anthropic": true, "ollama": true, "manual": true}
	if !validProviders[provider] {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("invalid provider %q — must be one of: anthropic, ollama, manual", provider))
		return
	}

	f := JobFilters{
		Search:   strings.TrimSpace(q.Get("search")),
		Status:   status,
		Score:    score,
		Provider: provider,
		Page:     page,
		PerPage:  perPage,
	}

	log.Printf("→ /api/jobs/list page=%d per_page=%d search=%q status=%q score=%q provider=%q",
		f.Page, f.PerPage, f.Search, f.Status, f.Score, f.Provider)

	jobs, total, err := dbGetJobListItems(f)
	if err != nil {
		log.Printf("✗ dbGetJobListItems error: %v", err)
		writeError(w, http.StatusInternalServerError,
			"Failed to load jobs from database. Check the terminal for details.")
		return
	}

	totalPages := 1
	if perPage > 0 && total > 0 {
		totalPages = (total + perPage - 1) / perPage
	}

	// Clamp page to valid range
	if page > totalPages && totalPages > 0 {
		log.Printf("→ page %d out of range (max %d) — clamping", page, totalPages)
		page = totalPages
	}

	if jobs == nil {
		jobs = []JobListItem{}
	}

	log.Printf("✓ /api/jobs/list total=%d page=%d/%d returned=%d", total, page, totalPages, len(jobs))
	writeJSON(w, http.StatusOK, JobsListResponse{
		Jobs:       jobs,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	})
}

// ── Job API ───────────────────────────────────────────────────────────────────

func handleAddJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	jobURL := strings.TrimSpace(r.FormValue("url"))
	if jobURL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	log.Printf("→ Scraping URL: %s", jobURL)

	existing, err := dbGetJobByURL(jobURL)
	if err != nil {
		log.Printf("✗ dbGetJobByURL error: %v", err)
	}
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "This URL has already been added.", "job_id": existing.ID, "title": existing.Title,
		})
		return
	}

	title, company, location, description, err := ScrapeJob(jobURL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	log.Printf("✓ Scraped: %q (%s)", title, company)

	id, err := dbInsertJob(jobURL, title, company, location, description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbInsertJob error: %v", err))
		return
	}
	log.Printf("✓ Job saved id=%d: %q", id, title)
	writeJSON(w, http.StatusOK, map[string]interface{}{"job_id": id, "title": title, "company": company})
}

func handleAddJobManual(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	company := strings.TrimSpace(r.FormValue("company"))
	description := strings.TrimSpace(r.FormValue("description"))

	log.Printf("→ Manual job: title=%q company=%q desc_len=%d", title, company, len(description))

	if len(description) < 50 {
		writeError(w, http.StatusUnprocessableEntity, "Description is too short (minimum 50 characters).")
		return
	}
	if title == "" {
		title = "Untitled Job"
	}

	slug := fmt.Sprintf("%x", md5.Sum([]byte(description[:min(200, len(description))])))
	syntheticURL := "manual://" + slug[:12]

	existing, err := dbGetJobByURL(syntheticURL)
	if err != nil {
		log.Printf("✗ dbGetJobByURL (manual) error: %v", err)
	}
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "This description has already been added.", "job_id": existing.ID,
		})
		return
	}
	if len(description) > 8000 {
		description = description[:8000] + "\n\n[...truncated for analysis]"
	}

	id, err := dbInsertJob(syntheticURL, title, company, "", description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbInsertJob (manual) error: %v", err))
		return
	}
	log.Printf("✓ Manual job saved id=%d: %q", id, title)
	writeJSON(w, http.StatusOK, map[string]interface{}{"job_id": id, "title": title, "company": company})
}

func handleJobActions(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if r.Method == http.MethodDelete {
		id, err := parseIDFromPath(path, "/api/jobs/")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := dbDeleteJob(id); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbDeleteJob(%d) error: %v", id, err))
			return
		}
		log.Printf("✓ Job deleted id=%d", id)
		writeJSON(w, http.StatusOK, APIOK{OK: true})
		return
	}

	if r.Method == http.MethodGet && strings.HasSuffix(path, "/description") {
		id, err := parseIDFromPath(path, "/api/jobs/")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		job, err := dbGetJobByID(id)
		if err != nil {
			log.Printf("✗ dbGetJobByID(%d) error: %v", id, err)
			http.Error(w, "failed to load job", http.StatusInternalServerError)
			return
		}
		if job == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"description": job.RawDescription})
		return
	}

	if r.Method == http.MethodPost && strings.HasSuffix(path, "/analyze") {
		handleAnalyzeJob(w, r)
		return
	}

	if r.Method == http.MethodPost && strings.HasSuffix(path, "/application") {
		handleUpsertApplication(w, r)
		return
	}

	log.Printf("✗ Unhandled job action: %s %s", r.Method, path)
	http.NotFound(w, r)
}

func handleAnalyzeJob(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	jobID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		log.Printf("✗ Invalid job ID in analyze path %q: %v", r.URL.Path, err)
		http.NotFound(w, r)
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	resumeID, err := strconv.ParseInt(r.FormValue("resume_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid resume_id %q: %v", r.FormValue("resume_id"), err))
		return
	}
	provider := r.FormValue("provider")
	if provider == "" {
		provider = "anthropic"
	}

	job, err := dbGetJobByID(jobID)
	if err != nil {
		log.Printf("✗ dbGetJobByID(%d) error: %v", jobID, err)
		http.Error(w, "failed to load job", http.StatusInternalServerError)
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("job id=%d not found", jobID))
		return
	}

	resume, err := dbGetResumeByID(resumeID)
	if err != nil {
		log.Printf("✗ dbGetResumeByID(%d) error: %v", resumeID, err)
		http.Error(w, "failed to load resume", http.StatusInternalServerError)
		return
	}
	if resume == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("resume id=%d not found", resumeID))
		return
	}

	log.Printf("→ Analyzing job=%d resume=%d provider=%s", jobID, resumeID, provider)
	analysis, err := AnalyzeMatch(resume.Content, job.RawDescription, provider, appCfg)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("analysis failed: %v", err))
		return
	}

	analysis.JobID = jobID
	analysis.ResumeID = resumeID

	if _, err := dbInsertAnalysis(analysis); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbInsertAnalysis error: %v", err))
		return
	}
	log.Printf("✓ Analysis saved: score=%d adjusted=%d provider=%s model=%s",
		analysis.Score, analysis.AdjustedScore, analysis.LLMProvider, analysis.LLMModel)
	writeJSON(w, http.StatusOK, analysis)
}

func handleUpsertApplication(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	jobID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		log.Printf("✗ Invalid job ID in application path %q: %v", r.URL.Path, err)
		http.NotFound(w, r)
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	app := Application{
		JobID:          jobID,
		Status:         r.FormValue("status"),
		RecruiterName:  r.FormValue("recruiter_name"),
		RecruiterEmail: r.FormValue("recruiter_email"),
		RecruiterPhone: r.FormValue("recruiter_phone"),
		Notes:          r.FormValue("notes"),
	}
	if app.Status == "" {
		app.Status = "not_applied"
	}
	if err := dbUpsertApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbUpsertApplication(%d) error: %v", jobID, err))
		return
	}
	log.Printf("✓ Application saved job=%d status=%s", jobID, app.Status)
	writeJSON(w, http.StatusOK, APIOK{OK: true})
}

// ── Analysis API ──────────────────────────────────────────────────────────────

func handleAnalysisActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/analyses/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	found, err := dbDeleteAnalysis(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbDeleteAnalysis(%d) error: %v", id, err))
		return
	}
	if !found {
		log.Printf("✗ Analysis id=%d not found for delete", id)
		http.NotFound(w, r)
		return
	}
	log.Printf("✓ Analysis deleted id=%d", id)
	writeJSON(w, http.StatusOK, APIOK{OK: true})
}

// ── Resume API ────────────────────────────────────────────────────────────────

func handleAddResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	content := strings.TrimSpace(r.FormValue("content"))

	log.Printf("→ Add resume: label=%q content_len=%d", label, len(content))

	if label == "" || content == "" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"label and content are required (got label=%q content_len=%d)", label, len(content)))
		return
	}
	id, err := dbInsertResume(label, content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbInsertResume error: %v", err))
		return
	}
	log.Printf("✓ Resume saved id=%d label=%q", id, label)
	writeJSON(w, http.StatusOK, map[string]interface{}{"resume_id": id, "label": label})
}

func handleResumeActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/resumes/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := dbDeleteResume(id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbDeleteResume(%d) error: %v", id, err))
		return
	}
	log.Printf("✓ Resume deleted id=%d", id)
	writeJSON(w, http.StatusOK, APIOK{OK: true})
}

// ── Shutdown ──────────────────────────────────────────────────────────────────

func handleShutdown(w http.ResponseWriter, r *http.Request) {
	log.Printf("→ Shutdown requested")
	writeJSON(w, http.StatusOK, APIOK{OK: true})
	go func() {
		if db != nil {
			if err := db.Close(); err != nil {
				log.Printf("✗ DB close error on shutdown: %v", err)
			}
		}
	}()
}
