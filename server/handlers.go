package server

import (
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuaziBit/job-matcher-go/config"
)

var appCfg config.Config

// ollamaAvailable does a fast HEAD/GET to Ollama to check if it is reachable.
func ollamaAvailable() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(appCfg.OllamaBaseURL + "/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

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

	// Static files — served from the shared ui/static/ directory
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		fsPath := "ui" + r.URL.Path
		data, err := uiFS.ReadFile(fsPath)
		if err != nil {
			log.Printf("✗ Static file not found: %s (ui path: %s)", r.URL.Path, fsPath)
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
	mux.HandleFunc("/vetting", handleVettingPage)
	mux.HandleFunc("/api/vetting", handleVettingAPI)

	mux.HandleFunc("/jobs/preview", handleJobPreview)
	mux.HandleFunc("/api/jobs/add-manual", handleAddJobManual)
	mux.HandleFunc("/api/jobs/add", handleAddJob)
	mux.HandleFunc("/api/jobs/scrape", handleScrapeJobPreview)
	mux.HandleFunc("/api/jobs/save-preview", handleSavePreview)
	mux.HandleFunc("/api/jobs/list", handleJobsList)
	mux.HandleFunc("/api/jobs/", handleJobActions)
	mux.HandleFunc("/api/analyses/", handleAnalysisActions)
	mux.HandleFunc("/api/resumes/add", handleAddResume)
	mux.HandleFunc("/api/resumes/extract", handleResumeExtract)
	mux.HandleFunc("/api/resumes/", handleResumeActions)
	mux.HandleFunc("/api/ollama/models", handleOllamaModels)
	mux.HandleFunc("/api/providers/models", handleProviderModels)
	mux.HandleFunc("/api/companies/crawl", handleCompanyCrawl)
	mux.HandleFunc("/api/companies/meta", handleCompanyMeta)
	mux.HandleFunc("/api/companies/meta/update", handleCompanyMetaUpdate)
	mux.HandleFunc("/api/companies/parse-snippet", handleParseSnippet)
	mux.HandleFunc("/api/companies/vet", handleCompanyVet)
	mux.HandleFunc("/api/providers/status", handleProvidersStatus)
	mux.HandleFunc("/api/email/validate-domain", handleEmailValidateDomain)
	mux.HandleFunc("/api/email/mx-cache", handleEmailMXCache)
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



// ── Page handlers ─────────────────────────────────────────────────────────────

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	serveUIFile(w, "index.html")
}

func handleJobDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/job/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Validate job exists before serving the shell
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
	serveUIFile(w, "job_detail.html")
}

func handleResumes(w http.ResponseWriter, r *http.Request) {
	serveUIFile(w, "resumes.html")
}

func handleVettingPage(w http.ResponseWriter, r *http.Request) {
	serveUIFile(w, "vetting.html")
}

// fetchVettingLLMMetaForCompanies loads llm_* fields from company_meta for vetting UI badges.
func fetchVettingLLMMetaForCompanies(names []string) (map[string]map[string]interface{}, error) {
	out := make(map[string]map[string]interface{}, len(names))
	if len(names) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	query := `SELECT company_name,
		glassdoor_url, glassdoor_rating, glassdoor_review_count,
		linkedin_url, linkedin_employee_count, linkedin_founded,
		bbb_url, bbb_rating,
		COALESCE(indeed_url,''), COALESCE(indeed_rating,0), COALESCE(indeed_review_count,0),
		COALESCE(company_url,''),
		llm_risk_level, llm_assessment, llm_signals, llm_provider, llm_model, llm_assessed_at
		FROM company_meta WHERE company_name IN (` + placeholders + `)`
	args := make([]interface{}, len(names))
	for i, n := range names {
		args[i] = n
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var company string
		var gdURL, liURL, liEmp, liFounded, bbbURL, bbbRating, indeedURL, companyURL string
		var gdRating, indeedRating sql.NullFloat64
		var gdReviews, indeedReviews sql.NullInt64
		var risk, assess, sigs, prov, mod, assessed sql.NullString
		if err := rows.Scan(
			&company,
			&gdURL, &gdRating, &gdReviews,
			&liURL, &liEmp, &liFounded,
			&bbbURL, &bbbRating,
			&indeedURL, &indeedRating, &indeedReviews,
			&companyURL,
			&risk, &assess, &sigs, &prov, &mod, &assessed,
		); err != nil {
			return nil, err
		}
		var signals []interface{}
		if sigs.Valid && strings.TrimSpace(sigs.String) != "" {
			_ = json.Unmarshal([]byte(sigs.String), &signals)
		}
		if signals == nil {
			signals = []interface{}{}
		}
		meta := map[string]interface{}{
			"glassdoor_url":           gdURL,
			"glassdoor_rating":        nullFloat64Val(gdRating),
			"glassdoor_review_count":  nullInt64Val(gdReviews),
			"linkedin_url":            liURL,
			"linkedin_employee_count": liEmp,
			"linkedin_founded":        liFounded,
			"bbb_url":                 bbbURL,
			"bbb_rating":              bbbRating,
			"indeed_url":              indeedURL,
			"indeed_rating":           nullFloat64Val(indeedRating),
			"indeed_review_count":     nullInt64Val(indeedReviews),
			"company_url":             companyURL,
			"llm_risk_level":          nullStrPtr(risk),
			"llm_assessment":          nullStrPtr(assess),
			"llm_signals":             signals,
			"llm_provider":            nullStrPtr(prov),
			"llm_model":               nullStrPtr(mod),
			"llm_assessed_at":         nullStrPtr(assessed),
		}
		out[company] = meta
	}
	return out, rows.Err()
}

// nullFloat64Val returns nil if the NullFloat64 is not valid or zero.
func nullFloat64Val(n sql.NullFloat64) interface{} {
	if !n.Valid || n.Float64 == 0 {
		return nil
	}
	return n.Float64
}

// nullInt64Val returns nil if the NullInt64 is not valid or zero.
func nullInt64Val(n sql.NullInt64) interface{} {
	if !n.Valid || n.Int64 == 0 {
		return nil
	}
	return int(n.Int64)
}

