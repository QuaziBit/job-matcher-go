package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/QuaziBit/job-matcher-go/config"
)

const anthropicModel = "claude-opus-4-5"

var blockerKeywords = []string{
	"clearance", "ts/sci", "top secret", "secret", "polygraph",
	"citizenship", "citizen only", "usc only",
}

var yearPattern = regexp.MustCompile(`(\d+)\+?\s*years?\s*(of\s*)?(\w+\s*)*?(experience|exp)`)

const systemPrompt = `You are an expert technical recruiter and career coach specializing in software engineering,
DevSecOps, and cloud infrastructure roles. You evaluate how well a candidate's resume matches a job description.

You MUST respond with ONLY valid JSON — no prose, no markdown, no code fences. Exactly this shape:
{
  "score": <integer 1-5>,
  "matched_skills": [
    {"skill": "skill name", "match_type": "exact|partial|inferred", "jd_snippet": "verbatim phrase from JD (max 100 chars)", "resume_snippet": "verbatim phrase from resume (max 100 chars)"},
    ...
  ],
  "missing_skills": [
    {"skill": "skill name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "verbatim phrase from JD (max 100 chars)"},
    ...
  ],
  "reasoning": "<2-4 sentence honest assessment>",
  "suggestions": [
    {"title": "short label", "detail": "specific actionable text referencing real resume phrases", "job_requirement": "verbatim JD phrase this addresses"},
    ...
  ]
}

Suggestion rules — you MUST follow these exactly:
  - Generate exactly 3 resume improvement suggestions
  - ONLY suggest clarifying, repositioning, or expanding EXISTING resume content
  - NEVER suggest adding skills, certifications, or experience the candidate does not already have
  - Each suggestion must cite the specific job requirement it addresses
  - Be concrete — reference actual resume phrases and JD phrases
  - If the resume is already strong for a requirement, skip it (fewer than 3 is acceptable)

Snippet rules — you MUST follow these exactly:
  - Snippets must be verbatim phrases copied from the provided text, max 100 characters
  - Do NOT fabricate or paraphrase snippets
  - If you cannot find a direct phrase for a matched skill, set match_type to "inferred" and omit resume_snippet
  - match_type values: exact = skill name appears verbatim, partial = related term found, inferred = implied by context

Severity definitions for missing_skills:
  blocker = eliminates candidacy entirely (e.g. required clearance, mandatory cert, minimum years not met)
  major   = significant gap that will hurt chances substantially
  minor   = nice-to-have or learnable gap that is unlikely to disqualify

Requirement type definitions for missing_skills:
  hard      = job uses words like: required, must have, must hold, mandatory, eligibility-blocking
  preferred = job uses words like: preferred, desired, strong plus, ideally
  bonus     = job uses words like: nice to have, is a plus, familiarity with
  If unclear, use "preferred" as the default.

Scoring rubric:
  1 = Poor match — major gaps, different domain entirely
  2 = Weak match — some overlap but significant missing requirements
  3 = Moderate match — meets roughly half the requirements
  4 = Strong match — meets most requirements with minor gaps
  5 = Excellent match — highly aligned, apply immediately`

func buildUserPrompt(resume, jobDescription string) string {
	return fmt.Sprintf("## RESUME\n%s\n\n---\n\n## JOB DESCRIPTION\n%s\n\n---\n\nEvaluate the match and return ONLY the JSON object described in your instructions.", resume, jobDescription)
}

// ── LLM response parsing ──────────────────────────────────────────────────────

type llmRawResponse struct {
	Score         int                `json:"score"`
	MatchedSkills []json.RawMessage  `json:"matched_skills"`
	MissingSkills []json.RawMessage  `json:"missing_skills"`
	Reasoning     string             `json:"reasoning"`
	Suggestions   []json.RawMessage  `json:"suggestions"`
}

