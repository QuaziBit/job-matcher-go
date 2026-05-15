package server

// snippet_parser.go — Parse company ratings from pasted Google search snippets.
//
// The user searches Google for e.g. "Techstra Solutions reviews site:glassdoor.com OR site:bbb.org"
// and pastes the raw text from the results page. We send that text to the LLM to extract
// structured rating data — no web scraping, zero bot detection risk.

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

const snippetSystemPrompt = "You are a data extraction assistant. Extract company rating data from pasted Google search result text. Always respond with valid JSON only."

const snippetPromptTemplate = `Extract company rating information from the following Google search result text.

Return ONLY a valid JSON object in this exact format:
{
  "glassdoor_rating": <float or null>,
  "glassdoor_review_count": <integer or null>,
  "glassdoor_url": "<string or null>",
  "indeed_rating": <float or null>,
  "indeed_review_count": <integer or null>,
  "indeed_url": "<string or null>",
  "bbb_rating": "<string grade like A+ or null>",
  "bbb_url": "<string or null>",
  "linkedin_url": "<string or null>",
  "linkedin_employee_count": "<string or null>",
  "linkedin_founded": "<string or null>"
}

Rules:
- Extract ratings as floats (e.g. 4.3 not "4.3 stars")
- Extract review counts as integers (e.g. 702 not "(702)")
- Extract URLs as full https:// strings when visible
- Use null for any field not found
- If multiple Glassdoor or Indeed results appear, use the one with the most reviews

Search result text:
%s`

// SnippetResult holds parsed company data extracted from a Google snippet.
type SnippetResult struct {
	GlassdoorRating      *float64 `json:"glassdoor_rating"`
	GlassdoorReviewCount *int     `json:"glassdoor_review_count"`
	GlassdoorURL         *string  `json:"glassdoor_url"`
	IndeedRating         *float64 `json:"indeed_rating"`
	IndeedReviewCount    *int     `json:"indeed_review_count"`
	IndeedURL            *string  `json:"indeed_url"`
	BBBRating            *string  `json:"bbb_rating"`
	BBBURL               *string  `json:"bbb_url"`
	LinkedInURL          *string  `json:"linkedin_url"`
	LinkedInEmployees    *string  `json:"linkedin_employee_count"`
	LinkedInFounded      *string  `json:"linkedin_founded"`
}

// HasData returns true if at least one field was extracted.
func (s SnippetResult) HasData() bool {
	return s.GlassdoorRating != nil || s.IndeedRating != nil ||
		s.BBBRating != nil || s.GlassdoorURL != nil ||
		s.IndeedURL != nil || s.BBBURL != nil || s.LinkedInURL != nil
}

// ToMap converts SnippetResult to a map suitable for dbUpsertSnippetMeta.
func (s SnippetResult) ToMap() map[string]interface{} {
	m := map[string]interface{}{}
	if s.GlassdoorRating != nil {
		m["glassdoor_rating"] = *s.GlassdoorRating
	}
	if s.GlassdoorReviewCount != nil {
		m["glassdoor_review_count"] = *s.GlassdoorReviewCount
	}
	if s.GlassdoorURL != nil {
		m["glassdoor_url"] = *s.GlassdoorURL
	}
	if s.IndeedRating != nil {
		m["indeed_rating"] = *s.IndeedRating
	}
	if s.IndeedReviewCount != nil {
		m["indeed_review_count"] = *s.IndeedReviewCount
	}
	if s.IndeedURL != nil {
		m["indeed_url"] = *s.IndeedURL
	}
	if s.BBBRating != nil {
		m["bbb_rating"] = *s.BBBRating
	}
	if s.BBBURL != nil {
		m["bbb_url"] = *s.BBBURL
	}
	if s.LinkedInURL != nil {
		m["linkedin_url"] = *s.LinkedInURL
	}
	if s.LinkedInEmployees != nil {
		m["linkedin_employee_count"] = *s.LinkedInEmployees
	}
	if s.LinkedInFounded != nil {
		m["linkedin_founded"] = *s.LinkedInFounded
	}
	return m
}