func nullStrPtr(ns sql.NullString) interface{} {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

// handleVettingAPI serves GET /api/vetting — returns all jobs grouped by
// company and by recruiter for the vetting page.
func handleVettingAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	type VettingJob struct {
		ID             int64  `json:"id"`
		Title          string `json:"title"`
		URL            string `json:"url"`
		ScrapedAt      string `json:"scraped_at"`
		Status         string `json:"status"`
		RecruiterName  string `json:"recruiter_name"`
		RecruiterEmail string `json:"recruiter_email"`
		RecruiterPhone string `json:"recruiter_phone"`
	}
	type CompanyGroup struct {
		Company string                 `json:"company"`
		Jobs    []VettingJob           `json:"jobs"`
		Meta    map[string]interface{} `json:"meta"`
	}
	type RecruiterJob struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Company   string `json:"company"`
		Status    string `json:"status"`
		ScrapedAt string `json:"scraped_at"`
	}
	type RecruiterGroup struct {
		Name      string         `json:"name"`
		Email     string         `json:"email"`
		Phone     string         `json:"phone"`
		Companies []string       `json:"companies"`
		Jobs      []RecruiterJob `json:"jobs"`
	}

	rows, err := db.Query(`
		SELECT j.id, j.title, j.company, j.url, j.scraped_at,
		       COALESCE(a.status,'not_applied'),
		       COALESCE(a.recruiter_name,''),
		       COALESCE(a.recruiter_email,''),
		       COALESCE(a.recruiter_phone,'')
		FROM jobs j
		LEFT JOIN applications a ON a.job_id = j.id
		ORDER BY j.company COLLATE NOCASE, j.scraped_at DESC`)
	if err != nil {
		log.Printf("✗ handleVettingAPI query error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	companyMap   := map[string]*CompanyGroup{}
	companyOrder := []string{}
	recruiterMap := map[string]*RecruiterGroup{}

	for rows.Next() {
		var id int64
		var title, company, url, scrapedAt, status string
		var rName, rEmail, rPhone string
		if err := rows.Scan(&id, &title, &company, &url, &scrapedAt, &status, &rName, &rEmail, &rPhone); err != nil {
			continue
		}
		if company == "" { company = "Unknown Company" }
		if title   == "" { title   = "Untitled" }
		scrapedAt = normTS(scrapedAt)

		// Group by company
		if _, ok := companyMap[company]; !ok {
			companyMap[company] = &CompanyGroup{Company: company}
			companyOrder = append(companyOrder, company)
		}
		companyMap[company].Jobs = append(companyMap[company].Jobs, VettingJob{
			ID: id, Title: title, URL: url, ScrapedAt: scrapedAt,
			Status: status, RecruiterName: rName, RecruiterEmail: rEmail, RecruiterPhone: rPhone,
		})

		// Group by recruiter
		key := rEmail
		if key == "" { key = rName }
		if key != "" {
			if _, ok := recruiterMap[key]; !ok {
				recruiterMap[key] = &RecruiterGroup{
					Name: rName, Email: rEmail, Phone: rPhone,
					Companies: []string{},
				}
			}
			r := recruiterMap[key]
			// Add company if not already present
			found := false
			for _, c := range r.Companies {
				if c == company { found = true; break }
			}
			if !found { r.Companies = append(r.Companies, company) }
			r.Jobs = append(r.Jobs, RecruiterJob{
				ID: id, Title: title, Company: company, Status: status, ScrapedAt: scrapedAt,
			})
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("✗ handleVettingAPI rows error: %v", err)
	}

	llmMetaByCompany, err := fetchVettingLLMMetaForCompanies(companyOrder)
	if err != nil {
		log.Printf("✗ handleVettingAPI company_meta llm batch: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Build ordered slices
	companies := make([]CompanyGroup, 0, len(companyOrder))
	for _, name := range companyOrder {
		cg := *companyMap[name]
		if m, ok := llmMetaByCompany[name]; ok {
			cg.Meta = m
		} else {
			cg.Meta = map[string]interface{}{}
		}
		companies = append(companies, cg)
	}

	recruiters := make([]RecruiterGroup, 0, len(recruiterMap))
	for _, r := range recruiterMap {
		recruiters = append(recruiters, *r)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"companies":  companies,
		"recruiters": recruiters,
	})
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
	validProviders := map[string]bool{"": true, "anthropic": true, "ollama": true, "openai": true, "gemini": true, "manual": true}
	if !validProviders[provider] {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("invalid provider %q — must be one of: anthropic, openai, gemini, ollama, manual", provider))
		return
	}

	addedDays := 0
	if raw := q.Get("added_days"); raw != "" {
		if d, err := strconv.Atoi(raw); err == nil && d > 0 {
			addedDays = d
		}
	}

	// date_from / date_to: accept YYYY-MM-DD only to avoid SQL injection
	dateFrom := q.Get("date_from")
	dateTo   := q.Get("date_to")
	validDate := func(s string) bool {
		if len(s) != 10 { return false }
		for i, c := range s {
			if i == 4 || i == 7 {
				if c != '-' { return false }
			} else if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	if dateFrom != "" && !validDate(dateFrom) { dateFrom = "" }
	if dateTo   != "" && !validDate(dateTo)   { dateTo = "" }

	f := JobFilters{
		Search:    strings.TrimSpace(q.Get("search")),
		Status:    status,
		Score:     score,
		Provider:  provider,
		Page:      page,
		PerPage:   perPage,
		AddedDays: addedDays,
		DateFrom:  dateFrom,
		DateTo:    dateTo,
	}

	log.Printf("→ /api/jobs/list page=%d per_page=%d search=%q status=%q score=%q provider=%q added_days=%d date_from=%q date_to=%q",
		f.Page, f.PerPage, f.Search, f.Status, f.Score, f.Provider, f.AddedDays, f.DateFrom, f.DateTo)

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

// handleScrapeJobPreview scrapes a URL and returns preview data without saving to DB.
func handleScrapeJobPreview(w http.ResponseWriter, r *http.Request) {
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

	// Check duplicate before scraping
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

	log.Printf("→ Scraping preview URL: %s", jobURL)
	title, company, location, description, err := ScrapeJob(jobURL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	log.Printf("✓ Scraped preview: %q (%s)", title, company)

	tq := assessJobTextQuality(description)
	blockers := []string{}
	descLower := strings.ToLower(description)
	for _, kw := range blockerKeywords {
		if strings.Contains(descLower, strings.ToLower(kw)) {
			blockers = append(blockers, kw)
		}
	}
	hasWarnings := tq.Level != "ok" || len(blockers) > 0

	writeJSON(w, http.StatusOK, ScrapePreviewResponse{
		URL:             jobURL,
		Title:           title,
		Company:         company,
		Location:        location,
		Description:     description,
		BlockerKeywords: blockers,
		TextQuality:     tq,
		HasWarnings:     hasWarnings,
	})
}

// handleJobPreview serves the job preview page (shell — data is loaded from sessionStorage by JS).
func handleJobPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	serveUIFile(w, "job_preview.html")
}

// handleSavePreview validates and saves a previewed job to the DB.
func handleSavePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	jobURL := strings.TrimSpace(r.FormValue("url"))
	title := strings.TrimSpace(r.FormValue("title"))
	company := strings.TrimSpace(r.FormValue("company"))
	location := strings.TrimSpace(r.FormValue("location"))
	description := cleanText(r.FormValue("description"))

	if jobURL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	if len(description) < 50 {
		writeError(w, http.StatusBadRequest, "description too short (min 50 chars)")
		return
	}
	// Truncate to 80 000 chars
	if len(description) > 80000 {
		description = description[:80000]
	}

	// Re-check duplicate
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

	id, err := dbInsertJob(jobURL, title, company, location, description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbInsertJob error: %v", err))
		return
	}
	log.Printf("✓ Preview job saved id=%d: %q", id, title)
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
	location := strings.TrimSpace(r.FormValue("location"))
	sourceURL := strings.TrimSpace(r.FormValue("source_url"))
	companyURL := strings.TrimSpace(r.FormValue("company_url"))
	// Silently ignore invalid company_url
	if companyURL != "" && !strings.HasPrefix(companyURL, "http://") && !strings.HasPrefix(companyURL, "https://") {
		companyURL = ""
	}
	description := cleanText(r.FormValue("description"))

	log.Printf("→ Manual job: title=%q company=%q location=%q desc_len=%d", title, company, location, len(description))

	if len(description) < 50 {
		writeError(w, http.StatusUnprocessableEntity, "Description is too short (minimum 50 characters).")
		return
	}
	if title == "" {
		title = "Untitled Job"
	}

	// Use provided source URL if given, otherwise generate synthetic manual:// URL
	slug := fmt.Sprintf("%x", md5.Sum([]byte(description[:min(200, len(description))])))
	syntheticURL := "manual://" + slug[:12]
	jobURL := syntheticURL
	if sourceURL != "" {
		jobURL = sourceURL
	}

	existing, err := dbGetJobByURL(jobURL)
	if err != nil {
		log.Printf("✗ dbGetJobByURL (manual) error: %v", err)
	}
	// Also check synthetic URL when source URL provided to catch content duplicates
	if existing == nil && sourceURL != "" {
		existing, err = dbGetJobByURL(syntheticURL)
		if err != nil {
			log.Printf("✗ dbGetJobByURL (synthetic) error: %v", err)
		}
	}
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "This job has already been added.", "job_id": existing.ID,
		})
		return
	}
	if len(description) > 8000 {
		description = description[:8000] + "\n\n[...truncated for analysis]"
	}

	id, err := dbInsertJob(jobURL, title, company, location, description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbInsertJob (manual) error: %v", err))
		return
	}
	// Save and sync company_url
	if companyURL != "" {
		if err := dbUpdateJobCompanyURL(id, companyURL); err != nil {
			log.Printf("✗ dbUpdateJobCompanyURL(%d) error: %v", id, err)
		}
		if company != "" {
			if err := dbSyncCompanyURLToMeta(company, companyURL); err != nil {
				log.Printf("✗ dbSyncCompanyURLToMeta(%q) error: %v", company, err)
			}
		}
	}
	log.Printf("✓ Manual job saved id=%d: %q", id, title)
	writeJSON(w, http.StatusOK, map[string]interface{}{"job_id": id, "title": title, "company": company})
}

func handleJobActions(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if r.Method == http.MethodDelete && strings.HasSuffix(path, "/salary-estimate") {
		handleClearSalaryEstimate(w, r)
		return
	}

	// Email endpoints — must come before the generic DELETE handler below
	if strings.HasSuffix(path, "/email") {
		switch r.Method {
		case http.MethodGet:
			handleGetJobEmail(w, r)
		case http.MethodPost:
			handlePostJobEmail(w, r)
		case http.MethodDelete:
			handleDeleteJobEmail(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "GET, POST or DELETE required")
		}
		return
	}

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

	if r.Method == http.MethodGet && strings.HasSuffix(path, "/detail") {
		handleJobDetail_API(w, r)
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

	if r.Method == http.MethodPost && strings.HasSuffix(path, "/estimate-salary") {
		handleEstimateSalary(w, r)
		return
	}

	if r.Method == http.MethodPost && strings.HasSuffix(path, "/application") {
		handleUpsertApplication(w, r)
		return
	}

	if r.Method == http.MethodPatch && strings.HasSuffix(path, "/url") {
		handleUpdateJobURL(w, r)
		return
	}

	if r.Method == http.MethodPatch && strings.HasSuffix(path, "/title") {
		handleUpdateJobTitle(w, r)
		return
	}

	if r.Method == http.MethodPatch && strings.HasSuffix(path, "/company") {
		handleUpdateJobCompany(w, r)
		return
	}

	if r.Method == http.MethodPatch && strings.HasSuffix(path, "/location") {
		handleUpdateJobLocation(w, r)
		return
	}
	if r.Method == http.MethodPatch && strings.HasSuffix(path, "/company-url") {
		handleUpdateJobCompanyURL(w, r)
		return
	}

	log.Printf("✗ Unhandled job action: %s %s", r.Method, path)
	http.NotFound(w, r)
}

// handleGetJobEmail serves GET /api/jobs/{id}/email
func handleGetJobEmail(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/jobs/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	email, err := dbGetJobEmail(id)
	if err != nil {
		log.Printf("✗ dbGetJobEmail(%d) error: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to load email")
		return
	}
	if email == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"email": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"email": email})
}

