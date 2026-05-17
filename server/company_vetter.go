package server

// company_vetter.go — LLM-based company legitimacy vetting.
//
// Only company names and publicly crawled data (BBB, Glassdoor, LinkedIn)
// are sent to the LLM provider. No personal data is ever transmitted.
//
// PRIVACY DECISION: LLM recruiter vetting is deliberately NOT implemented.
// Sending recruiter name/email/phone to cloud LLM providers would expose
// personal data. MX domain checks (mx_validator.go) are the privacy-safe
// recruiter signal: only the domain (e.g. abbtech.com) ever leaves the
// machine — never the full email address or any personal identifiers.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuaziBit/job-matcher-go/config"
)

// VETTING_CACHE_TTL_DAYS matches Python analyzer.company_vetter.CACHE_TTL_DAYS.
const VETTING_CACHE_TTL_DAYS = 7

// validRiskLevels matches Python RISK_LEVELS.
var validRiskLevels = map[string]bool{
	"low":     true,
	"medium":  true,
	"high":    true,
	"unknown": true,
}

// vettingSystemPrompt matches Python _call_vetting_llm `system` (non-thinking path).
const vettingSystemPrompt = "You are a job-search safety analyst. Always respond with valid JSON only."

// VettingResult matches the vetting API / Python vet_company return shape.
type VettingResult struct {
	RiskLevel    string   `json:"risk_level"`
	Assessment   string   `json:"assessment"`
	Signals      []string `json:"signals"`
	Company      string   `json:"company"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
}

var (
	vetFenceLeading  = regexp.MustCompile("(?m)^```(?:json)?\\s*")
	vetFenceTrailing = regexp.MustCompile("(?m)\\s*```$")
	// Python: re.search(r"\{.*\}", raw, re.DOTALL) — greedy first { to last }
	vetJSONGreedy = regexp.MustCompile(`(?s)\{.*\}`)
)

// buildCompanyPrompt mirrors Python build_company_prompt exactly.
func buildCompanyPrompt(companyName string, meta map[string]interface{}) string {
	lines := []string{fmt.Sprintf("Company name: %s", companyName)}

	bbbRating := strings.TrimSpace(metaString(meta, "bbb_rating"))
	bbbURL := strings.TrimSpace(metaString(meta, "bbb_url"))
	if bbbRating != "" {
		lines = append(lines, fmt.Sprintf("BBB rating: %s", bbbRating))
	} else if bbbURL != "" {
		lines = append(lines, "BBB: listed but no rating available")
	} else {
		lines = append(lines, "BBB: no listing found")
	}

	gdRating, hasGDRating := metaTruthyGlassdoorRating(meta)
	gdReviews, hasGDReviews := metaTruthyGlassdoorReviews(meta)
	gdURL := strings.TrimSpace(metaString(meta, "glassdoor_url"))
	if hasGDRating {
		revStr := ""
		if hasGDReviews {
			revStr = fmt.Sprintf(" (%d reviews)", gdReviews)
		}
		lines = append(lines, fmt.Sprintf("Glassdoor rating: %s/5%s", formatRating(gdRating), revStr))
	} else if gdURL != "" {
		lines = append(lines, "Glassdoor: listed but no rating available")
	} else {
		lines = append(lines, "Glassdoor: no listing found")
	}

	// Indeed
	inRatingRaw, hasINRating := metaTruthyIndeedRating(meta)
	inReviewsRaw, hasINReviews := metaTruthyIndeedReviews(meta)
	inURL := strings.TrimSpace(metaString(meta, "indeed_url"))
	if hasINRating {
		revStr := ""
		if hasINReviews {
			revStr = fmt.Sprintf(" (%d reviews)", inReviewsRaw)
		}
		lines = append(lines, fmt.Sprintf("Indeed rating: %s/5%s", formatRating(inRatingRaw), revStr))
	} else if inURL != "" {
		lines = append(lines, "Indeed: listed but no rating available")
	} else {
		lines = append(lines, "Indeed: no listing found")
	}

	liEmployees := strings.TrimSpace(metaString(meta, "linkedin_employee_count"))
	liFounded := strings.TrimSpace(metaString(meta, "linkedin_founded"))
	liURL := strings.TrimSpace(metaString(meta, "linkedin_url"))
	if liEmployees != "" || liFounded != "" {
		var liParts []string
		if liEmployees != "" {
			liParts = append(liParts, fmt.Sprintf("employees: %s", liEmployees))
		}
		if liFounded != "" {
			liParts = append(liParts, fmt.Sprintf("founded: %s", liFounded))
		}
		lines = append(lines, fmt.Sprintf("LinkedIn: %s", strings.Join(liParts, ", ")))
	} else if liURL != "" {
		lines = append(lines, "LinkedIn: listed but no details available")
	} else {
		lines = append(lines, "LinkedIn: no company page found")
	}

	dataSection := strings.Join(lines, "\n")

	return "You are a job-search safety analyst. A job seeker wants to know if a company is legitimate before applying.\n\n" +
		"Analyze the following company data and assess the legitimacy risk for a job applicant.\n\n" +
		dataSection + "\n\n" +
		"Respond with ONLY a valid JSON object in this exact format:\n" +
		"{\n" +
		`  "risk_level": "low" | "medium" | "high" | "unknown",` + "\n" +
		`  "assessment": "2-3 sentence plain-English summary of legitimacy signals",` + "\n" +
		`  "signals": ["key signal 1", "key signal 2"]` + "\n" +
		"}\n\n" +
		"Risk level guidance:\n" +
		"- low: established company, good ratings, strong online presence\n" +
		"- medium: limited data, mixed signals, or minor concerns\n" +
		"- high: no online presence, very poor ratings, suspicious patterns\n" +
		"- unknown: insufficient data to make a determination\n\n" +
		"Respond with JSON only. No prose, no markdown fences."
}

