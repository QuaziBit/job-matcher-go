package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuaziBit/job-matcher-go/config"
)

// showMoreLogs returns true when SHOW_MORE_LOGS env var is set to a truthy value.
// When true, raw chunk bodies (first 800 chars) are logged at INFO level.
func showMoreLogs() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SHOW_MORE_LOGS")))
	return v == "1" || v == "true" || v == "yes"
}

func analyzeWithAnthropic(resume, jobDescription string, cfg config.Config) (Analysis, error) {
	model := cfg.AnthropicModel
	if model == "" {
		model = "claude-opus-4-5"
	}
	log.Printf("→ Calling Anthropic API (model: %s mode: %s)", model, cfg.AnalysisMode)
	if cfg.AnthropicAPIKey == "" {
		return Analysis{}, fmt.Errorf("Anthropic API key is not set — add it in the launcher or config.json")
	}
	mcfg := getModeConfig(cfg)
	payload := map[string]interface{}{
		"model":      model,
		"max_tokens": mcfg.MaxTokens,
		"system":     buildSystemPrompt(mcfg, cfg.AnalysisMode, true),
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
	req.Header.Set("x-api-key", cfg.AnthropicAPIKey)
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

	analysis, err := parseLLMResponse(result.Content[0].Text, jobDescription, getModeConfig(cfg))
	if err != nil {
		rawPreview := result.Content[0].Text
		if len(rawPreview) > 1000 {
			rawPreview = rawPreview[:1000]
		}
		log.Printf("✗ Anthropic response parse error: %v\nRaw (first 1000 chars):\n%s", err, rawPreview)
		return Analysis{}, err
	}
	log.Printf("✓ Anthropic response: score=%d adjusted=%d", analysis.Score, analysis.AdjustedScore)
	if showMoreLogs() {
		preview := result.Content[0].Text
		if len(preview) > 800 {
			preview = preview[:800]
		}
		log.Printf("→ Anthropic raw body:\n%s", preview)
	}
	analysis.LLMProvider = "anthropic"
	analysis.LLMModel = model
	analysis.AnalysisMode = cfg.AnalysisMode
	return analysis, nil
}

// analyzeWithOllama delegates to callOllamaChunked which splits the request into
// 3-4 focused chunks instead of one large prompt.
func analyzeWithOllama(resume, jobDescription string, cfg config.Config) (Analysis, error) {
	log.Printf("→ Calling Ollama API (model: %s url: %s)", cfg.OllamaModel, cfg.OllamaBaseURL)
	analysis, err := callOllamaChunked(resume, jobDescription, cfg)
	if err != nil {
		return Analysis{}, err
	}
	log.Printf("✓ Ollama response: score=%d adjusted=%d", analysis.Score, analysis.AdjustedScore)
	analysis.LLMProvider = "ollama"
	analysis.LLMModel = cfg.OllamaModel
	return analysis, nil
}

// ── Chunked Ollama implementation ─────────────────────────────────────────────

// callChunk makes a single focused Ollama /api/chat request and returns the raw
// response content. Logs chunk name and response size. When SHOW_MORE_LOGS is
// set, also logs the first 800 chars of the raw body.
func callChunk(sysPrompt, userPrompt, model, baseURL string, numPredict int, chunkName string, timeout time.Duration) (string, error) {
	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": userPrompt},
		},
		"stream":  false,
		"options": map[string]interface{}{"temperature": 0.2, "num_predict": numPredict},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("chunk %s: build request: %w", chunkName, err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("chunk %s: request failed: %w", chunkName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("chunk %s: Ollama error %d: %s", chunkName, resp.StatusCode, string(b))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("chunk %s: decode failed: %w", chunkName, err)
	}

	raw := result.Message.Content
	log.Printf("→ chunk %s: %d chars", chunkName, len(raw))
	if showMoreLogs() {
		preview := raw
		if len(preview) > 800 {
			preview = preview[:800]
		}
		log.Printf("→ chunk %s raw body:\n%s", chunkName, preview)
	}
	return raw, nil
}

// callOllamaChunked splits the analysis into 3-4 focused requests:
//
//	fast/standard: chunk1 (score+reasoning, 350 tok)
//	               chunk2 (matched_skills, 800 tok)
//	               chunk3 (missing_skills, 800 tok)
//	detailed:      same + chunk4 (suggestions, 600 tok)
//	               chunk2 gets 1400 tok in detailed mode (#25)
//
// Chunk 1 failure is fatal (no score = no usable result).
// Chunks 2-4 degrade gracefully on failure (empty lists returned).
func callOllamaChunked(resume, jd string, cfg config.Config) (Analysis, error) {
	effectiveMode := capModeForModel(cfg.AnalysisMode, cfg.OllamaModel)
	mcfg := getModeConfigFor(effectiveMode)
	timeout := time.Duration(cfg.OllamaTimeoutSeconds) * time.Second

	log.Printf("→ Ollama chunked: model=%s mode=%s max_matched=%d suggestions=%v",
		cfg.OllamaModel, effectiveMode, mcfg.MaxMatched, mcfg.Suggestions)

	userPrompt := buildUserPrompt(resume, jd)

	// ── Chunk 1: score + reasoning ────────────────────────────────────────────
	raw1, err := callChunk(buildChunk1Prompt(effectiveMode), userPrompt, cfg.OllamaModel, cfg.OllamaBaseURL, 350, "1/score", timeout)
	if err != nil {
		log.Printf("⚠ cannot connect to Ollama at %s: %v", cfg.OllamaBaseURL, err)
		return Analysis{}, fmt.Errorf(
			"cannot connect to Ollama at %s — make sure Ollama is running: ollama serve",
			cfg.OllamaBaseURL,
		)
	}
	score, reasoning, err := parseScoreChunk(raw1)
	if err != nil {
		return Analysis{}, fmt.Errorf("chunk 1/score parse failed: %w", err)
	}
	log.Printf("→ chunk 1/score: score=%d reasoning=%d chars", score, len(reasoning))

	// ── Chunk 2: matched_skills ───────────────────────────────────────────────
	// #25: scale num_predict by mode — detailed mode needs more tokens for up to
	// 15 skills with full jd_snippet + resume_snippet.
	chunk2Predict := 800
	if effectiveMode == "detailed" {
		chunk2Predict = 1400
	}
	var matchedItems []json.RawMessage
	raw2, err := callChunk(buildChunk2Prompt(mcfg, effectiveMode), userPrompt, cfg.OllamaModel, cfg.OllamaBaseURL, chunk2Predict, "2/matched", timeout)
	if err != nil {
		log.Printf("⚠ chunk 2/matched failed: %v — using empty matched_skills", err)
	} else if items, perr := parseChunkArray(raw2, "matched_skills", "2/matched"); perr != nil {
		log.Printf("⚠ chunk 2/matched parse: %v — using empty matched_skills", perr)
	} else {
		matchedItems = items
	}
	log.Printf("→ chunk 2/matched: %d items", len(matchedItems))

	// ── Chunk 3: missing_skills ───────────────────────────────────────────────
	var missingItems []json.RawMessage
	raw3, err := callChunk(buildChunk3Prompt(mcfg, effectiveMode), userPrompt, cfg.OllamaModel, cfg.OllamaBaseURL, 800, "3/missing", timeout)
	if err != nil {
		log.Printf("⚠ chunk 3/missing failed: %v — using empty missing_skills", err)
	} else if items, perr := parseChunkArray(raw3, "missing_skills", "3/missing"); perr != nil {
		log.Printf("⚠ chunk 3/missing parse: %v — using empty missing_skills", perr)
	} else {
		missingItems = items
	}
	log.Printf("→ chunk 3/missing: %d items", len(missingItems))

	// ── Chunk 4: suggestions (detailed only) ──────────────────────────────────
	var suggestItems []json.RawMessage
	if effectiveMode == "detailed" && mcfg.Suggestions {
		raw4, err := callChunk(buildChunk4Prompt(), userPrompt, cfg.OllamaModel, cfg.OllamaBaseURL, 600, "4/suggestions", timeout)
		if err != nil {
			log.Printf("⚠ chunk 4/suggestions failed: %v — skipping suggestions", err)
		} else if items, perr := parseChunkArray(raw4, "suggestions", "4/suggestions"); perr != nil {
			log.Printf("⚠ chunk 4/suggestions parse: %v — skipping suggestions", perr)
		} else {
			suggestItems = items
		}
		log.Printf("→ chunk 4/suggestions: %d items", len(suggestItems))
	}

	// ── Merge chunks ──────────────────────────────────────────────────────────
	matched := buildMatchedSkills(matchedItems, mcfg)
	missing := buildMissingSkills(missingItems, jd, mcfg)
	adjusted, breakdown := computeAdjustedScore(score, missing)

	var suggestions []ResumeSuggestion
	if mcfg.Suggestions {
		suggestions = buildSuggestions(suggestItems)
	}

	return Analysis{
		Score:            score,
		AdjustedScore:    adjusted,
		PenaltyBreakdown: breakdown,
		MatchedSkills:    matched,
		MissingSkills:    missing,
		Reasoning:        reasoning,
		Suggestions:      suggestions,
		AnalysisMode:     effectiveMode,
	}, nil
}

// ── LLM output correction and validation ─────────────────────────────────────

type ValidationResult struct {
	Valid  bool
	Errors []string
}

// autoCorrectLLMOutput fixes minor model errors in-place without burning retries.
// Returns a list of corrections made for logging.
func autoCorrectLLMOutput(a *Analysis) []string {
	var corrections []string

	// Remove missing skills that also appear in matched skills
	// (llama3.1:8b detailed mode duplicate issue)
	matchedSet := map[string]bool{}
	for _, m := range a.MatchedSkills {
		matchedSet[strings.ToLower(m.Skill)] = true
	}
	var cleanMissing []MissingSkill
	for _, m := range a.MissingSkills {
		if matchedSet[strings.ToLower(m.Skill)] {
			corrections = append(corrections, fmt.Sprintf("removed duplicate %q from missing_skills", m.Skill))
		} else {
			cleanMissing = append(cleanMissing, m)
		}
	}
	if len(cleanMissing) != len(a.MissingSkills) {
		a.MissingSkills = cleanMissing
	}

	// Substitute default reasoning when empty (llama3.2:3b standard mode issue)
	if strings.TrimSpace(a.Reasoning) == "" {
		a.Reasoning = "Analysis completed. Please review the skill breakdown above."
		corrections = append(corrections, "substituted default reasoning (was empty)")
	}

	return corrections
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

	// Note: severity and requirement_type are normalized in parseLLMResponse
	// before validation runs — non-standard values are already mapped.

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

func analyzeWithOpenAI(resume, jobDescription string, cfg config.Config) (Analysis, error) {
	model := cfg.OpenAIModel
	if model == "" {
		model = "gpt-4o-mini"
	}
	log.Printf("→ Calling OpenAI API (model: %s mode: %s)", model, cfg.AnalysisMode)
	if cfg.OpenAIAPIKey == "" {
		return Analysis{}, fmt.Errorf("OpenAI API key is not set — add it in the launcher or config.json")
	}
	mcfg := getModeConfig(cfg)
	payload := map[string]interface{}{
		"model":      model,
		"max_tokens": mcfg.MaxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": buildSystemPrompt(mcfg, cfg.AnalysisMode, true)},
			{"role": "user", "content": buildUserPrompt(resume, jobDescription)},
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Analysis{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAIAPIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Analysis{}, fmt.Errorf("OpenAI API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return Analysis{}, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Analysis{}, fmt.Errorf("failed to decode OpenAI response: %w", err)
	}
	if len(result.Choices) == 0 {
		return Analysis{}, fmt.Errorf("empty response from OpenAI")
	}

	analysis, err := parseLLMResponse(result.Choices[0].Message.Content, jobDescription, getModeConfig(cfg))
	if err != nil {
		rawPreview := result.Choices[0].Message.Content
		if len(rawPreview) > 1000 {
			rawPreview = rawPreview[:1000]
		}
		log.Printf("✗ OpenAI response parse error: %v\nRaw (first 1000 chars):\n%s", err, rawPreview)
		return Analysis{}, err
	}
	log.Printf("✓ OpenAI response: score=%d adjusted=%d", analysis.Score, analysis.AdjustedScore)
	if showMoreLogs() {
		preview := result.Choices[0].Message.Content
		if len(preview) > 800 {
			preview = preview[:800]
		}
		log.Printf("→ OpenAI raw body:\n%s", preview)
	}
	analysis.LLMProvider = "openai"
	analysis.LLMModel = model
	analysis.AnalysisMode = cfg.AnalysisMode
	return analysis, nil
}

func analyzeWithGemini(resume, jobDescription string, cfg config.Config) (Analysis, error) {
	model := cfg.GeminiModel
	if model == "" {
		model = "gemini-2.5-flash"
	}
	log.Printf("→ Calling Gemini API (model: %s mode: %s)", model, cfg.AnalysisMode)
	if cfg.GeminiAPIKey == "" {
		return Analysis{}, fmt.Errorf("Gemini API key is not set — add it in the launcher or config.json")
	}
	mcfg := getModeConfig(cfg)

	type geminiPart struct {
		Text string `json:"text"`
	}
	type geminiContent struct {
		Role  string      `json:"role"`
		Parts []geminiPart `json:"parts"`
	}
	payload := map[string]interface{}{
		"system_instruction": map[string]interface{}{
			"parts": []geminiPart{{Text: buildSystemPrompt(mcfg, cfg.AnalysisMode, true)}},
		},
		"contents": []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: buildUserPrompt(resume, jobDescription)}}},
		},
		"generationConfig": map[string]interface{}{
			// Do not cap maxOutputTokens for Gemini — thinking models (gemini-2.5-flash etc.)
			// consume tokens internally before visible output, so a hard cap starves the
			// response and causes it to return a truncated/fast-style result.
			"temperature": 0.2,
		},
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.GeminiAPIKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return Analysis{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Analysis{}, fmt.Errorf("Gemini API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return Analysis{}, fmt.Errorf("Gemini API error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Analysis{}, fmt.Errorf("failed to decode Gemini response: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return Analysis{}, fmt.Errorf("empty response from Gemini")
	}

	text := result.Candidates[0].Content.Parts[0].Text
	analysis, err := parseLLMResponse(text, jobDescription, getModeConfig(cfg))
	if err != nil {
		rawPreview := text
		if len(rawPreview) > 1000 {
			rawPreview = rawPreview[:1000]
		}
		log.Printf("✗ Gemini response parse error: %v\nRaw (first 1000 chars):\n%s", err, rawPreview)
		return Analysis{}, err
	}
	log.Printf("✓ Gemini response: score=%d adjusted=%d", analysis.Score, analysis.AdjustedScore)
	if showMoreLogs() {
		preview := text
		if len(preview) > 800 {
			preview = preview[:800]
		}
		log.Printf("→ Gemini raw body:\n%s", preview)
	}
	analysis.LLMProvider = "gemini"
	analysis.LLMModel = model
	analysis.AnalysisMode = cfg.AnalysisMode
	return analysis, nil
}

// callLLMOnce dispatches to the right provider for a single attempt.
func callLLMOnce(resume, jd, provider string, cfg config.Config) (Analysis, error) {
	switch provider {
	case "ollama":
		return analyzeWithOllama(resume, jd, cfg)
	case "openai":
		return analyzeWithOpenAI(resume, jd, cfg)
	case "gemini":
		return analyzeWithGemini(resume, jd, cfg)
	default:
		return analyzeWithAnthropic(resume, jd, cfg)
	}
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

		if corrections := autoCorrectLLMOutput(&result); len(corrections) > 0 {
			log.Printf("→ auto-corrected LLM output: %v", corrections)
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