// handlePostJobEmail serves POST /api/jobs/{id}/email
func handlePostJobEmail(w http.ResponseWriter, r *http.Request) {
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/jobs/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rawHTML := strings.TrimSpace(r.FormValue("raw_html"))
	if rawHTML == "" {
		writeError(w, http.StatusUnprocessableEntity, "raw_html is required")
		return
	}
	if err := dbSaveJobEmail(id, rawHTML); err != nil {
		log.Printf("✗ dbSaveJobEmail(%d) error: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to save email.")
		return
	}
	log.Printf("✓ Email saved for job %d", id)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handleDeleteJobEmail serves DELETE /api/jobs/{id}/email
func handleDeleteJobEmail(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/jobs/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := dbDeleteJobEmail(id); err != nil {
		log.Printf("✗ dbDeleteJobEmail(%d) error: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to delete email.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handleUpdateJobURL serves PATCH /api/jobs/{id}/url — updates or clears the
// source URL of a saved job. Clearing restores a synthetic manual:// URL.
func handleUpdateJobURL(w http.ResponseWriter, r *http.Request) {
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/jobs/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	newURL := strings.TrimSpace(r.FormValue("url"))

	// Validate scheme if URL provided
	if newURL != "" && !strings.HasPrefix(newURL, "http://") && !strings.HasPrefix(newURL, "https://") {
		writeError(w, http.StatusUnprocessableEntity, "URL must start with http:// or https://")
		return
	}

	// Check job exists
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

	// If clearing, regenerate synthetic manual:// URL from description
	if newURL == "" {
		slug := fmt.Sprintf("%x", md5.Sum([]byte(job.RawDescription[:min(200, len(job.RawDescription))])))
		newURL = "manual://" + slug[:12]
	}

	if err := dbUpdateJobURL(id, newURL); err != nil {
		log.Printf("✗ dbUpdateJobURL(%d) error: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to update URL.")
		return
	}
	log.Printf("✓ Job %d URL updated to: %s", id, newURL)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "url": newURL})
}

// handleUpdateJobTitle serves PATCH /api/jobs/{id}/title
func handleUpdateJobTitle(w http.ResponseWriter, r *http.Request) {
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/jobs/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		writeError(w, http.StatusUnprocessableEntity, "Title cannot be empty.")
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
	if err := dbUpdateJobField(id, "title", title); err != nil {
		log.Printf("✗ dbUpdateJobField title(%d) error: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to update title.")
		return
	}
	log.Printf("✓ Job %d title updated to: %s", id, title)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "title": title})
}