func parseLLMResponse(raw, jobDescription string) (Analysis, error) {
	// Strip markdown fences
	raw = regexp.MustCompile("```(?:json)?").ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)

	// Find first JSON object
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return Analysis{}, fmt.Errorf("no JSON object found in LLM response")
	}
	raw = raw[start : end+1]

	var resp llmRawResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return Analysis{}, fmt.Errorf("failed to parse LLM JSON: %w", err)
	}

	if resp.Score < 1 || resp.Score > 5 {
		return Analysis{}, fmt.Errorf("score out of range: %d", resp.Score)
	}

	// Parse matched skills — handle v2 [{skill,match_type,jd_snippet,resume_snippet}] or v1 ["skill"]
	var matched []MatchedSkill
	for _, r := range resp.MatchedSkills {
		var v2 MatchedSkill
		if err := json.Unmarshal(r, &v2); err == nil && v2.Skill != "" {
			if v2.MatchType == "" {
				v2.MatchType = "exact"
			}
			matched = append(matched, v2)
		} else {
			var flat string
			if err := json.Unmarshal(r, &flat); err == nil && flat != "" {
				matched = append(matched, MatchedSkill{Skill: flat, MatchType: "exact"})
			}
		}
	}

	// Parse missing skills — handle v2 [{skill,severity,requirement_type,jd_snippet}] or v1 ["skill"]
	var missing []MissingSkill
	for _, r := range resp.MissingSkills {
		var structured MissingSkill
		if err := json.Unmarshal(r, &structured); err == nil && structured.Skill != "" {
			if structured.RequirementType == "" {
				structured.RequirementType = "preferred"
			}
			missing = append(missing, structured)
		} else {
			var flat string
			if err := json.Unmarshal(r, &flat); err == nil && flat != "" {
				missing = append(missing, MissingSkill{Skill: flat, Severity: "minor", RequirementType: "preferred"})
			}
		}
	}

	// Apply skill normalization — canonicalize names and assign categories
	for i := range matched {
		matched[i].Skill = NormalizeSkill(matched[i].Skill)
		matched[i].Category = GetSkillCategory(matched[i].Skill)
	}
	for i := range missing {
		missing[i].Skill = NormalizeSkill(missing[i].Skill)
		missing[i].ClusterGroup = GetSkillCategory(missing[i].Skill)
	}

	// Keyword detector pass — upgrade severities
	if jobDescription != "" {
		missing = keywordBoost(missing, jobDescription)
	}

	adjusted, breakdown := computeAdjustedScore(resp.Score, missing)

	// Parse suggestions — handle both object format and plain string format.
	// Ollama models often return plain strings despite being asked for objects.
	// We accept both and normalize to ResumeSuggestion.
	var suggestions []ResumeSuggestion
	for _, raw := range resp.Suggestions {
		// Try object format first: {"title":..., "detail":..., "job_requirement":...}
		var s ResumeSuggestion
		if err := json.Unmarshal(raw, &s); err == nil && s.Detail != "" {
			suggestions = append(suggestions, s)
			continue
		}
		// Fall back to plain string format: "some suggestion text"
		var str string
		if err := json.Unmarshal(raw, &str); err == nil && str != "" {
			suggestions = append(suggestions, ResumeSuggestion{
				Title:  "Suggestion",
				Detail: str,
			})
			continue
		}
		log.Printf("→ skipping unparseable suggestion: %s", string(raw))
	}
	if len(suggestions) > 3 {
		suggestions = suggestions[:3]
	}

	return Analysis{
		Score:            resp.Score,
		AdjustedScore:    adjusted,
		PenaltyBreakdown: breakdown,
		MatchedSkills:    matched,
		MissingSkills:    missing,
		Reasoning:        resp.Reasoning,
		Suggestions:      suggestions,
	}, nil
}

// penaltyForSkill returns the penalty points for a single missing skill.
// Bonus requirement type always returns 0 regardless of severity.
func penaltyForSkill(skill MissingSkill) int {
	if skill.RequirementType == "bonus" {
		return 0
	}
	switch skill.Severity {
	case "blocker":
		return 2
	case "major":
		return 1
	default: // minor
		return 0 // minors are aggregated by count in computeAdjustedScore
	}
}

// keywordBoost upgrades severity of missing skills matching hard-blocker patterns.
func keywordBoost(skills []MissingSkill, jd string) []MissingSkill {
	jdLower := strings.ToLower(jd)
	jdHasBlocker := false
	for _, kw := range blockerKeywords {
		if strings.Contains(jdLower, kw) {
			jdHasBlocker = true
			break
		}
	}
	jdHasYears := yearPattern.MatchString(jdLower)

	result := make([]MissingSkill, len(skills))
	for i, s := range skills {
		skillLower := strings.ToLower(s.Skill)
		severity := s.Severity

		for _, kw := range blockerKeywords {
			if strings.Contains(skillLower, kw) {
				severity = "blocker"
				break
			}
		}
		if yearPattern.MatchString(skillLower) && jdHasYears {
			severity = "blocker"
		}
		if severity == "major" && jdHasBlocker &&
			(strings.Contains(skillLower, "required") || strings.Contains(skillLower, "must")) {
			severity = "blocker"
		}
		// Preserve all fields; only overwrite severity
		result[i] = s
		result[i].Severity = severity
	}
	return result
}

