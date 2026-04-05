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

// salaryIncompatibleModels are rejected before any LLM call.
// Uses partial match — "gemma3" catches gemma3:27b, gemma3:12b, etc.
var salaryIncompatibleModels = []string{"gemma3"}

// salaryPatterns detects explicit salary information in JDs.
// Uses specific patterns to avoid false positives on vague boilerplate like
// "compensation determined by contract wage rates".
var salaryPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\$\s*\d`),
	regexp.MustCompile(`(?i)\bbase salary\b`),
	regexp.MustCompile(`(?i)\bsalary\s+(range|is|of|:)`),
	regexp.MustCompile(`(?i)\bpay range\b`),
	regexp.MustCompile(`(?i)\bcompensation range\b`),
	regexp.MustCompile(`(?i)\btotal compensation\b`),
	regexp.MustCompile(`(?i)\bannual salary\b`),
	regexp.MustCompile(`(?i)\bhourly rate\b`),
	regexp.MustCompile(`(?i)\bper hour\b`),
	regexp.MustCompile(`\d+k\s*-\s*\d+k`),
	regexp.MustCompile(`\d+,\d{3}`),
}

// jobHasSalary returns true if the JD contains explicit salary information.
func jobHasSalary(desc string) bool {
	for _, p := range salaryPatterns {
		if p.MatchString(desc) {
			return true
		}
	}
	return false
}

// buildSalaryPrompt builds the LLM prompt for salary estimation or extraction.
// Uses first 4000 chars of JD to avoid truncating salary info in long JDs.
func buildSalaryPrompt(title, company, location, desc string) string {
	excerpt := desc
	if len(excerpt) > 4000 {
		excerpt = excerpt[:4000]
	}
	schema := `{"min": <integer>, "max": <integer>, "currency": "USD", "period": "year", "confidence": "high|medium|low", "signals": ["reason1", "reason2"]}`
	rules := strings.Join([]string{
		"ALWAYS provide a best-guess range — never use 0 for min or max.",
		"If uncertain, use a wide range with confidence=low (e.g. 60000-120000).",
		"Respond ONLY with valid JSON matching the schema. No prose, no markdown, no code fences.",
	}, "\n")
	return strings.Join([]string{
		"Estimate the annual salary range for this job posting.",
		"Job title: " + title,
		"Company: " + company,
		"Location: " + location,
		"Job description excerpt:\n" + excerpt,
		"Required JSON schema:\n" + schema,
		"Rules:\n" + rules,
	}, "\n\n")
}

// toFloatVal safely converts interface{} to float64 (nil → 0).
func toFloatVal(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	}
	return 0
}

// parseSalaryResponse parses and validates an LLM salary JSON response.
// Handles markdown fences, null/None values, and reversed ranges.
func parseSalaryResponse(raw, model string) (map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	for _, fence := range []string{"```json", "```"} {
		raw = strings.TrimPrefix(raw, fence)
	}
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("%s salary parse failed: %w", model, err)
	}

	minVal := int(toFloatVal(data["min"]))
	maxVal := int(toFloatVal(data["max"]))
	if minVal == 0 && maxVal == 0 {
		return nil, fmt.Errorf("%s could not estimate salary — no valid range returned. Try again or switch to Anthropic for more reliable results.", model)
	}
	if minVal > maxVal {
		minVal, maxVal = maxVal, minVal
	}
	data["min"] = minVal
	data["max"] = maxVal

	conf := fmt.Sprintf("%v", data["confidence"])
	switch conf {
	case "high", "medium", "low":
	default:
		data["confidence"] = "low"
	}
	if _, ok := data["currency"]; !ok {
		data["currency"] = "USD"
	}
	if _, ok := data["period"]; !ok {
		data["period"] = "year"
	}
	if _, ok := data["signals"]; !ok {
		data["signals"] = []interface{}{}
	}
	return data, nil
}

// callSalaryLLM dispatches the salary prompt to the configured provider.
// Returns raw response text, model name used, and any error.
func callSalaryLLM(prompt, provider string, temperature float64, cfg config.Config) (string, string, error) {
	switch provider {
	case "anthropic":
		return callSalaryAnthropic(prompt, temperature, cfg)
	case "ollama":
		return callSalaryOllama(prompt, temperature, cfg)
	default:
		return "", "", fmt.Errorf("unsupported provider for salary estimation: %s", provider)
	}
}

func callSalaryAnthropic(prompt string, temperature float64, cfg config.Config) (string, string, error) {
	if cfg.AnthropicAPIKey == "" {
		return "", "", fmt.Errorf("Anthropic API key is not set")
	}
	model := cfg.AnthropicModel
	if model == "" {
		model = "claude-opus-4-5"
	}
	log.Printf("→ salary anthropic request: model=%s temperature=%.1f", model, temperature)

	payload := map[string]interface{}{
		"model":      model,
		"max_tokens": 400,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", model, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.AnthropicAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", model, fmt.Errorf("Anthropic salary request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", model, fmt.Errorf("Anthropic salary API error %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		Content []struct{ Text string `json:"text"` } `json:"content"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", model, fmt.Errorf("failed to decode Anthropic salary response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", model, fmt.Errorf("empty response from Anthropic salary call")
	}
	log.Printf("→ salary anthropic response (%d chars) input_tokens=%d output_tokens=%d",
		len(result.Content[0].Text), result.Usage.InputTokens, result.Usage.OutputTokens)
	return result.Content[0].Text, model, nil
}