// buildSnippetPrompt truncates text to 3000 chars and formats the prompt.
func buildSnippetPrompt(text string) string {
	if len(text) > 3000 {
		text = text[:3000]
	}
	return fmt.Sprintf(snippetPromptTemplate, text)
}

// parseSnippetResponse parses raw LLM JSON into a SnippetResult.
func parseSnippetResponse(raw string) (SnippetResult, error) {
	raw = stripThinking(raw)

	// Strip markdown fences
	raw = regexp.MustCompile("(?m)^```(?:json)?\\s*").ReplaceAllString(raw, "")
	raw = regexp.MustCompile("(?m)\\s*```$").ReplaceAllString(raw, "")

	// Find JSON object
	m := regexp.MustCompile(`(?s)\{.*\}`).FindString(raw)
	if m == "" {
		return SnippetResult{}, fmt.Errorf("no JSON object found in snippet response")
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(m), &data); err != nil {
		return SnippetResult{}, fmt.Errorf("failed to parse snippet JSON: %w", err)
	}

	var result SnippetResult

	// Float fields with validation
	for _, field := range []string{"glassdoor_rating", "indeed_rating"} {
		if v, ok := data[field]; ok && v != nil {
			if f, err := toFloat64(v); err == nil && f >= 1.0 && f <= 5.0 {
				f := f
				switch field {
				case "glassdoor_rating":
					result.GlassdoorRating = &f
				case "indeed_rating":
					result.IndeedRating = &f
				}
			}
		}
	}

	// Int fields
	for _, field := range []string{"glassdoor_review_count", "indeed_review_count"} {
		if v, ok := data[field]; ok && v != nil {
			if i, err := toInt(v); err == nil {
				i := i
				switch field {
				case "glassdoor_review_count":
					result.GlassdoorReviewCount = &i
				case "indeed_review_count":
					result.IndeedReviewCount = &i
				}
			}
		}
	}

	// String fields
	for _, field := range []string{"glassdoor_url", "indeed_url", "bbb_rating", "bbb_url",
		"linkedin_url", "linkedin_employee_count", "linkedin_founded"} {
		if v, ok := data[field]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "null" {
				s := s
				switch field {
				case "glassdoor_url":
					result.GlassdoorURL = &s
				case "indeed_url":
					result.IndeedURL = &s
				case "bbb_rating":
					result.BBBRating = &s
				case "bbb_url":
					result.BBBURL = &s
				case "linkedin_url":
					result.LinkedInURL = &s
				case "linkedin_employee_count":
					result.LinkedInEmployees = &s
				case "linkedin_founded":
					result.LinkedInFounded = &s
				}
			}
		}
	}

	return result, nil
}

// callSnippetLLM dispatches to the appropriate provider.
func callSnippetLLM(prompt string, provider string, model string, cfg config.Config) (string, error) {
	switch provider {
	case "anthropic":
		return callSnippetAnthropic(prompt, model, cfg)
	case "openai":
		return callSnippetOpenAI(prompt, model, cfg)
	case "gemini":
		return callSnippetGemini(prompt, model, cfg)
	case "ollama":
		return callSnippetOllama(prompt, model, cfg)
	default:
		return "", fmt.Errorf("unsupported provider for snippet parsing: %s", provider)
	}
}

func callSnippetAnthropic(prompt, model string, cfg config.Config) (string, error) {
	if cfg.AnthropicAPIKey == "" {
		return "", fmt.Errorf("Anthropic API key is not set")
	}
	m := snippetResolveModel("anthropic", model, cfg)
	log.Printf("→ snippet anthropic request: model=%s", m)
	payload := map[string]interface{}{
		"model":      m,
		"max_tokens": 400,
		"system":     snippetSystemPrompt,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.AnthropicAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Anthropic snippet request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Anthropic snippet API error %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response from Anthropic snippet call")
	}
	return result.Content[0].Text, nil
}

