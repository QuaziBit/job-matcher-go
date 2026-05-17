package server

import (
	"strings"
	"testing"

	"github.com/QuaziBit/job-matcher-go/config"
)

func TestBuildCompanyPrompt_IncludesCompanyName(t *testing.T) {
	p := buildCompanyPrompt("Acme Corp", map[string]interface{}{})
	if !strings.Contains(p, "Company name: Acme Corp") {
		t.Fatalf("expected company name line, got:\n%s", p)
	}
}

func TestBuildCompanyPrompt_IncludesBBBRating(t *testing.T) {
	meta := map[string]interface{}{"bbb_rating": "A+"}
	p := buildCompanyPrompt("Co", meta)
	if !strings.Contains(p, "BBB rating: A+") {
		t.Fatalf("expected BBB rating line, got:\n%s", p)
	}
}

func TestBuildCompanyPrompt_NoBBBWhenEmpty(t *testing.T) {
	p := buildCompanyPrompt("Co", map[string]interface{}{})
	if strings.Contains(p, "BBB rating:") {
		t.Error("did not expect BBB rating line when BBB fields empty")
	}
	if !strings.Contains(p, "BBB: no listing found") {
		t.Fatalf("expected no-listing BBB line, got:\n%s", p)
	}
}

func TestBuildCompanyPrompt_IncludesGlassdoorRating(t *testing.T) {
	meta := map[string]interface{}{
		"glassdoor_rating":        4.2,
		"glassdoor_review_count":  100,
	}
	p := buildCompanyPrompt("Co", meta)
	if !strings.Contains(p, "Glassdoor rating: 4.2/5 (100 reviews)") {
		t.Fatalf("expected Glassdoor rating line, got:\n%s", p)
	}
}

func TestBuildCompanyPrompt_IncludesLinkedInEmployees(t *testing.T) {
	meta := map[string]interface{}{
		"linkedin_employee_count": "5000+",
	}
	p := buildCompanyPrompt("Co", meta)
	if !strings.Contains(p, "LinkedIn: employees: 5000+") {
		t.Fatalf("expected LinkedIn employees line, got:\n%s", p)
	}
}

func TestBuildCompanyPrompt_RequestsJSONResponse(t *testing.T) {
	p := buildCompanyPrompt("X", nil)
	for _, needle := range []string{
		"Respond with ONLY a valid JSON object",
		`"risk_level": "low" | "medium" | "high" | "unknown"`,
		"Respond with JSON only. No prose, no markdown fences.",
	} {
		if !strings.Contains(p, needle) {
			t.Errorf("missing expected instruction fragment %q", needle)
		}
	}
}

func TestBuildCompanyPrompt_NoPIIInPrompt(t *testing.T) {
	meta := map[string]interface{}{
		"glassdoor_url":           "https://glassdoor.example/c",
		"bbb_rating":              "B",
		"recruiter_email":         "hacker@evil.com",
		"recruiter_phone":         "+1-555-0100",
		"applicant_ssn":           "123-45-6789",
	}
	p := buildCompanyPrompt("Legit Co", meta)
	if strings.Contains(p, "hacker@evil.com") || strings.Contains(p, "555-0100") || strings.Contains(p, "123-45-6789") {
		t.Fatalf("prompt must not echo unrelated PII fields from meta:\n%s", p)
	}
}

