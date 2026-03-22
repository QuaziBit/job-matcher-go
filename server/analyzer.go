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
  "matched_skills": ["skill1", "skill2", ...],
  "missing_skills": [
    {"skill": "skill name", "severity": "blocker|major|minor"},
    ...
  ],
  "reasoning": "<2-4 sentence honest assessment>"
}

Severity definitions for missing_skills:
  blocker = eliminates candidacy entirely (e.g. required clearance, mandatory cert, minimum years not met)
  major   = significant gap that will hurt chances substantially
  minor   = nice-to-have or learnable gap that is unlikely to disqualify

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
	Score         int               `json:"score"`
	MatchedSkills []string          `json:"matched_skills"`
	MissingSkills []json.RawMessage `json:"missing_skills"`
	Reasoning     string            `json:"reasoning"`
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

	// Parse missing skills — handle both [{skill,severity}] and ["skill"]
	var missing []MissingSkill
	for _, raw := range resp.MissingSkills {
		var structured MissingSkill
		if err := json.Unmarshal(raw, &structured); err == nil && structured.Skill != "" {
			missing = append(missing, structured)
		} else {
			var flat string
			if err := json.Unmarshal(raw, &flat); err == nil && flat != "" {
				missing = append(missing, MissingSkill{Skill: flat, Severity: "minor"})
			}
		}
	}

	// Keyword detector pass — upgrade severities
	if jobDescription != "" {
		missing = keywordBoost(missing, jobDescription)
	}

	adjusted, breakdown := computeAdjustedScore(resp.Score, missing)

	return Analysis{
		Score:            resp.Score,
		AdjustedScore:    adjusted,
		PenaltyBreakdown: breakdown,
		MatchedSkills:    resp.MatchedSkills,
		MissingSkills:    missing,
		Reasoning:        resp.Reasoning,
	}, nil
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
		result[i] = MissingSkill{Skill: s.Skill, Severity: severity}
	}
	return result
}

// computeAdjustedScore applies the full penalty pipeline.
func computeAdjustedScore(rawScore int, missing []MissingSkill) (int, PenaltyBreakdown) {
	var blockers, majors, minors int
	for _, s := range missing {
		switch s.Severity {
		case "blocker":
			blockers++
		case "major":
			majors++
		default:
			minors++
		}
	}

	bp := min(blockers*2, 3)
	mp := min(majors*1, 2)
	mnp := min(minors/2, 1) // integer division: need 2 minors for -1
	cp := 0
	if len(missing) > 6 {
		cp = 1
	}

	total := bp + mp + mnp + cp
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
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── Anthropic ─────────────────────────────────────────────────────────────────

func analyzeWithAnthropic(resume, jobDescription, apiKey string) (Analysis, error) {
	log.Printf("→ Calling Anthropic API (model: %s)", anthropicModel)
	if apiKey == "" {
		return Analysis{}, fmt.Errorf("Anthropic API key is not set — add it in the launcher or config.json")
	}
	payload := map[string]interface{}{
		"model":      anthropicModel,
		"max_tokens": 1024,
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

// ── Entry point ───────────────────────────────────────────────────────────────

func AnalyzeMatch(resume, jobDescription, provider string, cfg config.Config) (Analysis, error) {
	if provider == "ollama" {
		return analyzeWithOllama(resume, jobDescription, cfg)
	}
	return analyzeWithAnthropic(resume, jobDescription, cfg.AnthropicAPIKey)
}