func callSnippetOpenAI(prompt, model string, cfg config.Config) (string, error) {
	if cfg.OpenAIAPIKey == "" {
		return "", fmt.Errorf("OpenAI API key is not set")
	}
	m := snippetResolveModel("openai", model, cfg)
	log.Printf("→ snippet openai request: model=%s", m)
	payload := map[string]interface{}{
		"model":       m,
		"max_tokens":  400,
		"temperature": 0.0,
		"messages": []map[string]string{
			{"role": "system", "content": snippetSystemPrompt},
			{"role": "user", "content": prompt},
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAIAPIKey)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenAI snippet request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI snippet API error %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response from OpenAI snippet call")
	}
	return result.Choices[0].Message.Content, nil
}

func callSnippetGemini(prompt, model string, cfg config.Config) (string, error) {
	if cfg.GeminiAPIKey == "" {
		return "", fmt.Errorf("Gemini API key is not set")
	}
	m := snippetResolveModel("gemini", model, cfg)
	log.Printf("→ snippet gemini request: model=%s", m)
	payload := map[string]interface{}{
		"system_instruction": map[string]interface{}{
			"parts": []map[string]string{{"text": snippetSystemPrompt}},
		},
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.0,
			"maxOutputTokens": 400,
		},
	}
	gc := payload["generationConfig"].(map[string]interface{})
	mergeGeminiAFCIntoGenerationConfig(gc)

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", m, cfg.GeminiAPIKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Gemini snippet request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gemini snippet API error %d: %s", resp.StatusCode, string(b))
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
		return "", err
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini snippet call")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

func callSnippetOllama(prompt, model string, cfg config.Config) (string, error) {
	m := snippetResolveModel("ollama", model, cfg)
	log.Printf("→ snippet ollama request: model=%s", m)
	payload := map[string]interface{}{
		"model": m,
		"messages": []map[string]string{
			{"role": "system", "content": snippetSystemPrompt},
			{"role": "user", "content": prompt},
		},
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.0,
			"num_predict": 400,
		},
	}
	if isThinkingModel(m) {
		payload["format"] = "json"
	}
	body, _ := json.Marshal(payload)
	ollamaURL := cfg.OllamaBaseURL + "/api/chat"
	timeout := time.Duration(cfg.OllamaTimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 600 * time.Second
	}
	req, err := http.NewRequest("POST", ollamaURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Ollama snippet request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama snippet error %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return stripThinking(result.Message.Content), nil
}

// snippetResolveModel picks the model to use for snippet parsing.
func snippetResolveModel(provider, model string, cfg config.Config) string {
	if model != "" {
		return model
	}
	switch provider {
	case "openai":
		return cfg.OpenAIModel
	case "gemini":
		return cfg.GeminiModel
	case "ollama":
		return cfg.OllamaModel
	default:
		return cfg.AnthropicModel
	}
}

// parseCompanySnippet is the main entry point — calls LLM and returns parsed data.
func parseCompanySnippet(text, provider, model string, cfg config.Config) (SnippetResult, error) {
	prompt := buildSnippetPrompt(text)
	log.Printf("→ snippet parse provider=%s text_len=%d", provider, len(text))
	raw, err := callSnippetLLM(prompt, provider, model, cfg)
	if err != nil {
		return SnippetResult{}, err
	}
	if cfg.ShowMoreLogs {
		log.Printf("→ snippet raw body:\n%s", raw)
	}
	result, err := parseSnippetResponse(raw)
	if err != nil {
		return SnippetResult{}, err
	}
	log.Printf("✓ snippet parse complete: fields=%d", len(result.ToMap()))
	return result, nil
}

// ── Type helpers ──────────────────────────────────────────────────────────────

func toFloat64(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case json.Number:
		return val.Float64()
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case json.Number:
		f, err := val.Float64()
		return int(f), err
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}
