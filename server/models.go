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

type MatchedSkill struct {
	Skill         string `json:"skill"`
	MatchType     string `json:"match_type"`     // exact | partial | inferred
	JDSnippet     string `json:"jd_snippet"`     // max 100 chars from JD
	ResumeSnippet string `json:"resume_snippet"` // max 100 chars from resume
	Category      string `json:"category"`       // populated by NormalizeSkill pipeline
}

type MissingSkill struct {
	Skill           string `json:"skill"`
	Severity        string `json:"severity"`         // blocker | major | minor
	RequirementType string `json:"requirement_type"` // hard | preferred | bonus
	JDSnippet       string `json:"jd_snippet"`       // max 100 chars from JD
	ClusterGroup    string `json:"cluster_group"`    // populated by skill normalization
}

type PenaltyBreakdown struct {
	Blockers       int            `json:"blockers"`
	Majors         int            `json:"majors"`
	Minors         int            `json:"minors"`
	BlockerPenalty int            `json:"blocker_penalty"`
	MajorPenalty   int            `json:"major_penalty"`
	MinorPenalty   int            `json:"minor_penalty"`
	CountPenalty   int            `json:"count_penalty"`
	TotalPenalty   int            `json:"total_penalty"`
	Clusters       map[string]int `json:"clusters"` // group name → penalty applied
}

type ResumeSuggestion struct {
	Title          string `json:"title"`           // short label
	Detail         string `json:"detail"`          // specific actionable text
	JobRequirement string `json:"job_requirement"` // which JD requirement this addresses
}

type Analysis struct {
	ID               int64            `json:"id"`
	JobID            int64            `json:"job_id"`
	ResumeID         int64            `json:"resume_id"`
	ResumeLabel      string           `json:"resume_label"`
	Score            int              `json:"score"`
	AdjustedScore    int              `json:"adjusted_score"`
	PenaltyBreakdown PenaltyBreakdown `json:"penalty_breakdown"`
	MatchedSkills    []MatchedSkill   `json:"matched_skills"`
	MissingSkills    []MissingSkill   `json:"missing_skills"`
	Reasoning        string           `json:"reasoning"`
	LLMProvider      string           `json:"llm_provider"`
	LLMModel         string           `json:"llm_model"`
	CreatedAt        time.Time        `json:"created_at"`
	ValidationErrors string             `json:"validation_errors"`
	RetryCount       int                `json:"retry_count"`
	UsedFallback     bool               `json:"used_fallback"`
	Suggestions      []ResumeSuggestion `json:"suggestions"`
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

type JobTextQuality struct {
	Level         string   // "ok", "warn", "poor"
	Issues        []string
	CharCount     int
	TechKeywords  int // count of recognized tech terms
	BuzzwordCount int
}

type ResumeComparison struct {
	ResumeA      Analysis
	ResumeB      Analysis
	BetterFit    string // label of winning resume
	BetterReason string // one-sentence explanation
}

type JobDetailView struct {
	Job         Job
	Application Application
	Analyses    []Analysis
	Resumes     []Resume
	OllamaModel string
	TextQuality JobTextQuality
	Comparison  *ResumeComparison // nil if < 2 distinct resumes analyzed
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