func formatRating(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-" {
		s = fmt.Sprintf("%g", f)
	}
	return s
}

func metaTruthyGlassdoorRating(meta map[string]interface{}) (float64, bool) {
	if meta == nil {
		return 0, false
	}
	v, ok := meta["glassdoor_rating"]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, t != 0
	case int:
		return float64(t), t != 0
	case int64:
		return float64(t), t != 0
	case json.Number:
		f, err := t.Float64()
		return f, err == nil && f != 0
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil && f != 0
	default:
		return 0, false
	}
}

func metaTruthyGlassdoorReviews(meta map[string]interface{}) (int, bool) {
	if meta == nil {
		return 0, false
	}
	v, ok := meta["glassdoor_review_count"]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int(t), t != 0
	case int:
		return t, t != 0
	case int64:
		return int(t), t != 0
	case json.Number:
		i64, err := t.Int64()
		return int(i64), err == nil && i64 != 0
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		n, err := strconv.Atoi(s)
		return n, err == nil && n != 0
	default:
		return 0, false
	}
}

func metaTruthyIndeedRating(meta map[string]interface{}) (float64, bool) {
	if meta == nil {
		return 0, false
	}
	v, ok := meta["indeed_rating"]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, t != 0
	case int:
		return float64(t), t != 0
	case int64:
		return float64(t), t != 0
	case json.Number:
		f, err := t.Float64()
		return f, err == nil && f != 0
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil && f != 0
	default:
		return 0, false
	}
}

func metaTruthyIndeedReviews(meta map[string]interface{}) (int, bool) {
	if meta == nil {
		return 0, false
	}
	v, ok := meta["indeed_review_count"]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int(t), t != 0
	case int:
		return t, t != 0
	case int64:
		return int(t), t != 0
	case json.Number:
		i64, err := t.Int64()
		return int(i64), err == nil && i64 != 0
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		n, err := strconv.Atoi(s)
		return n, err == nil && n != 0
	default:
		return 0, false
	}
}