// handleUpdateJobCompany serves PATCH /api/jobs/{id}/company
func handleUpdateJobCompany(w http.ResponseWriter, r *http.Request) {
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/jobs/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	company := strings.TrimSpace(r.FormValue("company"))
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
	oldCompany := job.Company
	if err := dbUpdateJobField(id, "company", company); err != nil {
		log.Printf("✗ dbUpdateJobField company(%d) error: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to update company.")
		return
	}
	// Rename company_meta row so vetting/crawl data follows the new name
	if oldCompany != company {
		if err := dbRenameCompanyMeta(oldCompany, company); err != nil {
			log.Printf("✗ dbRenameCompanyMeta(%q→%q): %v", oldCompany, company, err)
			// Non-fatal — job name updated, meta rename failed
		} else if oldCompany != "" {
			log.Printf("✓ company_meta renamed: %q → %q", oldCompany, company)
		}
	}
	log.Printf("✓ Job %d company updated to: %q", id, company)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "company": company})
}

// handleUpdateJobLocation serves PATCH /api/jobs/{id}/location
func handleUpdateJobLocation(w http.ResponseWriter, r *http.Request) {
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/jobs/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	location := strings.TrimSpace(r.FormValue("location"))
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
	if err := dbUpdateJobField(id, "location", location); err != nil {
		log.Printf("✗ dbUpdateJobField location(%d) error: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to update location.")
		return
	}
	log.Printf("✓ Job %d location updated to: %q", id, location)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "location": location})
}

// handleUpdateJobCompanyURL serves PATCH /api/jobs/{id}/company-url
func handleUpdateJobCompanyURL(w http.ResponseWriter, r *http.Request) {
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/jobs/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	companyURL := strings.TrimSpace(r.FormValue("company_url"))
	if companyURL != "" && !strings.HasPrefix(companyURL, "http://") && !strings.HasPrefix(companyURL, "https://") {
		writeError(w, http.StatusUnprocessableEntity, "company_url must start with http:// or https://")
		return
	}
	job, err := dbGetJobByID(id)
	if err != nil || job == nil {
		http.NotFound(w, r)
		return
	}
	if err := dbUpdateJobCompanyURL(id, companyURL); err != nil {
		log.Printf("✗ dbUpdateJobCompanyURL(%d) error: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to update company URL.")
		return
	}
	// Sync to company_meta
	if job.Company != "" && companyURL != "" {
		if err := dbSyncCompanyURLToMeta(job.Company, companyURL); err != nil {
			log.Printf("✗ dbSyncCompanyURLToMeta(%q) error: %v", job.Company, err)
		} else {
			log.Printf("✓ company_meta company_url synced for %q", job.Company)
		}
	}
	log.Printf("✓ Job %d company_url updated to: %q", id, companyURL)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "company_url": companyURL})
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

	// Apply per-request overrides: mode and model from the UI dropdowns.
	cfg := appCfg
	if mode := r.FormValue("analysis_mode"); mode != "" {
		switch mode {
		case "fast", "standard", "detailed":
			cfg.AnalysisMode = mode
		}
	}
	if provider == "ollama" {
		if m := r.FormValue("ollama_model"); m != "" {
			cfg.OllamaModel = m
		}
	} else if m := r.FormValue("cloud_model"); m != "" {
		switch provider {
		case "anthropic":
			cfg.AnthropicModel = m
		case "openai":
			cfg.OpenAIModel = m
		case "gemini":
			cfg.GeminiModel = m
		}
	}

	job, err := dbGetJobByID(jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load job: %v", err))
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("job id=%d not found", jobID))
		return
	}

	resume, err := dbGetResumeByID(resumeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load resume: %v", err))
		return
	}
	if resume == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("resume id=%d not found", resumeID))
		return
	}

	log.Printf("→ Analyzing job=%d resume=%d provider=%s mode=%s", jobID, resumeID, provider, cfg.AnalysisMode)
	startTime := time.Now()
	analysis, err := AnalyzeMatch(resume.Content, job.RawDescription, provider, cfg)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("analysis failed: %v", err))
		return
	}
	analysis.DurationSeconds = int(time.Since(startTime).Seconds())

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

// ── Salary API ────────────────────────────────────────────────────────────────

func handleEstimateSalary(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	jobID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}

	cfg := appCfg
	provider := strings.TrimSpace(r.FormValue("provider"))
	if provider == "" {
		provider = "anthropic"
	}
	model := strings.TrimSpace(r.FormValue("model"))
	if model != "" {
		cfg.OllamaModel = model
	}

	// Guard incompatible models — thinking models now supported via format:json
	if provider == "ollama" && !isThinkingModel(cfg.OllamaModel) {
		for _, blocked := range salaryIncompatibleModels {
			if strings.Contains(strings.ToLower(cfg.OllamaModel), blocked) {
				writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf(
					"%s is not supported for salary estimation. Please switch to Anthropic or llama3.1:8b.", cfg.OllamaModel))
				return
			}
		}
	}

	job, err := dbGetJobByID(jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load job: %v", err))
		return
	}
	if job == nil {
		http.NotFound(w, r)
		return
	}

	// Return cached estimate if available
	cached, err := dbGetJobSalaryEstimate(jobID)
	if err != nil {
		log.Printf("✗ dbGetJobSalaryEstimate(%d) error: %v", jobID, err)
	}
	if cached != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, cached)
		return
	}

	// Extract if JD has salary, otherwise estimate
	var result map[string]interface{}
	hasSalary := jobHasSalary(job.RawDescription)
	if hasSalary {
		log.Printf("→ JD has salary — extracting for job=%d", jobID)
		result, err = extractSalary(job.Title, job.Company, job.Location, job.RawDescription, provider, cfg)
		if err != nil {
			// Fallback to estimate when extract fails (#8b)
			log.Printf("→ extract failed for job=%d, falling back to estimate: %v", jobID, err)
			result, err = estimateSalary(job.Title, job.Company, job.Location, job.RawDescription, provider, true, cfg)
		}
	} else {
		log.Printf("→ Estimating salary for job=%d provider=%s", jobID, provider)
		result, err = estimateSalary(job.Title, job.Company, job.Location, job.RawDescription, provider, false, cfg)
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Cache in DB
	cacheJSON, _ := json.Marshal(result)
	if cacheErr := dbSetJobSalaryEstimate(jobID, string(cacheJSON)); cacheErr != nil {
		log.Printf("✗ dbSetJobSalaryEstimate(%d) error: %v", jobID, cacheErr)
	}
	log.Printf("✓ Salary estimated job=%d min=%v max=%v conf=%v", jobID, result["min"], result["max"], result["confidence"])
	writeJSON(w, http.StatusOK, result)
}

