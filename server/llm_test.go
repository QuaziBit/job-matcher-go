package server

import (
	"strings"
	"testing"
)

func TestValidateLLMOutput_Score1NoMatchedIsValid(t *testing.T) {
	result := Analysis{Score: 1, Reasoning: "Complete mismatch"}
	out := validateLLMOutput(result, strings.Repeat("x", 600), "resume")
	if !out.Valid {
		t.Errorf("expected valid for score=1 + no matches, got errors: %v", out.Errors)
	}
}

func TestValidateLLMOutput_Score2NoMatchedRichJDIsInvalid(t *testing.T) {
	result := Analysis{Score: 2, Reasoning: "Some match"}
	out := validateLLMOutput(result, strings.Repeat("x", 600), "resume")
	if out.Valid {
		t.Error("expected invalid for score=2 + no matches + rich JD")
	}
	found := false
	for _, e := range out.Errors {
		if strings.Contains(e, "matched") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'matched' in errors, got: %v", out.Errors)
	}
}

func TestValidateLLMOutput_Score3WithMatchedIsValid(t *testing.T) {
	result := Analysis{
		Score:     3,
		Reasoning: "Good match",
		MatchedSkills: []MatchedSkill{{Skill: "Go"}},
	}
	out := validateLLMOutput(result, strings.Repeat("x", 600), "resume")
	if !out.Valid {
		t.Errorf("expected valid, got errors: %v", out.Errors)
	}
}

func TestValidateLLMOutput_ShortJDNotFlagged(t *testing.T) {
	result := Analysis{Score: 3, Reasoning: "ok"}
	out := validateLLMOutput(result, "short jd", "resume")
	if !out.Valid {
		t.Errorf("expected valid for short JD, got: %v", out.Errors)
	}
}