func callSalaryOllama(prompt string, temperature float64, cfg config.Config) (string, string, error) {
	model := cfg.OllamaModel
	if model == "" {
		model = "llama3.1:8b"
	}
	log.Printf("→ salary ollama request: model=%s temperature=%.1f", model, temperature)

	payload := map[string]interface{}{
		"model":    model,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
		"stream":   false,
		"options":  map[string]interface{}{"temperature": temperature, "num_predict": 400},
	}
	body, _ := json.Marshal(payload)

	timeout := time.Duration(cfg.OllamaTimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	req, err := http.NewRequest("POST", cfg.OllamaBaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", model, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", model, fmt.Errorf("Ollama salary request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", model, fmt.Errorf("Ollama salary API error %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		Message         struct{ Content string `json:"content"` } `json:"message"`
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", model, fmt.Errorf("failed to decode Ollama salary response: %w", err)
	}
	log.Printf("→ salary ollama response (%d chars) prompt_tokens=%d output_tokens=%d",
		len(result.Message.Content), result.PromptEvalCount, result.EvalCount)
	return result.Message.Content, model, nil
}

// estimateSalary estimates salary for a job where the JD does not post salary.
// skipCheck=true bypasses the jobHasSalary guard (used internally for fallback).
func estimateSalary(title, company, location, desc, provider string, skipCheck bool, cfg config.Config) (map[string]interface{}, error) {
	if !skipCheck && jobHasSalary(desc) {
		return nil, fmt.Errorf("JD contains salary information — use extract instead")
	}
	prompt := buildSalaryPrompt(title, company, location, desc)
	var lastErr error
	var lastModel string
	for attempt := 0; attempt < 2; attempt++ {
		raw, model, err := callSalaryLLM(prompt, provider, 0.2, cfg)
		lastModel = model
		if err != nil {
			log.Printf("→ %s salary LLM call failed (attempt %d): %v", model, attempt+1, err)
			lastErr = err
			continue
		}
		result, err := parseSalaryResponse(raw, model)
		if err != nil {
			log.Printf("→ %s salary parse failed (attempt %d), raw: %.300s", model, attempt+1, raw)
			lastErr = err
			continue
		}
		result["llm_provider"] = provider
		result["llm_model"] = model
		result["source"] = "estimated"
		return result, nil
	}
	_ = lastErr
	return nil, fmt.Errorf("%s could not estimate salary after 2 attempts. Try again or switch to a different provider.", lastModel)
}

// extractSalary extracts posted salary from a JD that contains salary information.
func extractSalary(title, company, location, desc, provider string, cfg config.Config) (map[string]interface{}, error) {
	prompt := buildSalaryPrompt(title, company, location, desc)
	var lastErr error
	var lastModel string
	for attempt := 0; attempt < 2; attempt++ {
		raw, model, err := callSalaryLLM(prompt, provider, 0.0, cfg)
		lastModel = model
		if err != nil {
			log.Printf("→ %s salary extraction LLM call failed (attempt %d): %v", model, attempt+1, err)
			lastErr = err
			continue
		}
		result, err := parseSalaryResponse(raw, model)
		if err != nil {
			log.Printf("→ %s salary extraction parse failed (attempt %d), raw: %.300s", model, attempt+1, raw)
			lastErr = err
			continue
		}
		result["llm_provider"] = provider
		result["llm_model"] = model
		result["source"] = "posted"
		return result, nil
	}
	_ = lastErr
	return nil, fmt.Errorf("%s could not extract salary after 2 attempts", lastModel)
}