func handleClearSalaryEstimate(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	jobID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := dbSetJobSalaryEstimate(jobID, ""); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dbSetJobSalaryEstimate(%d) error: %v", jobID, err))
		return
	}
	log.Printf("✓ Salary estimate cleared job=%d", jobID)
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
	content := cleanText(r.FormValue("content"))

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
	// GET /api/resumes/ — list all resumes (shared UI endpoint)
	if r.Method == http.MethodGet && r.URL.Path == "/api/resumes/" {
		handleResumesList(w, r)
		return
	}
	// GET /api/resumes/{id} — get single resume with full content
	if r.Method == http.MethodGet {
		handleGetResume(w, r)
		return
	}
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


// ── New v5 API endpoints ──────────────────────────────────────────────────────

// handleJobDetail_API serves GET /api/jobs/{id}/detail — returns all job
// detail page data as JSON so the shared static UI can render client-side.
func handleJobDetail_API(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/jobs/")
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
	if analyses == nil {
		analyses = []Analysis{}
	}
	resumes, err := dbGetResumes()
	if err != nil {
		log.Printf("✗ dbGetResumes error (job detail api): %v", err)
	}
	if resumes == nil {
		resumes = []Resume{}
	}
	var salaryEstimate *SalaryEstimate
	if rawSalary, err := dbGetJobSalaryEstimate(id); err != nil {
		log.Printf("✗ dbGetJobSalaryEstimate(%d) error: %v", id, err)
	} else if rawSalary != "" {
		var se SalaryEstimate
		if err := json.Unmarshal([]byte(rawSalary), &se); err == nil {
			salaryEstimate = &se
		}
	}

	// Derive last-used resume, provider, model, and mode from the most recent analysis.
	var lastResumeID    int64
	lastProvider       := "anthropic"
	lastAnalysisMode   := appCfg.AnalysisMode
	lastOllamaModel    := appCfg.OllamaModel
	lastAnthropicModel := appCfg.AnthropicModel
	lastOpenAIModel    := appCfg.OpenAIModel
	lastGeminiModel    := appCfg.GeminiModel
	if len(analyses) > 0 {
		a := analyses[0]
		lastResumeID = a.ResumeID
		if a.LLMProvider != "" {
			lastProvider = a.LLMProvider
		}
		if a.AnalysisMode != "" {
			lastAnalysisMode = a.AnalysisMode
		}
		if a.LLMModel != "" {
			switch lastProvider {
			case "ollama":
				lastOllamaModel = a.LLMModel
			case "anthropic":
				lastAnthropicModel = a.LLMModel
			case "openai":
				lastOpenAIModel = a.LLMModel
			case "gemini":
				lastGeminiModel = a.LLMModel
			}
		}
	} else if salaryEstimate != nil && salaryEstimate.LLMProvider != "" {
		lastProvider = salaryEstimate.LLMProvider
	}

	comp := buildComparison(analyses)
	var compJSON *ResumeComparisonJSON
	if comp != nil {
		compJSON = &ResumeComparisonJSON{
			ResumeA:      comp.ResumeA,
			ResumeB:      comp.ResumeB,
			BetterFit:    comp.BetterFit,
			BetterReason: comp.BetterReason,
		}
	}

	// company_url: single source of truth is company_meta.
	// Sync job → meta if meta is empty; always display from meta.
	if job.Company != "" {
		meta, _ := dbGetCompanyMeta(job.Company)
		jobURL := strings.TrimSpace(job.CompanyURL)
		metaURL := ""
		if meta != nil {
			metaURL = strings.TrimSpace(meta.CompanyURL)
		}
		if jobURL != "" && metaURL == "" {
			if err := dbSyncCompanyURLToMeta(job.Company, jobURL); err != nil {
				log.Printf("✗ dbSyncCompanyURLToMeta(%q) error: %v", job.Company, err)
			} else {
				metaURL = jobURL
			}
		}
		job.CompanyURL = metaURL
	}

	writeJSON(w, http.StatusOK, JobDetailAPIResponse{
		Job:            *job,
		Application:    *app,
		Analyses:       analyses,
		Resumes:        resumes,
		SalaryEstimate: salaryEstimate,
		TextQuality:    assessJobTextQuality(job.RawDescription),
		Comparison:     compJSON,
		LastResumeID:   lastResumeID,
		LastProvider:   lastProvider,
		AnalysisMode:   lastAnalysisMode,
		OllamaModel:    lastOllamaModel,
		AnthropicModel: lastAnthropicModel,
		OpenAIModel:    lastOpenAIModel,
		GeminiModel:    lastGeminiModel,
		HasSalaryInJD:  jobHasSalary(job.RawDescription),
	})
}

// handleProvidersStatus serves GET /api/providers/status — returns which
// LLM providers are configured/reachable and their default models.
func handleProvidersStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	// Derive default provider — first configured key wins, fallback to ollama
	defaultProvider := "ollama"
	switch {
	case appCfg.AnthropicAPIKey != "":
		defaultProvider = "anthropic"
	case appCfg.OpenAIAPIKey != "":
		defaultProvider = "openai"
	case appCfg.GeminiAPIKey != "":
		defaultProvider = "gemini"
	}
	writeJSON(w, http.StatusOK, ProvidersStatusResponse{
		HasAnthropic:    appCfg.AnthropicAPIKey != "",
		HasOpenAI:       appCfg.OpenAIAPIKey != "",
		HasGemini:       appCfg.GeminiAPIKey != "",
		HasOllama:       ollamaAvailable(),
		DefaultProvider: defaultProvider,
		DefaultModels: map[string]string{
			"anthropic": appCfg.AnthropicModel,
			"openai":    appCfg.OpenAIModel,
			"gemini":    appCfg.GeminiModel,
			"ollama":    appCfg.OllamaModel,
		},
		MXAutoCheck: appCfg.MXAutoCheck,
	})
}

func handleEmailValidateDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		writeError(w, http.StatusUnprocessableEntity, "email is required")
		return
	}
	res := validateEmailDomain(db, email)
	writeJSON(w, http.StatusOK, res)
}

func handleEmailMXCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	m, err := dbGetMXCacheMap()
	if err != nil {
		log.Printf("✗ dbGetMXCacheMap: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if m == nil {
		m = map[string]map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, m)
}