func TestParseVettingResponse_ValidJSON(t *testing.T) {
	raw := `{"risk_level":"low","assessment":"Looks fine.","signals":["a","b"]}`
	res, err := parseVettingResponse(raw, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	if res.RiskLevel != "low" || res.Assessment != "Looks fine." || len(res.Signals) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseVettingResponse_StripsMarkdownFences(t *testing.T) {
	raw := "```json\n{\"risk_level\":\"high\",\"assessment\":\"Bad.\",\"signals\":[]}\n```"
	res, err := parseVettingResponse(raw, "Co")
	if err != nil {
		t.Fatal(err)
	}
	if res.RiskLevel != "high" {
		t.Fatalf("risk=%q", res.RiskLevel)
	}
}

func TestParseVettingResponse_InvalidRiskLevelBecomesUnknown(t *testing.T) {
	raw := `{"risk_level":"nuclear","assessment":"x","signals":[]}`
	res, err := parseVettingResponse(raw, "Co")
	if err != nil {
		t.Fatal(err)
	}
	if res.RiskLevel != "unknown" {
		t.Fatalf("expected unknown, got %q", res.RiskLevel)
	}
}

func TestParseVettingResponse_AllRiskLevelsAccepted(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{`{"risk_level":"low","assessment":"ok","signals":[]}`, "low"},
		{`{"risk_level":"medium","assessment":"ok","signals":[]}`, "medium"},
		{`{"risk_level":"high","assessment":"ok","signals":[]}`, "high"},
		{`{"risk_level":"unknown","assessment":"ok","signals":[]}`, "unknown"},
		{`{"risk_level":"LOW","assessment":"ok","signals":[]}`, "low"},
		{`{"risk_level":" Medium ","assessment":"ok","signals":[]}`, "medium"},
	}
	for _, tc := range tests {
		res, err := parseVettingResponse(tc.raw, "Co")
		if err != nil {
			t.Fatalf("raw=%s: %v", tc.raw, err)
		}
		if res.RiskLevel != tc.want {
			t.Fatalf("raw=%s: got %q want %q", tc.raw, res.RiskLevel, tc.want)
		}
	}
}

func TestParseVettingResponse_MissingAssessmentGetsDefault(t *testing.T) {
	raw := `{"risk_level":"low","signals":[]}`
	res, err := parseVettingResponse(raw, "Co")
	if err != nil {
		t.Fatal(err)
	}
	if res.Assessment != "No assessment available." {
		t.Fatalf("got %q", res.Assessment)
	}
}

func TestParseVettingResponse_SignalsDefaultsToEmpty(t *testing.T) {
	for _, raw := range []string{
		`{"risk_level":"low","assessment":"ok"}`,
		`{"risk_level":"low","assessment":"ok","signals":null}`,
	} {
		res, err := parseVettingResponse(raw, "Co")
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if res.Signals == nil || len(res.Signals) != 0 {
			t.Fatalf("%s: want empty signals, got %#v", raw, res.Signals)
		}
	}
}

func TestParseVettingResponse_RaisesOnNoJSON(t *testing.T) {
	_, err := parseVettingResponse("no json here", "Co")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no JSON object found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseVettingResponse_JSONEmbeddedInProse(t *testing.T) {
	raw := "Here is my analysis.\n\n```json\n{\"risk_level\":\"medium\",\"assessment\":\"Mixed.\",\"signals\":[\"x\"]}\n```\nThanks."
	res, err := parseVettingResponse(raw, "Co")
	if err != nil {
		t.Fatal(err)
	}
	if res.RiskLevel != "medium" || res.Assessment != "Mixed." || len(res.Signals) != 1 || res.Signals[0] != "x" {
		t.Fatalf("unexpected: %+v", res)
	}
}

func TestParseVettingResponse_CompanyNameInResult(t *testing.T) {
	raw := `{"risk_level":"low","assessment":"ok","signals":[],"company":"Ignored Inc"}`
	res, err := parseVettingResponse(raw, "Canonical LLC")
	if err != nil {
		t.Fatal(err)
	}
	if res.Company != "Canonical LLC" {
		t.Fatalf("expected company from arg, got %q", res.Company)
	}
}

// ── formatRating ──────────────────────────────────────────────────────────────

func TestFormatRating_WholeNumber(t *testing.T) {
	if got := formatRating(4.0); got != "4" {
		t.Errorf("expected '4', got %q", got)
	}
}

func TestFormatRating_OneDecimal(t *testing.T) {
	if got := formatRating(4.2); got != "4.2" {
		t.Errorf("expected '4.2', got %q", got)
	}
}

func TestFormatRating_TrailingZeroStripped(t *testing.T) {
	if got := formatRating(4.50); got != "4.5" {
		t.Errorf("expected '4.5', got %q", got)
	}
}

// ── vettingResolveModel ───────────────────────────────────────────────────────

func TestVettingResolveModel_ExplicitModelUsed(t *testing.T) {
	cfg := config.Config{OllamaModel: "default:7b"}
	got := vettingResolveModel("ollama", "custom:13b", cfg)
	if got != "custom:13b" {
		t.Errorf("expected explicit model, got %q", got)
	}
}

func TestVettingResolveModel_OllamaFallsBackToCfg(t *testing.T) {
	cfg := config.Config{OllamaModel: "llama3.1:8b"}
	got := vettingResolveModel("ollama", "", cfg)
	if got != "llama3.1:8b" {
		t.Errorf("expected cfg ollama model, got %q", got)
	}
}

func TestVettingResolveModel_AnthropicFallsBackToCfg(t *testing.T) {
	cfg := config.Config{AnthropicModel: "claude-haiku-4-5-20251001"}
	got := vettingResolveModel("anthropic", "", cfg)
	if got != "claude-haiku-4-5-20251001" {
		t.Errorf("expected cfg anthropic model, got %q", got)
	}
}

func TestVettingResolveModel_GeminiDefault(t *testing.T) {
	cfg := config.Config{}
	got := vettingResolveModel("gemini", "", cfg)
	if got == "" {
		t.Error("expected non-empty default gemini model")
	}
}

func TestBuildCompanyPrompt_IncludesIndeedRating(t *testing.T) {
	meta := map[string]interface{}{
		"indeed_rating":       3.8,
		"indeed_review_count": 8,
	}
	prompt := buildCompanyPrompt("Co", meta)
	if !strings.Contains(prompt, "3.8") {
		t.Error("expected indeed_rating in prompt")
	}
	if !strings.Contains(prompt, "8") {
		t.Error("expected indeed_review_count in prompt")
	}
}

func TestBuildCompanyPrompt_IndeedNoListingWhenEmpty(t *testing.T) {
	prompt := buildCompanyPrompt("Co", map[string]interface{}{})
	if !strings.Contains(prompt, "Indeed: no listing found") {
		t.Errorf("expected 'Indeed: no listing found', got:\n%s", prompt)
	}
}

func TestBuildCompanyPrompt_IndeedListedNoRating(t *testing.T) {
	meta := map[string]interface{}{"indeed_url": "https://indeed.com/cmp/co"}
	prompt := buildCompanyPrompt("Co", meta)
	if !strings.Contains(prompt, "Indeed: listed but no rating available") {
		t.Errorf("expected 'Indeed: listed but no rating available', got:\n%s", prompt)
	}
}
