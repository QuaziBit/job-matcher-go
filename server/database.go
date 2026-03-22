package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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

func dbGetJobListItems() ([]JobListItem, error) {
	rows, err := db.Query(`
		SELECT j.id, j.url, j.title, j.company, j.location, j.scraped_at,
		       COALESCE(a.status, 'not_applied'),
		       (SELECT score          FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1),
		       (SELECT adjusted_score FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1),
		       (SELECT llm_provider   FROM analyses WHERE job_id = j.id ORDER BY created_at DESC LIMIT 1)
		FROM jobs j
		LEFT JOIN applications a ON a.job_id = j.id
		ORDER BY j.scraped_at DESC`)
	if err != nil {
		return nil, err
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
			return nil, err
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
	return items, nil
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
		       a.llm_provider, a.llm_model, a.created_at
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
		var ts, pbJSON, matchedJSON, missingJSON string
		if err := rows.Scan(
			&a.ID, &a.JobID, &a.ResumeID, &a.ResumeLabel,
			&a.Score, &a.AdjustedScore, &pbJSON,
			&matchedJSON, &missingJSON, &a.Reasoning,
			&a.LLMProvider, &a.LLMModel, &ts,
		); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = parseTS(ts)
		json.Unmarshal([]byte(pbJSON), &a.PenaltyBreakdown)
		json.Unmarshal([]byte(matchedJSON), &a.MatchedSkills)
		a.MissingSkills = parseMissingSkills(missingJSON)
		if a.AdjustedScore == 0 {
			a.AdjustedScore = a.Score
		}
		analyses = append(analyses, a)
	}
	return analyses, nil
}

func dbInsertAnalysis(a Analysis) (int64, error) {
	pbJSON, _ := json.Marshal(a.PenaltyBreakdown)
	matchedJSON, _ := json.Marshal(a.MatchedSkills)
	missingJSON, _ := json.Marshal(a.MissingSkills)

	res, err := db.Exec(`
		INSERT INTO analyses
		(job_id, resume_id, score, adjusted_score, penalty_breakdown,
		 matched_skills, missing_skills, reasoning, llm_provider, llm_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.JobID, a.ResumeID, a.Score, a.AdjustedScore, string(pbJSON),
		string(matchedJSON), string(missingJSON), a.Reasoning,
		a.LLMProvider, a.LLMModel,
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
			result = append(result, structured)
		} else {
			var flat string
			if err := json.Unmarshal(r, &flat); err == nil {
				result = append(result, MissingSkill{Skill: flat, Severity: "minor"})
			}
		}
	}
	return result
}
