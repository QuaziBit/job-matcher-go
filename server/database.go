package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func initDB(dbPath string) error {
	log.Printf("→ Opening SQLite database: %s", dbPath)
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping sqlite %q: %w", dbPath, err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if err := createSchema(); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	runMigrations()
	log.Printf("✓ SQLite ready: %s", dbPath)
	return nil
}

func createSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS resumes (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		label      TEXT NOT NULL,
		content    TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS jobs (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		url             TEXT NOT NULL UNIQUE,
		title           TEXT,
		company         TEXT,
		location        TEXT,
		raw_description TEXT,
		scraped_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS analyses (
		id                INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id            INTEGER NOT NULL,
		resume_id         INTEGER NOT NULL,
		score             INTEGER NOT NULL,
		adjusted_score    INTEGER NOT NULL DEFAULT 0,
		penalty_breakdown TEXT DEFAULT '{}',
		matched_skills    TEXT,
		missing_skills    TEXT,
		reasoning         TEXT,
		llm_provider      TEXT DEFAULT 'anthropic',
		llm_model         TEXT DEFAULT '',
		created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (job_id)    REFERENCES jobs(id)    ON DELETE CASCADE,
		FOREIGN KEY (resume_id) REFERENCES resumes(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS applications (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id          INTEGER NOT NULL UNIQUE,
		status          TEXT DEFAULT 'not_applied',
		recruiter_name  TEXT,
		recruiter_email TEXT,
		recruiter_phone TEXT,
		notes           TEXT,
		updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
	);`
	_, err := db.Exec(schema)
	return err
}

// runMigrations adds new columns to existing tables. Each ALTER TABLE is
// executed with the error ignored — SQLite returns "duplicate column name"
// if the column already exists, which is safe to ignore.
func runMigrations() {
	migrations := []string{
		"ALTER TABLE analyses ADD COLUMN matched_skills_v2 TEXT DEFAULT '[]'",
		"ALTER TABLE analyses ADD COLUMN missing_skills_v2 TEXT DEFAULT '[]'",
		"ALTER TABLE analyses ADD COLUMN validation_errors TEXT DEFAULT ''",
		"ALTER TABLE analyses ADD COLUMN retry_count INTEGER DEFAULT 0",
		"ALTER TABLE analyses ADD COLUMN used_fallback INTEGER DEFAULT 0",
		"ALTER TABLE analyses ADD COLUMN suggestions TEXT DEFAULT '[]'",
		"ALTER TABLE analyses ADD COLUMN duration_seconds INTEGER DEFAULT 0",
		"ALTER TABLE analyses ADD COLUMN analysis_mode TEXT DEFAULT 'standard'",
	}
	for _, m := range migrations {
		_, _ = db.Exec(m)
	}
}

// ── Resumes ───────────────────────────────────────────────────────────────────

func dbGetResumes() ([]Resume, error) {
	rows, err := db.Query(`SELECT id, label, content, created_at FROM resumes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var resumes []Resume
	for rows.Next() {
		var r Resume
		var ts string
		if err := rows.Scan(&r.ID, &r.Label, &r.Content, &ts); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = parseTS(ts)
		resumes = append(resumes, r)
	}
	return resumes, nil
}

func dbGetResumeByID(id int64) (*Resume, error) {
	var r Resume
	var ts string
	err := db.QueryRow(`SELECT id, label, content, created_at FROM resumes WHERE id = ?`, id).
		Scan(&r.ID, &r.Label, &r.Content, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.CreatedAt, _ = parseTS(ts)
	return &r, nil
}

func dbInsertResume(label, content string) (int64, error) {
	res, err := db.Exec(`INSERT INTO resumes (label, content) VALUES (?, ?)`, label, content)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func dbDeleteResume(id int64) error {
	_, err := db.Exec(`DELETE FROM resumes WHERE id = ?`, id)
	return err
}

// ── Jobs ──────────────────────────────────────────────────────────────────────

func dbGetJobListItems(f JobFilters) ([]JobListItem, int, error) {
	// ── Build WHERE clause dynamically ───────────────────────────────────────
	where := []string{}
	args  := []interface{}{}

	if f.Search != "" {
		where = append(where, "(LOWER(j.title) LIKE ? OR LOWER(j.company) LIKE ?)")
		like := "%" + strings.ToLower(f.Search) + "%"
		args  = append(args, like, like)
	}
	if f.Status != "" {
		where = append(where, "COALESCE(a.status, 'not_applied') = ?")
		args  = append(args, f.Status)
	}
	if f.Provider != "" {
		if f.Provider == "manual" {
			where = append(where, "j.url LIKE 'manual://%'")
		} else {
			where = append(where,
				"(SELECT llm_provider FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1) = ?")
			args = append(args, f.Provider)
		}
	}
	if f.Score != "" {
		switch f.Score {
		case "0": // not scored — no analyses at all
			log.Printf("→ dbGetJobListItems: score filter = not scored")
			where = append(where,
				"(SELECT COUNT(*) FROM analyses WHERE job_id = j.id) = 0")
		case "5": // exact match only
			log.Printf("→ dbGetJobListItems: score filter = exactly 5")
			where = append(where,
				"COALESCE((SELECT adjusted_score FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1),"+
					"(SELECT score FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1), 0) = 5")
		default: // >= minimum, pass as integer
			minScore, err := strconv.Atoi(f.Score)
			if err != nil {
				log.Printf("✗ dbGetJobListItems: invalid score filter %q: %v — ignoring", f.Score, err)
			} else {
				log.Printf("→ dbGetJobListItems: score filter >= %d", minScore)
				where = append(where,
					"COALESCE((SELECT adjusted_score FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1),"+
						"(SELECT score FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1), 0) >= ?")
				args = append(args, minScore)
			}
		}
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	baseQuery := `
		SELECT j.id, j.url, j.title, j.company, j.location, j.scraped_at,
		       COALESCE(a.status, 'not_applied'),
		       (SELECT score          FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1),
		       (SELECT adjusted_score FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1),
		       (SELECT llm_provider   FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1)
		FROM jobs j
		LEFT JOIN applications a ON a.job_id = j.id
		` + whereSQL + `
		ORDER BY j.scraped_at DESC`

	// ── Count total matching rows ─────────────────────────────────────────────
	countQuery := `SELECT COUNT(*) FROM jobs j LEFT JOIN applications a ON a.job_id = j.id ` + whereSQL
	var total int
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count query: %w", err)
	}

	// ── Apply LIMIT / OFFSET ──────────────────────────────────────────────────
	page    := f.Page
	perPage := f.PerPage
	if page < 1    { page = 1 }
	if perPage < 0 { perPage = 0 }

	paginatedArgs := append(args[:len(args):len(args)], args...)
	paginatedArgs  = args

	if perPage > 0 {
		offset := (page - 1) * perPage
		baseQuery += " LIMIT ? OFFSET ?"
		paginatedArgs = append(args, perPage, offset)
	}

	// ── Execute main query ────────────────────────────────────────────────────
	rows, err := db.Query(baseQuery, paginatedArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []JobListItem
	for rows.Next() {
		var item JobListItem
		var ts string
		var score, adjScore sql.NullInt64
		var provider sql.NullString
		if err := rows.Scan(
			&item.ID, &item.URL, &item.Title, &item.Company, &item.Location, &ts,
			&item.Status, &score, &adjScore, &provider,
		); err != nil {
			return nil, 0, err
		}
		item.ScrapedAt, _ = parseTS(ts)
		if score.Valid {
			v := int(score.Int64)
			item.BestScore = &v
		}
		if adjScore.Valid {
			v := int(adjScore.Int64)
			item.AdjustedScore = &v
		}
		if provider.Valid {
			item.Provider = provider.String
		}
		item.IsManual = strings.HasPrefix(item.URL, "manual://")
		items = append(items, item)
	}
	return items, total, nil
}

func dbGetJobByID(id int64) (*Job, error) {
	var j Job
	var ts string
	err := db.QueryRow(
		`SELECT id, url, title, company, location, raw_description, scraped_at FROM jobs WHERE id = ?`, id,
	).Scan(&j.ID, &j.URL, &j.Title, &j.Company, &j.Location, &j.RawDescription, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.ScrapedAt, _ = parseTS(ts)
	return &j, nil
}

func dbGetJobByURL(u string) (*Job, error) {
	var j Job
	var ts string
	err := db.QueryRow(
		`SELECT id, url, title, company, location, raw_description, scraped_at FROM jobs WHERE url = ?`, u,
	).Scan(&j.ID, &j.URL, &j.Title, &j.Company, &j.Location, &j.RawDescription, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.ScrapedAt, _ = parseTS(ts)
	return &j, nil
}

func dbInsertJob(jobURL, title, company, location, description string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO jobs (url, title, company, location, raw_description) VALUES (?, ?, ?, ?, ?)`,
		jobURL, title, company, location, description,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func dbDeleteJob(id int64) error {
	_, err := db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}

// ── Analyses ──────────────────────────────────────────────────────────────────

func dbGetAnalysesByJobID(jobID int64) ([]Analysis, error) {
	rows, err := db.Query(`
		SELECT a.id, a.job_id, a.resume_id, r.label,
		       a.score, a.adjusted_score, a.penalty_breakdown,
		       a.matched_skills, a.missing_skills, a.reasoning,
		       a.llm_provider, a.llm_model, a.created_at,
		       COALESCE(a.matched_skills_v2, '[]'),
		       COALESCE(a.missing_skills_v2, '[]'),
		       COALESCE(a.validation_errors, ''),
		       COALESCE(a.retry_count, 0),
		       COALESCE(a.used_fallback, 0),
		       COALESCE(a.suggestions, '[]'),
		       COALESCE(a.duration_seconds, 0),
		       COALESCE(a.analysis_mode, 'standard')
		FROM analyses a
		JOIN resumes r ON r.id = a.resume_id
		WHERE a.job_id = ?
		ORDER BY a.created_at DESC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var analyses []Analysis
	for rows.Next() {
		var a Analysis
		var ts, pbJSON, matchedV1JSON, missingV1JSON, matchedV2JSON, missingV2JSON, suggestionsJSON string
		var usedFallbackInt int
		if err := rows.Scan(
			&a.ID, &a.JobID, &a.ResumeID, &a.ResumeLabel,
			&a.Score, &a.AdjustedScore, &pbJSON,
			&matchedV1JSON, &missingV1JSON, &a.Reasoning,
			&a.LLMProvider, &a.LLMModel, &ts,
			&matchedV2JSON, &missingV2JSON,
			&a.ValidationErrors, &a.RetryCount, &usedFallbackInt,
			&suggestionsJSON,
			&a.DurationSeconds, &a.AnalysisMode,
		); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = parseTS(ts)
		a.UsedFallback = usedFallbackInt != 0
		json.Unmarshal([]byte(pbJSON), &a.PenaltyBreakdown)
		json.Unmarshal([]byte(suggestionsJSON), &a.Suggestions)

		// Prefer v2 columns; fall back to v1 if v2 is empty
		var matchedV2 []MatchedSkill
		if err := json.Unmarshal([]byte(matchedV2JSON), &matchedV2); err == nil && len(matchedV2) > 0 {
			a.MatchedSkills = matchedV2
		} else {
			// v1 fallback: plain string list → MatchedSkill stubs
			var v1Names []string
			if err := json.Unmarshal([]byte(matchedV1JSON), &v1Names); err == nil {
				for _, name := range v1Names {
					a.MatchedSkills = append(a.MatchedSkills, MatchedSkill{Skill: name, MatchType: "exact"})
				}
			}
		}

		var missingV2 []MissingSkill
		if err := json.Unmarshal([]byte(missingV2JSON), &missingV2); err == nil && len(missingV2) > 0 {
			a.MissingSkills = missingV2
		} else {
			a.MissingSkills = parseMissingSkills(missingV1JSON)
		}

		if a.AdjustedScore == 0 {
			a.AdjustedScore = a.Score
		}
		analyses = append(analyses, a)
	}
	return analyses, nil
}

func dbInsertAnalysis(a Analysis) (int64, error) {
	pbJSON, _ := json.Marshal(a.PenaltyBreakdown)
	// v2: structured MatchedSkill / MissingSkill with snippets
	matchedV2JSON, _ := json.Marshal(a.MatchedSkills)
	missingV2JSON, _ := json.Marshal(a.MissingSkills)
	// v1: plain skill name strings for backward compat
	matchedV1 := make([]string, len(a.MatchedSkills))
	for i, m := range a.MatchedSkills {
		matchedV1[i] = m.Skill
	}
	matchedV1JSON, _ := json.Marshal(matchedV1)
	missingV1JSON, _ := json.Marshal(a.MissingSkills) // still valid JSON for old readers

	suggestionsJSON, _ := json.Marshal(a.Suggestions)
	usedFallbackInt := 0
	if a.UsedFallback {
		usedFallbackInt = 1
	}
	res, err := db.Exec(`
		INSERT INTO analyses
		(job_id, resume_id, score, adjusted_score, penalty_breakdown,
		 matched_skills, missing_skills, reasoning, llm_provider, llm_model,
		 matched_skills_v2, missing_skills_v2,
		 validation_errors, retry_count, used_fallback, suggestions,
		 duration_seconds, analysis_mode)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.JobID, a.ResumeID, a.Score, a.AdjustedScore, string(pbJSON),
		string(matchedV1JSON), string(missingV1JSON), a.Reasoning,
		a.LLMProvider, a.LLMModel,
		string(matchedV2JSON), string(missingV2JSON),
		a.ValidationErrors, a.RetryCount, usedFallbackInt, string(suggestionsJSON),
		a.DurationSeconds, a.AnalysisMode,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func dbDeleteAnalysis(id int64) (bool, error) {
	res, err := db.Exec(`DELETE FROM analyses WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ── Applications ──────────────────────────────────────────────────────────────

func dbGetApplicationByJobID(jobID int64) (*Application, error) {
	var app Application
	var ts string
	err := db.QueryRow(`
		SELECT id, job_id, status, recruiter_name, recruiter_email,
		       recruiter_phone, notes, updated_at
		FROM applications WHERE job_id = ?`, jobID,
	).Scan(&app.ID, &app.JobID, &app.Status,
		&app.RecruiterName, &app.RecruiterEmail, &app.RecruiterPhone,
		&app.Notes, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	app.UpdatedAt, _ = parseTS(ts)
	return &app, nil
}

func dbUpsertApplication(app Application) error {
	_, err := db.Exec(`
		INSERT INTO applications
		(job_id, status, recruiter_name, recruiter_email, recruiter_phone, notes)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET
			status          = excluded.status,
			recruiter_name  = excluded.recruiter_name,
			recruiter_email = excluded.recruiter_email,
			recruiter_phone = excluded.recruiter_phone,
			notes           = excluded.notes,
			updated_at      = CURRENT_TIMESTAMP`,
		app.JobID, app.Status, app.RecruiterName,
		app.RecruiterEmail, app.RecruiterPhone, app.Notes,
	)
	return err
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parseTS(ts string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, ts); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not parse timestamp: %s", ts)
}

func parseMissingSkills(raw string) []MissingSkill {
	var result []MissingSkill
	var rawMissing []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rawMissing); err != nil {
		return result
	}
	for _, r := range rawMissing {
		var structured MissingSkill
		if err := json.Unmarshal(r, &structured); err == nil && structured.Skill != "" {
			if structured.RequirementType == "" {
				structured.RequirementType = "preferred"
			}
			result = append(result, structured)
		} else {
			var flat string
			if err := json.Unmarshal(r, &flat); err == nil {
				result = append(result, MissingSkill{Skill: flat, Severity: "minor", RequirementType: "preferred"})
			}
		}
	}
	return result
}