// handleGetResume serves GET /api/resumes/{id} — returns full resume content.
func handleGetResume(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/resumes/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r2, err := dbGetResumeByID(id)
	if err != nil {
		log.Printf("✗ dbGetResumeByID(%d) error: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to load resume")
		return
	}
	if r2 == nil {
		writeError(w, http.StatusNotFound, "Resume not found.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":         r2.ID,
		"label":      r2.Label,
		"content":    r2.Content,
		"created_at": r2.CreatedAt.Format("2006-01-02 15:04:05"),
		"char_count": len(r2.Content),
	})
}

// handleResumeExtract serves POST /api/resumes/extract — extracts plain text
// from an uploaded TXT, PDF, or DOCX file.
func handleResumeExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	name := strings.ToLower(header.Filename)
	var text string

	switch {
	case strings.HasSuffix(name, ".txt"):
		text = string(raw)
	case strings.HasSuffix(name, ".docx"):
		text, err = extractDocxText(raw)
		if err != nil {
			log.Printf("✗ extractDocxText error: %v", err)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to extract DOCX text: %v", err))
			return
		}
	case strings.HasSuffix(name, ".pdf"):
		text, err = extractPDFText(raw)
		if err != nil {
			log.Printf("✗ extractPDFText error: %v", err)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to extract PDF text: %v", err))
			return
		}
	default:
		writeError(w, http.StatusUnprocessableEntity, "Unsupported file type. Please upload a TXT, PDF, or DOCX file.")
		return
	}

	text = strings.TrimSpace(text)
	if len(text) < 50 {
		writeError(w, http.StatusUnprocessableEntity, "Could not extract enough text from the file (minimum 50 characters).")
		return
	}
	log.Printf("✓ Resume extracted from %s: %d chars", header.Filename, len(text))
	writeJSON(w, http.StatusOK, map[string]interface{}{"text": text, "char_count": len(text)})
}

// handleResumesList serves GET /api/resumes/ — returns all saved resumes as JSON.
func handleResumesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	resumes, err := dbGetResumes()
	if err != nil {
		log.Printf("✗ dbGetResumes error (api): %v", err)
		http.Error(w, "failed to load resumes", http.StatusInternalServerError)
		return
	}
	if resumes == nil {
		resumes = []Resume{}
	}
	log.Printf("→ /api/resumes/ returned %d resumes", len(resumes))
	writeJSON(w, http.StatusOK, map[string]interface{}{"resumes": resumes})
}

// ── Template view helpers ─────────────────────────────────────────────────────

// SkillGroup holds matched skills grouped under a single category label.
type SkillGroup struct {
	Category string
	Skills   []MatchedSkill
}

// ClusterPenaltyLine is one row in the score breakdown for a skill cluster.
type ClusterPenaltyLine struct {
	Name    string
	Skills  []string // skill names in this cluster
	Penalty int
	Capped  bool // true when raw penalty was reduced by the cluster cap
}

var skillGroupOrder = []string{
	"security", "backend", "frontend", "cloud", "devops", "database", "ai", "other",
}

// groupMatchedSkills returns matched skills grouped by category in a stable order.
func groupMatchedSkills(skills []MatchedSkill) []SkillGroup {
	groups := map[string][]MatchedSkill{}
	for _, s := range skills {
		cat := s.Category
		if cat == "" {
			cat = "other"
		}
		groups[cat] = append(groups[cat], s)
	}
	seen := map[string]bool{}
	var result []SkillGroup
	for _, cat := range skillGroupOrder {
		if g, ok := groups[cat]; ok {
			result = append(result, SkillGroup{Category: cat, Skills: g})
			seen[cat] = true
		}
	}
	for cat, g := range groups {
		if !seen[cat] {
			result = append(result, SkillGroup{Category: cat, Skills: g})
		}
	}
	return result
}

// buildClusterLines builds ordered penalty rows for the score breakdown,
// grouping missing skills by cluster and showing whether the cap was applied.
func buildClusterLines(missing []MissingSkill, clusters map[string]int) []ClusterPenaltyLine {
	skillsByCluster := map[string][]string{}
	rawByCluster := map[string]int{}
	for _, s := range missing {
		p := penaltyForSkill(s)
		if p > 0 {
			skillsByCluster[s.ClusterGroup] = append(skillsByCluster[s.ClusterGroup], s.Skill)
			rawByCluster[s.ClusterGroup] += p
		}
	}
	seen := map[string]bool{}
	var lines []ClusterPenaltyLine
	for _, name := range skillGroupOrder {
		if pen, ok := clusters[name]; ok {
			lines = append(lines, ClusterPenaltyLine{
				Name:    name,
				Skills:  skillsByCluster[name],
				Penalty: pen,
				Capped:  rawByCluster[name] > pen,
			})
			seen[name] = true
		}
	}
	for name, pen := range clusters {
		if !seen[name] {
			lines = append(lines, ClusterPenaltyLine{
				Name:    name,
				Skills:  skillsByCluster[name],
				Penalty: pen,
				Capped:  rawByCluster[name] > pen,
			})
		}
	}
	return lines
}

// ── Ollama proxy ──────────────────────────────────────────────────────────────

// handleOllamaModels proxies GET /api/ollama/models to the configured Ollama
// server so the browser avoids CORS issues hitting localhost:11434 directly.
func handleOllamaModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	ollamaURL := appCfg.OllamaBaseURL
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	resp, err := http.Get(ollamaURL + "/api/tags")
	if err != nil {
		log.Printf("→ Ollama /api/tags unreachable: %v", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{"models": []string{}})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("→ Ollama /api/tags error: status=%d err=%v", resp.StatusCode, err)
		writeJSON(w, http.StatusOK, map[string]interface{}{"models": []string{}})
		return
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("→ Ollama /api/tags parse error: %v", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{"models": []string{}})
		return
	}
	names := make([]string, 0, len(payload.Models))
	for _, m := range payload.Models {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	log.Printf("→ Ollama models proxy: returned %d models", len(names))
	writeJSON(w, http.StatusOK, map[string]interface{}{"models": names})
}

// handleProviderModels serves GET /api/providers/models?provider=anthropic|openai|gemini
// Returns the static list of known models for the requested cloud provider.
func handleProviderModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	provider := r.URL.Query().Get("provider")
	models, ok := knownModels[provider]
	if !ok {
		models = []KnownModel{}
	}
	log.Printf("→ Provider models: provider=%s count=%d", provider, len(models))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider": provider,
		"models":   models,
	})
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

// ── Company crawl ─────────────────────────────────────────────────────────────

