package server

import (
	"strings"
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// fastCfg / stdCfg / detCfg mirror the real modeConfigs entries.
var (
	fastCfg = modeConfigs["fast"]
	stdCfg  = modeConfigs["standard"]
	detCfg  = modeConfigs["detailed"]
)

func assertContains(t *testing.T, label, s, want string) {
	t.Helper()
	if !strings.Contains(s, want) {
		t.Errorf("%s: expected to contain %q", label, want)
	}
}

func assertNotContains(t *testing.T, label, s, want string) {
	t.Helper()
	if strings.Contains(s, want) {
		t.Errorf("%s: expected NOT to contain %q", label, want)
	}
}

// ── buildChunk1Prompt ─────────────────────────────────────────────────────────

func TestBuildChunk1Prompt_NotEmpty(t *testing.T) {
	out := buildChunk1Prompt("")
	if out == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestBuildChunk1Prompt_ContainsJsonOnly(t *testing.T) {
	assertContains(t, "chunk1", buildChunk1Prompt(""), jsonOnly)
}

func TestBuildChunk1Prompt_ContainsScoreField(t *testing.T) {
	assertContains(t, "chunk1", buildChunk1Prompt(""), `"score"`)
}

func TestBuildChunk1Prompt_ContainsReasoningField(t *testing.T) {
	assertContains(t, "chunk1", buildChunk1Prompt(""), `"reasoning"`)
}

func TestBuildChunk1Prompt_NoMatchedSkills(t *testing.T) {
	assertNotContains(t, "chunk1", buildChunk1Prompt(""), "matched_skills")
}

// ── buildChunk2Prompt ─────────────────────────────────────────────────────────

func TestBuildChunk2Prompt_FastNotEmpty(t *testing.T) {
	if buildChunk2Prompt(fastCfg, "fast") == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestBuildChunk2Prompt_FastContainsJsonOnly(t *testing.T) {
	assertContains(t, "chunk2/fast", buildChunk2Prompt(fastCfg, "fast"), jsonOnly)
}

func TestBuildChunk2Prompt_FastContainsMatchTypeDef(t *testing.T) {
	assertContains(t, "chunk2/fast", buildChunk2Prompt(fastCfg, "fast"), matchTypeDef)
}

func TestBuildChunk2Prompt_FastNoResumeSnippet(t *testing.T) {
	assertNotContains(t, "chunk2/fast", buildChunk2Prompt(fastCfg, "fast"), "resume_snippet")
}

func TestBuildChunk2Prompt_StandardContainsResumeSnippet(t *testing.T) {
	assertContains(t, "chunk2/standard", buildChunk2Prompt(stdCfg, "standard"), "resume_snippet")
}

func TestBuildChunk2Prompt_DetailedContainsResumeSnippet(t *testing.T) {
	assertContains(t, "chunk2/detailed", buildChunk2Prompt(detCfg, "detailed"), "resume_snippet")
}

func TestBuildChunk2Prompt_FastVsStandardDiffer(t *testing.T) {
	if buildChunk2Prompt(fastCfg, "fast") == buildChunk2Prompt(stdCfg, "standard") {
		t.Error("fast and standard prompts should differ")
	}
}

// ── buildChunk3Prompt ─────────────────────────────────────────────────────────

func TestBuildChunk3Prompt_NotEmpty(t *testing.T) {
	if buildChunk3Prompt(stdCfg, "standard") == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestBuildChunk3Prompt_ContainsJsonOnly(t *testing.T) {
	assertContains(t, "chunk3", buildChunk3Prompt(stdCfg, "standard"), jsonOnly)
}

func TestBuildChunk3Prompt_ContainsSeverityDef(t *testing.T) {
	assertContains(t, "chunk3", buildChunk3Prompt(stdCfg, "standard"), severityDef)
}

func TestBuildChunk3Prompt_ContainsRequirementTypeDef(t *testing.T) {
	assertContains(t, "chunk3", buildChunk3Prompt(stdCfg, "standard"), requirementTypeDef)
}

func TestBuildChunk3Prompt_ContainsMissingSkillsField(t *testing.T) {
	assertContains(t, "chunk3", buildChunk3Prompt(stdCfg, "standard"), "missing_skills")
}

func TestBuildChunk3Prompt_NoMatchedSkills(t *testing.T) {
	assertNotContains(t, "chunk3", buildChunk3Prompt(stdCfg, "standard"), "matched_skills")
}

// ── buildChunk4Prompt ─────────────────────────────────────────────────────────

func TestBuildChunk4Prompt_NotEmpty(t *testing.T) {
	if buildChunk4Prompt() == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestBuildChunk4Prompt_ContainsJsonOnly(t *testing.T) {
	assertContains(t, "chunk4", buildChunk4Prompt(), jsonOnly)
}

func TestBuildChunk4Prompt_ContainsSuggestionsField(t *testing.T) {
	assertContains(t, "chunk4", buildChunk4Prompt(), "suggestions")
}

func TestBuildChunk4Prompt_ContainsExactly3Rule(t *testing.T) {
	assertContains(t, "chunk4", buildChunk4Prompt(), "EXACTLY 3")
}

func TestBuildChunk4Prompt_NoScoreField(t *testing.T) {
	assertNotContains(t, "chunk4", buildChunk4Prompt(), `"score"`)
}

// ── buildUserPrompt ───────────────────────────────────────────────────────────

func TestBuildUserPrompt_ContainsResume(t *testing.T) {
	out := buildUserPrompt("my resume text", "job description text")
	assertContains(t, "userPrompt", out, "my resume text")
}

func TestBuildUserPrompt_ContainsJobDescription(t *testing.T) {
	out := buildUserPrompt("my resume text", "job description text")
	assertContains(t, "userPrompt", out, "job description text")
}

func TestBuildUserPrompt_ContainsSectionHeaders(t *testing.T) {
	out := buildUserPrompt("r", "j")
	assertContains(t, "userPrompt", out, "## RESUME")
	assertContains(t, "userPrompt", out, "## JOB DESCRIPTION")
}

func TestBuildUserPrompt_ResumeBeforeJobDescription(t *testing.T) {
	out := buildUserPrompt("RESUME_CONTENT", "JD_CONTENT")
	if strings.Index(out, "RESUME_CONTENT") > strings.Index(out, "JD_CONTENT") {
		t.Error("resume should appear before job description")
	}
}

// ── buildSystemPrompt — fast ──────────────────────────────────────────────────

func TestBuildSystemPrompt_Fast_ContainsSystemLite(t *testing.T) {
	assertContains(t, "sysPrompt/fast", buildSystemPrompt(fastCfg, "fast", false), systemLite)
}

func TestBuildSystemPrompt_Fast_NoSystemFull(t *testing.T) {
	// systemFull adds a unique phrase not in systemLite
	assertNotContains(t, "sysPrompt/fast", buildSystemPrompt(fastCfg, "fast", false), "thorough and precise")
}

func TestBuildSystemPrompt_Fast_ContainsMatchedAndMissingFields(t *testing.T) {
	out := buildSystemPrompt(fastCfg, "fast", false)
	assertContains(t, "sysPrompt/fast", out, "matched_skills")
	assertContains(t, "sysPrompt/fast", out, "missing_skills")
}

func TestBuildSystemPrompt_Fast_NoSuggestions(t *testing.T) {
	assertNotContains(t, "sysPrompt/fast", buildSystemPrompt(fastCfg, "fast", false), "suggestions")
}

// ── buildSystemPrompt — standard ─────────────────────────────────────────────

func TestBuildSystemPrompt_Standard_ContainsSystemLite(t *testing.T) {
	assertContains(t, "sysPrompt/standard", buildSystemPrompt(stdCfg, "standard", true), systemLite)
}

func TestBuildSystemPrompt_Standard_WithResumeSnippet(t *testing.T) {
	assertContains(t, "sysPrompt/standard/withSnippet", buildSystemPrompt(stdCfg, "standard", true), "resume_snippet")
}

func TestBuildSystemPrompt_Standard_WithoutResumeSnippet(t *testing.T) {
	out := buildSystemPrompt(stdCfg, "standard", false)
	// The omit instruction contains the word "resume_snippet" — check the JSON field definition is absent instead
	assertNotContains(t, "sysPrompt/standard/noSnippet", out, `"resume_snippet":`)
	assertContains(t, "sysPrompt/standard/noSnippet", out, "Omit resume_snippet")
}

func TestBuildSystemPrompt_Standard_ResumeSnippetFlagDiffers(t *testing.T) {
	with := buildSystemPrompt(stdCfg, "standard", true)
	without := buildSystemPrompt(stdCfg, "standard", false)
	if with == without {
		t.Error("resumeSnippet=true and false should produce different prompts")
	}
}

// ── buildSystemPrompt — detailed ─────────────────────────────────────────────

func TestBuildSystemPrompt_Detailed_ContainsSystemFull(t *testing.T) {
	assertContains(t, "sysPrompt/detailed", buildSystemPrompt(detCfg, "detailed", true), "thorough and precise")
}

func TestBuildSystemPrompt_Detailed_WithSuggestions_ContainsSuggestionsBlock(t *testing.T) {
	assertContains(t, "sysPrompt/detailed/suggestions", buildSystemPrompt(detCfg, "detailed", true), "suggestions")
}

func TestBuildSystemPrompt_Detailed_WithSuggestions_ContainsSuggestionRules(t *testing.T) {
	out := buildSystemPrompt(detCfg, "detailed", true)
	assertContains(t, "sysPrompt/detailed/suggestions", out, "Suggestion rules")
	assertContains(t, "sysPrompt/detailed/suggestions", out, "exactly 3")
}

func TestBuildSystemPrompt_Detailed_WithSuggestions_ContainsSeverityDefs(t *testing.T) {
	assertContains(t, "sysPrompt/detailed/suggestions", buildSystemPrompt(detCfg, "detailed", true), severityDefs)
}

func TestBuildSystemPrompt_Detailed_WithSuggestions_ContainsScoringRubric(t *testing.T) {
	assertContains(t, "sysPrompt/detailed/suggestions", buildSystemPrompt(detCfg, "detailed", true), scoringRubric)
}

func TestBuildSystemPrompt_Detailed_WithoutSuggestions_NoSuggestionsBlock(t *testing.T) {
	noSugCfg := detCfg
	noSugCfg.Suggestions = false
	assertNotContains(t, "sysPrompt/detailed/noSuggestions", buildSystemPrompt(noSugCfg, "detailed", true), "Suggestion rules")
}

func TestBuildSystemPrompt_Detailed_SuggestionsFlagDiffers(t *testing.T) {
	noSugCfg := detCfg
	noSugCfg.Suggestions = false
	with := buildSystemPrompt(detCfg, "detailed", true)
	without := buildSystemPrompt(noSugCfg, "detailed", true)
	if with == without {
		t.Error("Suggestions=true and false should produce different prompts")
	}
}

// ── mode isolation ────────────────────────────────────────────────────────────

func TestBuildSystemPrompt_AllModesDistinct(t *testing.T) {
	fast := buildSystemPrompt(fastCfg, "fast", true)
	std  := buildSystemPrompt(stdCfg, "standard", true)
	det  := buildSystemPrompt(detCfg, "detailed", true)

	if fast == std {
		t.Error("fast and standard prompts should differ")
	}
	if std == det {
		t.Error("standard and detailed prompts should differ")
	}
	if fast == det {
		t.Error("fast and detailed prompts should differ")
	}
}
