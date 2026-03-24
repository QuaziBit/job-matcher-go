package server

import "time"

// ── Core domain models ────────────────────────────────────────────────────────

type Resume struct {
	ID        int64     `json:"id"`
	Label     string    `json:"label"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Job struct {
	ID             int64     `json:"id"`
	URL            string    `json:"url"`
	Title          string    `json:"title"`
	Company        string    `json:"company"`
	Location       string    `json:"location"`
	RawDescription string    `json:"raw_description"`
	ScrapedAt      time.Time `json:"scraped_at"`
}

type MissingSkill struct {
	Skill    string `json:"skill"`
	Severity string `json:"severity"` // blocker | major | minor
}

type PenaltyBreakdown struct {
	Blockers       int `json:"blockers"`
	Majors         int `json:"majors"`
	Minors         int `json:"minors"`
	BlockerPenalty int `json:"blocker_penalty"`
	MajorPenalty   int `json:"major_penalty"`
	MinorPenalty   int `json:"minor_penalty"`
	CountPenalty   int `json:"count_penalty"`
	TotalPenalty   int `json:"total_penalty"`
}

type Analysis struct {
	ID               int64            `json:"id"`
	JobID            int64            `json:"job_id"`
	ResumeID         int64            `json:"resume_id"`
	ResumeLabel      string           `json:"resume_label"`
	Score            int              `json:"score"`
	AdjustedScore    int              `json:"adjusted_score"`
	PenaltyBreakdown PenaltyBreakdown `json:"penalty_breakdown"`
	MatchedSkills    []string         `json:"matched_skills"`
	MissingSkills    []MissingSkill   `json:"missing_skills"`
	Reasoning        string           `json:"reasoning"`
	LLMProvider      string           `json:"llm_provider"`
	LLMModel         string           `json:"llm_model"`
	CreatedAt        time.Time        `json:"created_at"`
}

type Application struct {
	ID             int64     `json:"id"`
	JobID          int64     `json:"job_id"`
	Status         string    `json:"status"`
	RecruiterName  string    `json:"recruiter_name"`
	RecruiterEmail string    `json:"recruiter_email"`
	RecruiterPhone string    `json:"recruiter_phone"`
	Notes          string    `json:"notes"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ── View models (for templates) ───────────────────────────────────────────────

type JobListItem struct {
	Job
	Status        string `json:"status"`
	BestScore     *int   `json:"best_score"`
	AdjustedScore *int   `json:"adjusted_score"`
	Provider      string `json:"provider"`
	IsManual      bool   `json:"is_manual"`
}

type JobDetailView struct {
	Job         Job
	Application Application
	Analyses    []Analysis
	Resumes     []Resume
	OllamaModel string
}

type IndexView struct {
	Jobs    []JobListItem
	Resumes []Resume
}

type ResumesView struct {
	Resumes []Resume
}

// ── API response helpers ──────────────────────────────────────────────────────

type APIError struct {
	Error string `json:"error"`
}

type APIOK struct {
	OK bool `json:"ok"`
}

// ── Pagination & filter models ────────────────────────────────────────────────

type JobFilters struct {
	Search   string
	Status   string
	Score    string // "", "0", "2", "3", "4", "5"
	Provider string
	Page     int
	PerPage  int // 0 = all
}

type JobsListResponse struct {
	Jobs       []JobListItem `json:"jobs"`
	Total      int           `json:"total"`
	Page       int           `json:"page"`
	PerPage    int           `json:"per_page"`
	TotalPages int           `json:"total_pages"`
}