// handleCompanyCrawl serves POST /api/companies/crawl
func handleCompanyCrawl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	companyName := strings.TrimSpace(r.FormValue("company_name"))
	if companyName == "" {
		writeError(w, http.StatusUnprocessableEntity, "company_name is required.")
		return
	}

	// Return cached result if fresh (within 7 days)
	cached, err := dbGetCompanyMeta(companyName)
	if err != nil {
		log.Printf("✗ dbGetCompanyMeta(%q): %v", companyName, err)
	}
	if cached != nil && cached.CrawledAt != "" {
		t, err := time.Parse("2006-01-02 15:04:05", cached.CrawledAt)
		if err == nil && time.Since(t) < 7*24*time.Hour {
			log.Printf("✓ Returning cached company_meta for: %q", companyName)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":                      true,
				"cached":                  true,
				"company_name":            cached.CompanyName,
				"glassdoor_url":           cached.GlassdoorURL,
				"glassdoor_rating":        cached.GlassdoorRating,
				"glassdoor_review_count":  cached.GlassdoorReviewCount,
				"linkedin_url":            cached.LinkedInURL,
				"linkedin_employee_count": cached.LinkedInEmployees,
				"linkedin_founded":        cached.LinkedInFounded,
				"bbb_url":                 cached.BBBURL,
				"bbb_rating":              cached.BBBRating,
				"crawled_at":              cached.CrawledAt,
			})
			return
		}
	}

	log.Printf("→ Crawling company: %q", companyName)
	result := CrawlCompany(companyName)

	if err := dbUpsertCompanyMeta(companyName, result); err != nil {
		log.Printf("✗ dbUpsertCompanyMeta(%q): %v", companyName, err)
	}

	fresh, _ := dbGetCompanyMeta(companyName)
	if fresh == nil {
		fresh = &CompanyMeta{CompanyName: companyName}
	}
	log.Printf("✓ Crawl complete for: %q", companyName)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                     true,
		"cached":                 false,
		"company_name":           fresh.CompanyName,
		"glassdoor_url":          fresh.GlassdoorURL,
		"glassdoor_rating":       fresh.GlassdoorRating,
		"glassdoor_review_count": fresh.GlassdoorReviewCount,
		"linkedin_url":           fresh.LinkedInURL,
		"linkedin_employee_count": fresh.LinkedInEmployees,
		"linkedin_founded":       fresh.LinkedInFounded,
		"bbb_url":                fresh.BBBURL,
		"bbb_rating":             fresh.BBBRating,
		"crawled_at":             fresh.CrawledAt,
	})
}

// handleCompanyMeta serves GET /api/companies/meta?company_name=...
func handleCompanyMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		handleCompanyMetaDelete(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	companyName := strings.TrimSpace(r.URL.Query().Get("company_name"))
	if companyName == "" {
		writeError(w, http.StatusUnprocessableEntity, "company_name is required.")
		return
	}
	meta, err := dbGetCompanyMeta(companyName)
	if err != nil {
		log.Printf("✗ dbGetCompanyMeta(%q): %v", companyName, err)
		writeError(w, http.StatusInternalServerError, "DB error")
		return
	}
	if meta == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "cached": false, "company_name": companyName})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                     true,
		"cached":                 true,
		"company_name":           meta.CompanyName,
		"glassdoor_url":          meta.GlassdoorURL,
		"glassdoor_rating":       meta.GlassdoorRating,
		"glassdoor_review_count": meta.GlassdoorReviewCount,
		"linkedin_url":           meta.LinkedInURL,
		"linkedin_employee_count": meta.LinkedInEmployees,
		"linkedin_founded":       meta.LinkedInFounded,
		"bbb_url":                meta.BBBURL,
		"bbb_rating":             meta.BBBRating,
		"crawled_at":             meta.CrawledAt,
	})
}

func vettingForceRescan(force string) bool {
	v := strings.TrimSpace(strings.ToLower(force))
	return v == "1" || v == "true" || v == "yes"
}

func parseLLMCacheTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04:05.000000000"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func companyMetaToVettingMeta(m *CompanyMeta) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"company_name":             m.CompanyName,
		"glassdoor_url":            m.GlassdoorURL,
		"glassdoor_rating":         m.GlassdoorRating,
		"glassdoor_review_count":   m.GlassdoorReviewCount,
		"linkedin_url":             m.LinkedInURL,
		"linkedin_employee_count":  m.LinkedInEmployees,
		"linkedin_founded":         m.LinkedInFounded,
		"bbb_url":                  m.BBBURL,
		"bbb_rating":               m.BBBRating,
	}
}

// handleParseSnippet serves POST /api/companies/parse-snippet.
// Accepts pasted Google search text and uses LLM to extract company ratings.
func handleParseSnippet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	companyName := strings.TrimSpace(r.FormValue("company_name"))
	text        := strings.TrimSpace(r.FormValue("text"))
	provider    := strings.TrimSpace(strings.ToLower(r.FormValue("provider")))
	model       := strings.TrimSpace(r.FormValue("model"))

	if companyName == "" {
		writeError(w, http.StatusUnprocessableEntity, "company_name is required.")
		return
	}
	if text == "" {
		writeError(w, http.StatusUnprocessableEntity, "text is required.")
		return
	}
	if len(text) > 5000 {
		writeError(w, http.StatusUnprocessableEntity, "text too long (max 5000 chars).")
		return
	}
	if provider == "" {
		provider = "anthropic"
	}

	cfg := appCfg
	result, err := parseCompanySnippet(text, provider, model, cfg)
	if err != nil {
		log.Printf("✗ parseCompanySnippet(%q): %v", companyName, err)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if !result.HasData() {
		writeJSON(w, http.StatusOK, ParseSnippetResponse{
			OK:      true,
			Company: companyName,
			Found:   false,
			Message: "No rating data could be extracted from the pasted text.",
		})
		return
	}

	fields := result.ToMap()
	if err := dbUpsertSnippetMeta(companyName, fields); err != nil {
		log.Printf("✗ dbUpsertSnippetMeta(%q): %v", companyName, err)
		writeError(w, http.StatusInternalServerError, "Failed to save extracted data.")
		return
	}

	meta, _ := dbGetCompanyMeta(companyName)
	log.Printf("✓ parse-snippet: company=%q fields=%d", companyName, len(fields))
	writeJSON(w, http.StatusOK, ParseSnippetResponse{
		OK:      true,
		Company: companyName,
		Found:   true,
		Data:    fields,
		Meta:    meta,
	})
}

// handleCompanyMetaDelete serves DELETE /api/companies/meta — removes all
// company_meta for a company (ratings, URLs, LLM vetting).
func handleCompanyMetaDelete(w http.ResponseWriter, r *http.Request) {
	companyName := strings.TrimSpace(r.URL.Query().Get("company_name"))
	if companyName == "" {
		writeError(w, http.StatusUnprocessableEntity, "company_name is required.")
		return
	}
	if err := dbDeleteCompanyMeta(companyName); err != nil {
		log.Printf("✗ dbDeleteCompanyMeta(%q): %v", companyName, err)
		writeError(w, http.StatusInternalServerError, "Failed to delete company meta.")
		return
	}
	log.Printf("✓ delete_company_meta: company=%q", companyName)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"company": companyName,
	})
}