// clusterPenaltyCap returns the maximum total penalty allowed for a skill cluster group.
func clusterPenaltyCap(group string) int {
	if group == "security" {
		return 2
	}
	return 1
}

// computeAdjustedScore applies the full penalty pipeline with per-cluster caps.
func computeAdjustedScore(rawScore int, missing []MissingSkill) (int, PenaltyBreakdown) {
	// Ensure ClusterGroup is set on all skills
	for i := range missing {
		if missing[i].ClusterGroup == "" {
			missing[i].ClusterGroup = GetSkillCategory(missing[i].Skill)
		}
	}

	// Count severity totals and group by cluster
	var blockers, majors, minors int
	type clusterData struct{ rawPenalty int }
	clusters := map[string]*clusterData{}

	for _, s := range missing {
		switch s.Severity {
		case "blocker":
			blockers++
		case "major":
			majors++
		default:
			minors++
		}
		p := penaltyForSkill(s)
		if p > 0 {
			if clusters[s.ClusterGroup] == nil {
				clusters[s.ClusterGroup] = &clusterData{}
			}
			clusters[s.ClusterGroup].rawPenalty += p
		}
	}

	// Cap each cluster and sum up
	clusterPenalties := map[string]int{}
	clusterTotal := 0
	for group, data := range clusters {
		cap := clusterPenaltyCap(group)
		capped := data.rawPenalty
		if capped > cap {
			capped = cap
		}
		clusterPenalties[group] = capped
		clusterTotal += capped
	}

	// For the breakdown display, report raw severity penalties capped globally
	bp := blockers * 2
	if bp > 3 {
		bp = 3
	}
	mp := majors * 1
	if mp > 2 {
		mp = 2
	}
	mnp := minors / 2
	if mnp > 1 {
		mnp = 1
	}
	cp := 0
	if len(missing) > 6 {
		cp = 1
	}

	total := clusterTotal + mnp + cp
	adjusted := rawScore - total
	if adjusted < 1 {
		adjusted = 1
	}

	return adjusted, PenaltyBreakdown{
		Blockers:       blockers,
		Majors:         majors,
		Minors:         minors,
		BlockerPenalty: bp,
		MajorPenalty:   mp,
		MinorPenalty:   mnp,
		CountPenalty:   cp,
		TotalPenalty:   total,
		Clusters:       clusterPenalties,
	}
}

// ── Anthropic ─────────────────────────────────────────────────────────────────

