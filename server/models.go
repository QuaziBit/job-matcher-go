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
	DurationSeconds  int                `json:"duration_seconds"`
	AnalysisMode     string             `json:"analysis_mode"`
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
	LastModel     string `json:"last_model"`
	IsManual      bool   `json:"is_manual"`
	HasRecruiter  bool   `json:"has_recruiter"`
}

type SalaryEstimate struct {
	Min         int      `json:"min"`
	Max         int      `json:"max"`
	Currency    string   `json:"currency"`
	Period      string   `json:"period"`      // "year" | "hour"
	Confidence  string   `json:"confidence"`  // "high" | "medium" | "low"
	Signals     []string `json:"signals"`
	LLMProvider string   `json:"llm_provider"`
	LLMModel    string   `json:"llm_model"`
	Source      string   `json:"source"` // "estimated" | "posted"
}

type JobTextQuality struct {
	Level         string   `json:"level"`          // "ok", "warn", "poor"
	Issues        []string `json:"issues"`
	CharCount     int      `json:"char_count"`
	TechKeywords  int      `json:"tech_keywords"`  // count of recognized tech terms
	BuzzwordCount int      `json:"buzzword_count"`
}

type ScrapePreviewResponse struct {
	Title           string         `json:"title"`
	Company         string         `json:"company"`
	Location        string         `json:"location"`
	Description     string         `json:"description"`
	BlockerKeywords []string       `json:"blocker_keywords"`
	TextQuality     JobTextQuality `json:"text_quality"`
	HasWarnings     bool           `json:"has_warnings"`
}

type ResumeComparison struct {
	ResumeA      Analysis
	ResumeB      Analysis
	BetterFit    string // label of winning resume
	BetterReason string // one-sentence explanation
}

type JobDetailView struct {
	Job              Job
	Application      Application
	Analyses         []Analysis
	Resumes          []Resume
	OllamaModel      string // pre-selected Ollama model (from last analysis or config default)
	AnthropicModel   string // pre-selected Anthropic model (from last analysis or config default)
	OpenAIModel      string // pre-selected OpenAI model (from last analysis or config default)
	GeminiModel      string // pre-selected Gemini model (from last analysis or config default)
	AnalysisMode     string // kept for backwards compat
	LastAnalysisMode string // pre-selected mode (from last analysis or config default)
	TextQuality      JobTextQuality
	Comparison       *ResumeComparison // nil if < 2 distinct resumes analyzed
	LastResumeID     int64             // resume_id from most recent analysis (0 if none)
	LastProvider     string            // llm_provider from most recent analysis
	HasAnthropic     bool              // true if Anthropic API key is configured
	HasOpenAI        bool              // true if OpenAI API key is configured
	HasGemini        bool              // true if Gemini API key is configured
	HasOllama        bool              // true if Ollama is reachable
	SalaryEstimate   *SalaryEstimate   // nil if not yet estimated
	HasSalaryInJD    bool              // true if JD contains explicit salary info
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
	Search    string
	Status    string
	Score     string // "", "0", "2", "3", "4", "5"
	Provider  string
	Page      int
	PerPage   int // 0 = all
	AddedDays int    // simple: last N days (0 = any time)
	DateFrom  string // advanced: YYYY-MM-DD lower bound
	DateTo    string // advanced: YYYY-MM-DD upper bound
}

type JobsListResponse struct {
	Jobs       []JobListItem `json:"jobs"`
	Total      int           `json:"total"`
	Page       int           `json:"page"`
	PerPage    int           `json:"per_page"`
	TotalPages int           `json:"total_pages"`
}
