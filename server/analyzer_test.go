package server

import (
	"encoding/json"
	"testing"
)

// ── parseLLMResponse ──────────────────────────────────────────────────────────

func TestParseLLMResponse_ValidJSON(t *testing.T) {
	raw := `{"score": 4, "matched_skills": ["Python", "Docker"], "missing_skills": [{"skill": "Kubernetes", "severity": "major"}], "reasoning": "Good match."}`
	a, err := parseLLMResponse(raw, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Score != 4 {
		t.Errorf("expected score 4, got %d", a.Score)
	}
	if len(a.MatchedSkills) != 2 {
		t.Errorf("expected 2 matched skills, got %d", len(a.MatchedSkills))
	}
	if len(a.MissingSkills) != 1 {
		t.Errorf("expected 1 missing skill, got %d", len(a.MissingSkills))
	}
	if a.MissingSkills[0].Skill != "Kubernetes" {
		t.Errorf("expected Kubernetes, got %s", a.MissingSkills[0].Skill)
	}
}

func TestParseLLMResponse_StripsFences(t *testing.T) {
	raw := "```json\n{\"score\": 3, \"matched_skills\": [], \"missing_skills\": [], \"reasoning\": \"ok\"}\n```"
	a, err := parseLLMResponse(raw, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Score != 3 {
		t.Errorf("expected score 3, got %d", a.Score)
	}
}

func TestParseLLMResponse_InvalidScore(t *testing.T) {
	raw := `{"score": 9, "matched_skills": [], "missing_skills": [], "reasoning": "bad"}`
	_, err := parseLLMResponse(raw, "")
	if err == nil {
		t.Error("expected error for score 9")
	}
}

func TestParseLLMResponse_ScoreZeroInvalid(t *testing.T) {
	raw := `{"score": 0, "matched_skills": [], "missing_skills": [], "reasoning": "bad"}`
	_, err := parseLLMResponse(raw, "")
	if err == nil {
		t.Error("expected error for score 0")
	}
}

func TestParseLLMResponse_NoJSON(t *testing.T) {
	_, err := parseLLMResponse("no json here at all", "")
	if err == nil {
		t.Error("expected error for response with no JSON")
	}
}

func TestParseLLMResponse_JSONEmbeddedInProse(t *testing.T) {
	raw := `Here is my eval: {"score": 4, "matched_skills": ["Go"], "missing_skills": [], "reasoning": "Good."} Done.`
	a, err := parseLLMResponse(raw, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Score != 4 {
		t.Errorf("expected score 4, got %d", a.Score)
	}
}

func TestParseLLMResponse_FlatMissingSkillsAccepted(t *testing.T) {
	raw := `{"score": 3, "matched_skills": ["Python"], "missing_skills": ["Kubernetes", "Terraform"], "reasoning": "ok"}`
	a, err := parseLLMResponse(raw, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(a.MissingSkills) != 2 {
		t.Errorf("expected 2 missing skills, got %d", len(a.MissingSkills))
	}
	if a.MissingSkills[0].Skill != "Kubernetes" {
		t.Errorf("expected Kubernetes, got %s", a.MissingSkills[0].Skill)
	}
	// Flat strings default to minor
	if a.MissingSkills[0].Severity != "minor" {
		t.Errorf("expected severity minor for flat string, got %s", a.MissingSkills[0].Severity)
	}
}

func TestParseLLMResponse_AllScoreBoundaries(t *testing.T) {
	for _, score := range []int{1, 2, 3, 4, 5} {
		raw, _ := json.Marshal(map[string]interface{}{
			"score": score, "matched_skills": []string{},
			"missing_skills": []interface{}{}, "reasoning": "ok",
		})
		a, err := parseLLMResponse(string(raw), "")
		if err != nil {
			t.Errorf("score %d should be valid, got error: %v", score, err)
		}
		if a.Score != score {
			t.Errorf("expected score %d, got %d", score, a.Score)
		}
	}
}

// ── keywordBoost ──────────────────────────────────────────────────────────────

func TestKeywordBoost_ClearanceUpgradedToBlocker(t *testing.T) {
	skills := []MissingSkill{{Skill: "Active TS/SCI Clearance", Severity: "minor"}}
	result := keywordBoost(skills, "Must have active clearance to apply")
	if result[0].Severity != "blocker" {
		t.Errorf("expected blocker, got %s", result[0].Severity)
	}
}

func TestKeywordBoost_YearsRequirementUpgraded(t *testing.T) {
	skills := []MissingSkill{{Skill: "7 years of experience", Severity: "major"}}
	result := keywordBoost(skills, "Requires 7+ years of software development experience")
	if result[0].Severity != "blocker" {
		t.Errorf("expected blocker, got %s", result[0].Severity)
	}
}

func TestKeywordBoost_NoJDPreservesExistingSeverity(t *testing.T) {
	skills := []MissingSkill{{Skill: "Terraform", Severity: "major"}}
	result := keywordBoost(skills, "")
	if result[0].Severity != "major" {
		t.Errorf("expected major preserved, got %s", result[0].Severity)
	}
}

func TestKeywordBoost_NonBlockerSkillUnchanged(t *testing.T) {
	skills := []MissingSkill{{Skill: "Nice-to-have Ansible", Severity: "minor"}}
	result := keywordBoost(skills, "Experience with Ansible is a plus")
	if result[0].Severity != "minor" {
		t.Errorf("expected minor preserved, got %s", result[0].Severity)
	}
}

// ── computeAdjustedScore ──────────────────────────────────────────────────────

func TestComputeAdjustedScore_NoGapsNoChange(t *testing.T) {
	adj, breakdown := computeAdjustedScore(5, []MissingSkill{})
	if adj != 5 {
		t.Errorf("expected 5, got %d", adj)
	}
	if breakdown.TotalPenalty != 0 {
		t.Errorf("expected 0 total penalty, got %d", breakdown.TotalPenalty)
	}
}

func TestComputeAdjustedScore_BlockerReducesScore(t *testing.T) {
	missing := []MissingSkill{{Skill: "TS/SCI Clearance", Severity: "blocker"}}
	adj, breakdown := computeAdjustedScore(4, missing)
	if adj >= 4 {
		t.Errorf("expected adjusted < 4, got %d", adj)
	}
	if breakdown.BlockerPenalty != 2 {
		t.Errorf("expected blocker penalty 2, got %d", breakdown.BlockerPenalty)
	}
}

func TestComputeAdjustedScore_NeverBelowOne(t *testing.T) {
	missing := []MissingSkill{
		{Skill: "Clearance", Severity: "blocker"},
		{Skill: "10 years exp", Severity: "blocker"},
		{Skill: "K8s", Severity: "major"},
	}
	adj, _ := computeAdjustedScore(2, missing)
	if adj < 1 {
		t.Errorf("adjusted score should never be below 1, got %d", adj)
	}
}

func TestComputeAdjustedScore_TwoMinorsGivePenalty(t *testing.T) {
	missing := []MissingSkill{
		{Skill: "A", Severity: "minor"},
		{Skill: "B", Severity: "minor"},
	}
	_, breakdown := computeAdjustedScore(5, missing)
	if breakdown.MinorPenalty != 1 {
		t.Errorf("expected minor penalty 1 for 2 minors, got %d", breakdown.MinorPenalty)
	}
}

func TestComputeAdjustedScore_OneMinorNoPenalty(t *testing.T) {
	missing := []MissingSkill{{Skill: "A", Severity: "minor"}}
	_, breakdown := computeAdjustedScore(5, missing)
	if breakdown.MinorPenalty != 0 {
		t.Errorf("expected minor penalty 0 for 1 minor, got %d", breakdown.MinorPenalty)
	}
}

func TestComputeAdjustedScore_CountPenaltyAboveSix(t *testing.T) {
	var missing []MissingSkill
	for i := 0; i < 7; i++ {
		missing = append(missing, MissingSkill{Skill: "skill", Severity: "minor"})
	}
	_, breakdown := computeAdjustedScore(5, missing)
	if breakdown.CountPenalty != 1 {
		t.Errorf("expected count penalty 1 for 7 gaps, got %d", breakdown.CountPenalty)
	}
}

func TestComputeAdjustedScore_BlockerCapAt3(t *testing.T) {
	missing := []MissingSkill{
		{Skill: "A", Severity: "blocker"},
		{Skill: "B", Severity: "blocker"},
		{Skill: "C", Severity: "blocker"},
	}
	_, breakdown := computeAdjustedScore(5, missing)
	if breakdown.BlockerPenalty > 3 {
		t.Errorf("blocker penalty should be capped at 3, got %d", breakdown.BlockerPenalty)
	}
}

func TestComputeAdjustedScore_MajorCapAt2(t *testing.T) {
	missing := []MissingSkill{
		{Skill: "A", Severity: "major"},
		{Skill: "B", Severity: "major"},
		{Skill: "C", Severity: "major"},
	}
	_, breakdown := computeAdjustedScore(5, missing)
	if breakdown.MajorPenalty > 2 {
		t.Errorf("major penalty should be capped at 2, got %d", breakdown.MajorPenalty)
	}
}

// ── Additional edge cases ─────────────────────────────────────────────────────

func TestParseLLMResponse_EmptySkillLists(t *testing.T) {
	raw := `{"score": 3, "matched_skills": [], "missing_skills": [], "reasoning": "No skills detected."}`
	a, err := parseLLMResponse(raw, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Score != 3 {
		t.Errorf("expected score 3, got %d", a.Score)
	}
	if len(a.MatchedSkills) != 0 {
		t.Errorf("expected 0 matched skills, got %d", len(a.MatchedSkills))
	}
	if len(a.MissingSkills) != 0 {
		t.Errorf("expected 0 missing skills, got %d", len(a.MissingSkills))
	}
}

func TestComputeAdjustedScore_MinorCapAt1(t *testing.T) {
	// 5 minors should cap total minor penalty at -1, not -2
	missing := []MissingSkill{
		{Skill: "Skill A", Severity: "minor"},
		{Skill: "Skill B", Severity: "minor"},
		{Skill: "Skill C", Severity: "minor"},
		{Skill: "Skill D", Severity: "minor"},
		{Skill: "Skill E", Severity: "minor"},
	}
	adjusted, pb := computeAdjustedScore(4, missing)
	if pb.MinorPenalty > 1 {
		t.Errorf("minor penalty should be capped at 1, got %d", pb.MinorPenalty)
	}
	if adjusted < 3 {
		t.Errorf("5 minors from score 4 should not drop below 3, got %d", adjusted)
	}
}

func TestComputeAdjustedScore_NeverBelowOneWithManyBlockers(t *testing.T) {
	// Many blockers should not push score below 1
	missing := []MissingSkill{
		{Skill: "TS/SCI Clearance", Severity: "blocker"},
		{Skill: "US Citizenship", Severity: "blocker"},
		{Skill: "Polygraph", Severity: "blocker"},
		{Skill: "Secret Clearance", Severity: "blocker"},
	}
	adjusted, _ := computeAdjustedScore(2, missing)
	if adjusted < 1 {
		t.Errorf("adjusted score should never go below 1, got %d", adjusted)
	}
}

func TestKeywordBoost_MultipleClearanceLevels(t *testing.T) {
	jd := "Must have TS/SCI clearance with full scope polygraph"
	skills := []MissingSkill{
		{Skill: "TS/SCI", Severity: "major"},
		{Skill: "Polygraph", Severity: "major"},
	}
	results := keywordBoost(skills, jd)
	for _, result := range results {
		if result.Severity != "blocker" {
			t.Errorf("expected %q to be upgraded to blocker in clearance JD, got %q",
				result.Skill, result.Severity)
		}
	}
}

func TestKeywordBoost_PreservesMinorSeverity(t *testing.T) {
	// A minor skill with no clearance/years keywords should stay minor
	skills := []MissingSkill{
		{Skill: "Tailwind CSS", Severity: "minor"},
	}
	results := keywordBoost(skills, "Looking for a full stack developer with React experience")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Severity != "minor" {
		t.Errorf("expected 'minor' to be preserved, got %q", results[0].Severity)
	}
}