func analyzeWithAnthropic(resume, jobDescription, apiKey string) (Analysis, error) {
	log.Printf("→ Calling Anthropic API (model: %s)", anthropicModel)
	if apiKey == "" {
		return Analysis{}, fmt.Errorf("Anthropic API key is not set — add it in the launcher or config.json")
	}
	payload := map[string]interface{}{
		"model":      anthropicModel,
		"max_tokens": 4096,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": buildUserPrompt(resume, jobDescription)},
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Analysis{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Analysis{}, fmt.Errorf("Anthropic API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return Analysis{}, fmt.Errorf("Anthropic API error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Analysis{}, fmt.Errorf("failed to decode Anthropic response: %w", err)
	}
	if len(result.Content) == 0 {
		return Analysis{}, fmt.Errorf("empty response from Anthropic")
	}

	analysis, err := parseLLMResponse(result.Content[0].Text, jobDescription)
	if err != nil {
		log.Printf("✗ Anthropic response parse error: %v\nRaw: %s", err, result.Content[0].Text)
		return Analysis{}, err
	}
	log.Printf("✓ Anthropic response: score=%d adjusted=%d", analysis.Score, analysis.AdjustedScore)
	analysis.LLMProvider = "anthropic"
	analysis.LLMModel = anthropicModel
	return analysis, nil
}

// ── Ollama ────────────────────────────────────────────────────────────────────

func analyzeWithOllama(resume, jobDescription string, cfg config.Config) (Analysis, error) {
	log.Printf("→ Calling Ollama API (model: %s url: %s)", cfg.OllamaModel, cfg.OllamaBaseURL)
	payload := map[string]interface{}{
		"model": cfg.OllamaModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": buildUserPrompt(resume, jobDescription)},
		},
		"stream":  false,
		"options": map[string]interface{}{"temperature": 0.2},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", cfg.OllamaBaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Analysis{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(cfg.OllamaTimeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Analysis{}, fmt.Errorf(
			"cannot connect to Ollama at %s — make sure Ollama is running: ollama serve",
			cfg.OllamaBaseURL,
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return Analysis{}, fmt.Errorf("Ollama error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Analysis{}, fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	analysis, err := parseLLMResponse(result.Message.Content, jobDescription)
	if err != nil {
		log.Printf("✗ Ollama response parse error: %v\nRaw: %s", err, result.Message.Content)
		return Analysis{}, err
	}
	log.Printf("✓ Ollama response: score=%d adjusted=%d", analysis.Score, analysis.AdjustedScore)
	analysis.LLMProvider = "ollama"
	analysis.LLMModel = cfg.OllamaModel
	return analysis, nil
}

// ── Validation ────────────────────────────────────────────────────────────────

type ValidationResult struct {
	Valid  bool
	Errors []string
}

func validateLLMOutput(result Analysis, jd, resume string) ValidationResult {
	var errs []string

	if result.Score < 1 || result.Score > 5 {
		errs = append(errs, fmt.Sprintf("score %d out of range 1-5", result.Score))
	}

	if len(jd) > 500 && len(result.MatchedSkills) == 0 {
		errs = append(errs, "no matched skills despite rich job description")
	}

	matchedSet := map[string]bool{}
	for _, m := range result.MatchedSkills {
		matchedSet[strings.ToLower(m.Skill)] = true
	}
	for _, m := range result.MissingSkills {
		if matchedSet[strings.ToLower(m.Skill)] {
			errs = append(errs, fmt.Sprintf("skill %q appears in both matched and missing", m.Skill))
		}
	}

	validSeverities := map[string]bool{"blocker": true, "major": true, "minor": true}
	for _, m := range result.MissingSkills {
		if !validSeverities[m.Severity] {
			errs = append(errs, fmt.Sprintf("invalid severity %q for skill %q", m.Severity, m.Skill))
		}
	}

	if strings.TrimSpace(result.Reasoning) == "" {
		errs = append(errs, "empty reasoning")
	}

	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

func partialFallbackAnalysis() Analysis {
	// Score must be 1 (minimum valid) — 0 would fail validateLLMOutput's own check.
	return Analysis{
		Score:         1,
		AdjustedScore: 1,
		Reasoning:     "Analysis could not be completed reliably. Please try again or switch providers.",
		MatchedSkills: []MatchedSkill{},
		MissingSkills: []MissingSkill{},
	}
}

// ── Entry point ───────────────────────────────────────────────────────────────

// callLLMOnce dispatches to the right provider for a single attempt.
func callLLMOnce(resume, jd, provider string, cfg config.Config) (Analysis, error) {
	if provider == "ollama" {
		return analyzeWithOllama(resume, jd, cfg)
	}
	return analyzeWithAnthropic(resume, jd, cfg.AnthropicAPIKey)
}

// AnalyzeMatch runs the analysis with up to maxRetries attempts, validating
// each result. If all attempts fail, returns a partial fallback analysis.
func AnalyzeMatch(resume, jobDescription, provider string, cfg config.Config) (Analysis, error) {
	const maxRetries = 3
	var lastValidation ValidationResult

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("→ LLM retry %d/%d (prev errors: %v)", attempt, maxRetries-1, lastValidation.Errors)
		}

		result, err := callLLMOnce(resume, jobDescription, provider, cfg)
		if err != nil {
			log.Printf("✗ LLM attempt %d failed: %v", attempt+1, err)
			lastValidation = ValidationResult{Errors: []string{err.Error()}}
			continue
		}

		lastValidation = validateLLMOutput(result, jobDescription, resume)
		if lastValidation.Valid {
			result.RetryCount = attempt
			return result, nil
		}

		log.Printf("✗ LLM output validation failed (attempt %d): %v", attempt+1, lastValidation.Errors)
	}

	log.Printf("✗ All %d attempts failed, using fallback analysis", maxRetries)
	fallback := partialFallbackAnalysis()
	fallback.RetryCount = maxRetries
	fallback.UsedFallback = true
	fallback.ValidationErrors = strings.Join(lastValidation.Errors, "; ")
	return fallback, nil
}
