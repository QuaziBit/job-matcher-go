package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuaziBit/job-matcher-go/config"
)

// ── buildSnippetPrompt ────────────────────────────────────────────────────────

func TestBuildSnippetPrompt_IncludesText(t *testing.T) {
	prompt := buildSnippetPrompt("Glassdoor 4.2 stars (500 reviews)")
	if !strings.Contains(prompt, "4.2 stars") {
		t.Error("expected prompt to contain pasted text")
	}
}

func TestBuildSnippetPrompt_TruncatesLongText(t *testing.T) {
	long := strings.Repeat("x", 5000)
	prompt := buildSnippetPrompt(long)
	if len(prompt) > 6000 {
		t.Errorf("expected prompt to be truncated, got len=%d", len(prompt))
	}
}

func TestBuildSnippetPrompt_RequestsJSONOutput(t *testing.T) {
	prompt := buildSnippetPrompt("some text")
	for _, field := range []string{"glassdoor_rating", "bbb_rating", "indeed_rating"} {
		if !strings.Contains(prompt, field) {
			t.Errorf("expected prompt to contain field %q", field)
		}
	}
}

// ── parseSnippetResponse ──────────────────────────────────────────────────────

func validSnippetJSON(overrides map[string]interface{}) string {
	base := map[string]interface{}{
		"glassdoor_rating":       4.2,
		"glassdoor_review_count": 500,
		"glassdoor_url":          "https://glassdoor.com/Overview/Acme.htm",
		"indeed_rating":          nil,
		"indeed_review_count":    nil,
		"indeed_url":             nil,
		"bbb_rating":             "A+",
		"bbb_url":                "https://bbb.org/acme",
		"linkedin_url":           nil,
		"linkedin_employee_count": nil,
		"linkedin_founded":       nil,
	}
	for k, v := range overrides {
		base[k] = v
	}
	b, _ := json.Marshal(base)
	return string(b)
}

func TestParseSnippetResponse_ParsesGlassdoorRating(t *testing.T) {
	result, err := parseSnippetResponse(validSnippetJSON(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.GlassdoorRating == nil || *result.GlassdoorRating != 4.2 {
		t.Errorf("expected glassdoor_rating=4.2, got %v", result.GlassdoorRating)
	}
}

func TestParseSnippetResponse_ParsesReviewCount(t *testing.T) {
	result, err := parseSnippetResponse(validSnippetJSON(nil))
	if err != nil {
		t.Fatal(err)
	}
	if result.GlassdoorReviewCount == nil || *result.GlassdoorReviewCount != 500 {
		t.Errorf("expected review_count=500, got %v", result.GlassdoorReviewCount)
	}
}

func TestParseSnippetResponse_ParsesBBBGrade(t *testing.T) {
	result, err := parseSnippetResponse(validSnippetJSON(nil))
	if err != nil {
		t.Fatal(err)
	}
	if result.BBBRating == nil || *result.BBBRating != "A+" {
		t.Errorf("expected bbb_rating=A+, got %v", result.BBBRating)
	}
}

func TestParseSnippetResponse_NullFieldsExcluded(t *testing.T) {
	result, err := parseSnippetResponse(validSnippetJSON(nil))
	if err != nil {
		t.Fatal(err)
	}
	if result.IndeedRating != nil {
		t.Error("expected indeed_rating to be nil")
	}
	if result.LinkedInURL != nil {
		t.Error("expected linkedin_url to be nil")
	}
}

func TestParseSnippetResponse_StripsMarkdownFences(t *testing.T) {
	raw := "```json\n" + validSnippetJSON(nil) + "\n```"
	result, err := parseSnippetResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.GlassdoorRating == nil {
		t.Error("expected glassdoor_rating after stripping fences")
	}
}

func TestParseSnippetResponse_JSONEmbeddedInProse(t *testing.T) {
	raw := "Here is the data:\n" + validSnippetJSON(nil) + "\nDone."
	result, err := parseSnippetResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.GlassdoorRating == nil {
		t.Error("expected glassdoor_rating from embedded JSON")
	}
}

func TestParseSnippetResponse_InvalidRatingExcluded(t *testing.T) {
	raw := `{"glassdoor_rating": 99.9, "glassdoor_review_count": 100}`
	result, err := parseSnippetResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.GlassdoorRating != nil {
		t.Error("expected invalid rating to be excluded")
	}
}

func TestParseSnippetResponse_ErrorOnNoJSON(t *testing.T) {
	_, err := parseSnippetResponse("no json here at all")
	if err == nil {
		t.Error("expected error on no JSON")
	}
}

func TestParseSnippetResponse_IndeedFieldsParsed(t *testing.T) {
	raw := `{"indeed_rating": 3.8, "indeed_review_count": 200, "indeed_url": "https://indeed.com/cmp/acme"}`
	result, err := parseSnippetResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.IndeedRating == nil || *result.IndeedRating != 3.8 {
		t.Errorf("expected indeed_rating=3.8, got %v", result.IndeedRating)
	}
	if result.IndeedReviewCount == nil || *result.IndeedReviewCount != 200 {
		t.Errorf("expected indeed_review_count=200, got %v", result.IndeedReviewCount)
	}
}

func TestParseSnippetResponse_NullStringExcluded(t *testing.T) {
	raw := `{"glassdoor_url": "null", "bbb_rating": "A"}`
	result, err := parseSnippetResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.GlassdoorURL != nil {
		t.Error(`expected "null" string to be excluded`)
	}
	if result.BBBRating == nil || *result.BBBRating != "A" {
		t.Errorf("expected bbb_rating=A, got %v", result.BBBRating)
	}
}

// ── SnippetResult.HasData ─────────────────────────────────────────────────────

func TestSnippetResult_HasData_EmptyIsFalse(t *testing.T) {
	if (SnippetResult{}).HasData() {
		t.Error("empty SnippetResult should not HasData")
	}
}

func TestSnippetResult_HasData_WithRatingIsTrue(t *testing.T) {
	f := 4.2
	r := SnippetResult{GlassdoorRating: &f}
	if !r.HasData() {
		t.Error("SnippetResult with rating should HasData")
	}
}

// ── snippetResolveModel ───────────────────────────────────────────────────────

func TestSnippetResolveModel_ExplicitModelUsed(t *testing.T) {
	cfg := config.Config{OllamaModel: "llama3.1:8b"}
	got := snippetResolveModel("ollama", "custom:13b", cfg)
	if got != "custom:13b" {
		t.Errorf("expected explicit model, got %q", got)
	}
}

func TestSnippetResolveModel_OllamaFallsBackToCfg(t *testing.T) {
	cfg := config.Config{OllamaModel: "llama3.1:8b"}
	got := snippetResolveModel("ollama", "", cfg)
	if got != "llama3.1:8b" {
		t.Errorf("expected cfg ollama model, got %q", got)
	}
}