func metaString(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// parseVettingResponse mirrors Python _parse_vetting_response.
func parseVettingResponse(raw string, companyName string) (VettingResult, error) {
	raw = strings.TrimSpace(stripThinking(raw))

	raw = vetFenceLeading.ReplaceAllString(raw, "")
	raw = vetFenceTrailing.ReplaceAllString(raw, "")

	m := vetJSONGreedy.FindString(raw)
	if m == "" {
		return VettingResult{}, fmt.Errorf("no JSON object found in response: %.200q", raw)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(m), &data); err != nil {
		return VettingResult{}, fmt.Errorf("vetting JSON parse failed: %w", err)
	}

	rl := strings.ToLower(strings.TrimSpace(fmt.Sprint(data["risk_level"])))
	if rl == "" {
		rl = "unknown"
	}
	if !validRiskLevels[rl] {
		rl = "unknown"
	}

	assessment := ""
	if v, ok := data["assessment"]; ok && v != nil {
		assessment = strings.TrimSpace(fmt.Sprint(v))
	}
	if assessment == "" {
		assessment = "No assessment available."
	}

	signalsRaw := data["signals"]
	signals := []string{}
	switch t := signalsRaw.(type) {
	case []interface{}:
		for _, s := range t {
			s := strings.TrimSpace(fmt.Sprint(s))
			if s != "" {
				signals = append(signals, s)
			}
		}
	case []string:
		for _, s := range t {
			s = strings.TrimSpace(s)
			if s != "" {
				signals = append(signals, s)
			}
		}
	default:
		if signalsRaw != nil {
			s := strings.TrimSpace(fmt.Sprint(signalsRaw))
			if s != "" {
				signals = []string{s}
			}
		}
	}

	return VettingResult{
		RiskLevel:  rl,
		Assessment: assessment,
		Signals:    signals,
		Company:    companyName,
	}, nil
}

func vettingResolveModel(provider, model string, cfg config.Config) string {
	if strings.TrimSpace(model) != "" {
		return strings.TrimSpace(model)
	}
	switch provider {
	case "anthropic":
		if cfg.AnthropicModel != "" {
			return cfg.AnthropicModel
		}
		return "claude-opus-4-5"
	case "openai":
		if cfg.OpenAIModel != "" {
			return cfg.OpenAIModel
		}
		return "gpt-4o-mini"
	case "gemini":
		if cfg.GeminiModel != "" {
			return cfg.GeminiModel
		}
		return "gemini-2.5-flash"
	case "ollama":
		if cfg.OllamaModel != "" {
			return cfg.OllamaModel
		}
		return "llama3.1:8b"
	default:
		return strings.TrimSpace(model)
	}
}

// callVettingLLM mirrors Python _call_vetting_llm (HTTP for cloud parity with salary.go).
func callVettingLLM(prompt string, provider string, model string, cfg config.Config) (string, error) {
	switch provider {
	case "anthropic":
		return callVettingAnthropic(prompt, model, cfg)
	case "openai":
		return callVettingOpenAI(prompt, model, cfg)
	case "gemini":
		return callVettingGemini(prompt, model, cfg)
	case "ollama":
		return callVettingOllama(prompt, model, cfg)
	default:
		return "", fmt.Errorf("unsupported provider for company vetting: %s", provider)
	}
}

func callVettingAnthropic(prompt string, model string, cfg config.Config) (string, error) {
	if cfg.AnthropicAPIKey == "" {
		return "", fmt.Errorf("Anthropic API key is not set")
	}
	m := vettingResolveModel("anthropic", model, cfg)
	log.Printf("→ vetting anthropic request: model=%s", m)

	payload := map[string]interface{}{
		"model":      m,
		"max_tokens": 300,
		"system":     vettingSystemPrompt,
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
		return "", fmt.Errorf("Anthropic vetting request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Anthropic vetting API error %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode Anthropic vetting response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response from Anthropic vetting call")
	}
	text := result.Content[0].Text
	log.Printf("→ vetting anthropic response (%d chars)", len(text))
	if showMoreLogs() {
		log.Printf("→ vetting anthropic raw body:\n%s", text)
	}
	return text, nil
}

func callVettingOpenAI(prompt string, model string, cfg config.Config) (string, error) {
	if cfg.OpenAIAPIKey == "" {
		return "", fmt.Errorf("OpenAI API key is not set")
	}
	m := vettingResolveModel("openai", model, cfg)
	log.Printf("→ vetting openai request: model=%s", m)

	payload := map[string]interface{}{
		"model":       m,
		"max_tokens":  300,
		"temperature": 0.1,
		"messages": []map[string]string{
			{"role": "system", "content": vettingSystemPrompt},
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
		return "", fmt.Errorf("OpenAI vetting request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI vetting API error %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode OpenAI vetting response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response from OpenAI vetting call")
	}
	text := result.Choices[0].Message.Content
	log.Printf("→ vetting openai response (%d chars)", len(text))
	if showMoreLogs() {
		log.Printf("→ vetting openai raw body:\n%s", text)
	}
	return text, nil
}

func callVettingGemini(prompt string, model string, cfg config.Config) (string, error) {
	if cfg.GeminiAPIKey == "" {
		return "", fmt.Errorf("Gemini API key is not set")
	}
	m := vettingResolveModel("gemini", model, cfg)
	log.Printf("→ vetting gemini request: model=%s", m)

	type part struct {
		Text string `json:"text"`
	}
	payload := map[string]interface{}{
		"systemInstruction": map[string]interface{}{
			"parts": []part{{Text: vettingSystemPrompt}},
		},
		"contents": []map[string]interface{}{
			{"role": "user", "parts": []part{{Text: prompt}}},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.1,
		},
	}
	mergeGeminiAFCIntoGenerationConfig(payload["generationConfig"].(map[string]interface{}))
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
		return "", fmt.Errorf("Gemini vetting request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gemini vetting API error %d: %s", resp.StatusCode, string(b))
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
		return "", fmt.Errorf("failed to decode Gemini vetting response: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini vetting call")
	}
	text := result.Candidates[0].Content.Parts[0].Text
	log.Printf("→ vetting gemini response (%d chars)", len(text))
	if showMoreLogs() {
		log.Printf("→ vetting gemini raw body:\n%s", text)
	}
	return text, nil
}

func callVettingOllama(prompt string, model string, cfg config.Config) (string, error) {
	m := vettingResolveModel("ollama", model, cfg)
	thinking := isThinkingModel(m)
	system := vettingSystemPrompt
	if thinking {
		system = "CRITICAL: Respond with ONLY a valid JSON object. " +
			"No prose, no markdown, no explanations. Start with '{' end with '}'.\n\n" + system
	}

	log.Printf("→ vetting ollama request: model=%s", m)

	numPredict := 300
	if thinking {
		numPredict = 600
	}
	payload := map[string]interface{}{
		"model": m,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": prompt},
		},
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.1,
			"num_predict": numPredict,
			"think":       false,
		},
	}
	if thinking {
		payload["format"] = "json"
	}
	body, _ := json.Marshal(payload)

	timeout := time.Duration(cfg.OllamaTimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = time.Duration(600) * time.Second
	}
	base := strings.TrimRight(cfg.OllamaBaseURL, "/")
	req, err := http.NewRequest("POST", base+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Ollama vetting request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama vetting API error %d: %s", resp.StatusCode, string(b))
	}
	respBody, _ := io.ReadAll(resp.Body)
	cnt, thinkingOut := ollamaMessageContent(respBody)
	raw := cnt
	if raw == "" {
		raw = thinkingOut
	}
	raw = strings.TrimSpace(stripThinking(raw))

	var meta struct {
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}
	_ = json.Unmarshal(respBody, &meta)
	log.Printf("→ vetting ollama response (%d chars) prompt_tokens=%d output_tokens=%d",
		len(raw), meta.PromptEvalCount, meta.EvalCount)
	if showMoreLogs() {
		log.Printf("→ vetting ollama raw body:\n%s", raw)
	}
	return raw, nil
}

// vetCompany mirrors Python vet_company (sync; Go server uses blocking HTTP).
func vetCompany(companyName string, meta map[string]interface{}, provider string, model string, cfg config.Config) (VettingResult, error) {
	prompt := buildCompanyPrompt(companyName, meta)
	log.Printf("→ vetting company=%q provider=%s", companyName, provider)

	raw, err := callVettingLLM(prompt, provider, model, cfg)
	if err != nil {
		return VettingResult{}, err
	}
	res, err := parseVettingResponse(raw, companyName)
	if err != nil {
		log.Printf("✗ company vetting parse failed for %q: %v raw_preview=%.400q", companyName, err, raw)
		return VettingResult{}, err
	}
	res.Provider = provider
	res.Model = vettingResolveModel(provider, model, cfg)

	log.Printf("✓ vetting complete: company=%q risk=%s provider=%s", res.Company, res.RiskLevel, res.Provider)
	return res, nil
}