// handleCompanyMetaUpdate serves POST /api/companies/meta/update.
// Manually saves ratings, review counts, and URLs for a company.
func handleCompanyMetaUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	companyName := strings.TrimSpace(r.FormValue("company_name"))
	if companyName == "" {
		writeError(w, http.StatusUnprocessableEntity, "company_name is required.")
		return
	}

	fields := map[string]interface{}{}
	updated := []string{}

	// Ratings
	if v := strings.TrimSpace(r.FormValue("glassdoor_rating")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 1.0 || f > 5.0 {
			writeError(w, http.StatusUnprocessableEntity, "glassdoor_rating must be between 1 and 5.")
			return
		}
		fields["glassdoor_rating"] = f
		updated = append(updated, "glassdoor_rating")
	}
	if v := strings.TrimSpace(r.FormValue("glassdoor_review_count")); v != "" {
		i, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "glassdoor_review_count must be an integer.")
			return
		}
		fields["glassdoor_review_count"] = i
		updated = append(updated, "glassdoor_review_count")
	}
	if v := strings.TrimSpace(r.FormValue("indeed_rating")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 1.0 || f > 5.0 {
			writeError(w, http.StatusUnprocessableEntity, "indeed_rating must be between 1 and 5.")
			return
		}
		fields["indeed_rating"] = f
		updated = append(updated, "indeed_rating")
	}
	if v := strings.TrimSpace(r.FormValue("indeed_review_count")); v != "" {
		i, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "indeed_review_count must be an integer.")
			return
		}
		fields["indeed_review_count"] = i
		updated = append(updated, "indeed_review_count")
	}
	if v := strings.TrimSpace(r.FormValue("bbb_rating")); v != "" {
		fields["bbb_rating"] = strings.ToUpper(v)
		updated = append(updated, "bbb_rating")
	}

	// URLs
	for _, field := range []string{"glassdoor_url", "indeed_url", "bbb_url", "linkedin_url", "company_url"} {
		if v := strings.TrimSpace(r.FormValue(field)); v != "" {
			if !strings.HasPrefix(v, "http") {
				writeError(w, http.StatusUnprocessableEntity, field+" must start with http:// or https://")
				return
			}
			fields[field] = v
			updated = append(updated, field)
		}
	}

	if len(fields) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "No fields provided to update.")
		return
	}

	if err := dbUpsertManualMeta(companyName, fields); err != nil {
		log.Printf("✗ dbUpsertManualMeta(%q): %v", companyName, err)
		writeError(w, http.StatusInternalServerError, "Failed to save manual data.")
		return
	}

	meta, _ := dbGetCompanyMeta(companyName)
	log.Printf("✓ meta/update: company=%q fields=%v", companyName, updated)
	writeJSON(w, http.StatusOK, UpdateMetaResponse{
		OK:      true,
		Company: companyName,
		Updated: updated,
		Meta:    meta,
	})
}

// handleCompanyVet serves POST /api/companies/vet — LLM company vetting with 7-day cache.
func handleCompanyVet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if err := parseAnyForm(r); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse form: %v", err))
		return
	}
	companyName := strings.TrimSpace(r.FormValue("company_name"))
	if companyName == "" {
		writeError(w, http.StatusUnprocessableEntity, "company_name is required.")
		return
	}
	provider := strings.TrimSpace(strings.ToLower(r.FormValue("provider")))
	if provider == "" {
		provider = "anthropic"
	}
	model := strings.TrimSpace(r.FormValue("model"))
	force := strings.TrimSpace(r.FormValue("force"))

	row, err := dbGetCompanyMeta(companyName)
	if err != nil {
		log.Printf("✗ dbGetCompanyMeta(%q): %v", companyName, err)
		writeError(w, http.StatusInternalServerError, "DB error")
		return
	}

	if !vettingForceRescan(force) && row != nil && strings.TrimSpace(row.LLMAssessedAt) != "" {
		if assessed, ok := parseLLMCacheTime(row.LLMAssessedAt); ok {
			if time.Since(assessed) < time.Duration(VETTING_CACHE_TTL_DAYS)*24*time.Hour {
				signals := []interface{}{}
				if strings.TrimSpace(row.LLMSignals) != "" {
					_ = json.Unmarshal([]byte(row.LLMSignals), &signals)
				}
				if signals == nil {
					signals = []interface{}{}
				}
				risk := strings.TrimSpace(row.LLMRiskLevel)
				if risk == "" {
					risk = "unknown"
				}
				log.Printf("✓ Returning cached vetting for: %q", companyName)
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"ok":          true,
					"cached":      true,
					"company":     companyName,
					"risk_level":  risk,
					"assessment":  row.LLMAssessment,
					"signals":     signals,
					"provider":    row.LLMProvider,
					"model":       row.LLMModel,
				})
				return
			}
		}
	}

	if row == nil || strings.TrimSpace(row.CrawledAt) == "" {
		log.Printf("→ auto-crawling %q before vetting", companyName)
		crawl := CrawlCompany(companyName)
		if err := dbUpsertCompanyMeta(companyName, crawl); err != nil {
			log.Printf("✗ dbUpsertCompanyMeta before vet: %v", err)
		}
		row, err = dbGetCompanyMeta(companyName)
		if err != nil {
			log.Printf("✗ dbGetCompanyMeta after crawl %q: %v", companyName, err)
			writeError(w, http.StatusInternalServerError, "DB error")
			return
		}
		if row == nil {
			row = &CompanyMeta{CompanyName: companyName}
		}
	}

	metaMap := companyMetaToVettingMeta(row)
	res, err := vetCompany(companyName, metaMap, provider, model, appCfg)
	if err != nil {
		log.Printf("✗ vetCompany %q: %v", companyName, err)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	sigBytes, jerr := json.Marshal(res.Signals)
	if jerr != nil {
		sigBytes = []byte("[]")
	}
	if err := dbUpsertCompanyVetting(companyName, res.RiskLevel, res.Assessment, string(sigBytes), res.Provider, res.Model); err != nil {
		log.Printf("✗ dbUpsertCompanyVetting(%q): %v", companyName, err)
		writeError(w, http.StatusInternalServerError, "failed to save vetting result")
		return
	}

	signalsOut := make([]interface{}, len(res.Signals))
	for i, s := range res.Signals {
		signalsOut[i] = s
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"cached":      false,
		"company":     companyName,
		"risk_level":  res.RiskLevel,
		"assessment":  res.Assessment,
		"signals":     signalsOut,
		"provider":    res.Provider,
		"model":       res.Model,
	})
}
