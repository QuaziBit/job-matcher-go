package server

import (
	"fmt"
	"strings"
)

// ── Shared prompt fragments ───────────────────────────────────────────────────

const jsonOnly          = "Return ONLY valid JSON — no prose, no markdown, no code fences."
const matchTypeDef      = "match_type: exact=verbatim in both, partial=related term, inferred=implied by context"
const severityDef       = "severity: blocker=eliminates candidacy, major=significant gap, minor=nice-to-have"
const requirementTypeDef = "requirement_type: hard=required/must have, preferred=preferred/desired, bonus=nice to have"

// systemLite is the base persona used for fast and standard modes.
const systemLite = "" +
	"You are an expert technical recruiter and career coach specializing in software engineering,\n" +
	"DevSecOps, and cloud infrastructure roles. You evaluate how well a candidate's resume matches a job description.\n\n" +
	"You MUST respond with ONLY valid JSON — no prose, no markdown, no code fences."

// systemFull is the base persona for detailed mode — adds thoroughness requirement.
const systemFull = "" +
	"You are an expert technical recruiter and career coach specializing in software engineering,\n" +
	"DevSecOps, and cloud infrastructure roles. You evaluate how well a candidate's resume matches a job description.\n\n" +
	"You MUST respond with ONLY valid JSON — no prose, no markdown, no code fences." +
	" Your analysis must be thorough and precise — reference actual phrases from the resume and job description."

const severityDefs = "\n" +
	"Severity definitions for missing_skills:\n" +
	"  blocker = eliminates candidacy entirely (e.g. required clearance, mandatory cert, minimum years not met)\n" +
	"  major   = significant gap that will hurt chances substantially\n" +
	"  minor   = nice-to-have or learnable gap that is unlikely to disqualify\n\n" +
	"Requirement type definitions for missing_skills:\n" +
	"  hard      = job uses words like: required, must have, must hold, mandatory, eligibility-blocking\n" +
	"  preferred = job uses words like: preferred, desired, strong plus, ideally\n" +
	"  bonus     = job uses words like: nice to have, is a plus, familiarity with\n" +
	"  If unclear, use \"preferred\" as the default.\n\n" +
	"match_type definitions for matched_skills:\n" +
	"  exact    = skill name appears verbatim in both JD and resume\n" +
	"  partial  = related term found (e.g. \"REST\" matches \"REST APIs\")\n" +
	"  inferred = implied by context, no direct phrase found"

const scoringRubric = "\n" +
	"Scoring rubric:\n" +
	"  1 = Poor match — major gaps, different domain entirely\n" +
	"  2 = Weak match — some overlap but significant missing requirements\n" +
	"  3 = Moderate match — meets roughly half the requirements\n" +
	"  4 = Strong match — meets most requirements with minor gaps\n" +
	"  5 = Excellent match — highly aligned, apply immediately"

// ── Chunk prompt builders (used by callOllamaChunked) ────────────────────────

// buildChunk1Prompt returns the system prompt for chunk 1: score + reasoning only.
// Reasoning is capped at 2 sentences MAX so small models don't overrun num_predict=350.
func buildChunk1Prompt(_ string) string {
	return strings.Join([]string{
		"You are a technical recruiter evaluating a candidate's resume against a job description.",
		jsonOnly,
		"",
		"Exactly this JSON shape:",
		`{`,
		`  "reasoning": "<2 sentences MAX — direct and honest assessment>",`,
		`  "score": <integer 1-5>`,
		`}`,
		"",
		"Scoring: 1=poor 2=weak 3=moderate 4=strong 5=excellent",
		"Reasoning MUST be 2 sentences MAX. No more.",
	}, "\n")
}

// buildChunk2Prompt returns the system prompt for chunk 2: matched_skills only.
// fast mode: jd_snippet only. standard/detailed: jd_snippet + resume_snippet.
// num_predict is scaled by the caller (800 fast/standard, 1400 detailed).
func buildChunk2Prompt(mcfg ModeConfig, mode string) string {
	slen := mcfg.SnippetLen
	mm   := mcfg.MaxMatched

	if mode == "fast" {
		return strings.Join([]string{
			"You are a technical recruiter. List only matched skills from the resume vs the job description.",
			jsonOnly,
			"",
			sprintf("Return at most %d skills. Skill names max 4 words. Snippets verbatim, max %d chars.", mm, slen),
			"Exactly this JSON shape:",
			`{`,
			`  "matched_skills": [`,
			sprintf(`    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>"},`, slen),
			`    ...`,
			`  ]`,
			`}`,
			matchTypeDef,
		}, "\n")
	}

	// standard and detailed: include resume_snippet
	return strings.Join([]string{
		"You are a technical recruiter. List only matched skills from the resume vs the job description.",
		jsonOnly,
		"",
		sprintf("Return at most %d skills. Snippets must be verbatim phrases, max %d chars. Do NOT fabricate snippets.", mm, slen),
		`If no direct phrase exists: set match_type to "inferred" and omit resume_snippet.`,
		"Exactly this JSON shape:",
		`{`,
		`  "matched_skills": [`,
		sprintf(`    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>", "resume_snippet": "<%d chars>"},`, slen, slen),
		`    ...`,
		`  ]`,
		`}`,
		matchTypeDef,
	}, "\n")
}

// buildChunk3Prompt returns the system prompt for chunk 3: missing_skills only.
func buildChunk3Prompt(mcfg ModeConfig, _ string) string {
	slen := mcfg.SnippetLen
	mms  := mcfg.MaxMissing

	return strings.Join([]string{
		"You are a technical recruiter. List only skills/requirements that are missing from the resume.",
		jsonOnly,
		"",
		sprintf("Return at most %d skills. Snippets verbatim from the job description, max %d chars.", mms, slen),
		"Exactly this JSON shape:",
		`{`,
		`  "missing_skills": [`,
		sprintf(`    {"skill": "name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "<%d chars>"},`, slen),
		`    ...`,
		`  ]`,
		`}`,
		severityDef,
		requirementTypeDef,
	}, "\n")
}

// buildChunk4Prompt returns the system prompt for chunk 4: suggestions only.
// Only called in detailed mode when mcfg.Suggestions is true.
func buildChunk4Prompt() string {
	return strings.Join([]string{
		"You are a technical recruiter. Given a resume and job description, provide exactly 3 actionable suggestions to improve the resume for this role.",
		jsonOnly,
		"",
		"Exactly this JSON shape:",
		`{`,
		`  "suggestions": [`,
		`    {"title": "short label", "detail": "specific actionable text", "job_requirement": "verbatim JD phrase"},`,
		`    {"title": "...", "detail": "...", "job_requirement": "..."},`,
		`    {"title": "...", "detail": "...", "job_requirement": "..."}`,
		`  ]`,
		`}`,
		"CRITICAL rules:",
		"- Generate EXACTLY 3 suggestions",
		"- ONLY suggest clarifying, repositioning, or expanding EXISTING resume content",
		"- NEVER suggest adding skills the candidate does not already have",
		"- Each suggestion must cite the specific job requirement it addresses",
	}, "\n")
}

// buildUserPrompt assembles the user-turn message shared by all providers.
func buildUserPrompt(resume, jobDescription string) string {
	return strings.Join([]string{
		"## RESUME",
		resume,
		"",
		"---",
		"",
		"## JOB DESCRIPTION",
		jobDescription,
		"",
		"---",
		"",
		"Evaluate the match and return ONLY the JSON object described in your instructions.",
	}, "\n")
}

// buildSystemPrompt returns a mode-appropriate system prompt for cloud providers.
// resumeSnippet controls whether standard mode includes resume_snippet in matched_skills.
// reasoning is placed FIRST in all schemas — small models sometimes stop early after
// generating skill lists; reasoning first ensures it is always produced.
func buildSystemPrompt(mcfg ModeConfig, mode string, resumeSnippet bool) string {
	slen := mcfg.SnippetLen
	mm   := mcfg.MaxMatched
	mms  := mcfg.MaxMissing

	switch mode {
	case "fast":
		return strings.Join([]string{
			systemLite,
			"",
			sprintf("Return at most %d matched skills and at most %d missing skills — only the most significant ones.", mm, mms),
			sprintf("Snippets must be verbatim phrases, max %d characters. Do NOT fabricate snippets.", slen),
			"",
			"Exactly this JSON shape:",
			`{`,
			`  "reasoning": "<1-2 sentence honest assessment>",`,
			`  "score": <integer 1-5: 1=poor 2=weak 3=moderate 4=strong 5=excellent>,`,
			`  "matched_skills": [`,
			sprintf(`    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>"},`, slen),
			`    ...`,
			`  ],`,
			`  "missing_skills": [`,
			sprintf(`    {"skill": "name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "<%d chars>"},`, slen),
			`    ...`,
			`  ]`,
			`}`,
		}, "\n")

	case "standard":
		snippetNote := `If no direct phrase exists, set match_type to "inferred" and omit resume_snippet.`
		matchedLine := sprintf(`    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>", "resume_snippet": "<%d chars>"},`, slen, slen)
		if !resumeSnippet {
			snippetNote = "Omit resume_snippet from all matched_skills entries."
			matchedLine = sprintf(`    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>"},`, slen)
		}
		return strings.Join([]string{
			systemLite,
			"",
			sprintf("Return at most %d matched skills and at most %d missing skills.", mm, mms),
			sprintf("Snippets must be verbatim phrases copied from the provided text, max %d characters.", slen),
			"Do NOT fabricate or paraphrase snippets. " + snippetNote,
			"",
			"Exactly this JSON shape:",
			`{`,
			`  "reasoning": "<2-4 sentence honest assessment>",`,
			`  "score": <integer 1-5: 1=poor 2=weak 3=moderate 4=strong 5=excellent>,`,
			`  "matched_skills": [`,
			matchedLine,
			`    ...`,
			`  ],`,
			`  "missing_skills": [`,
			sprintf(`    {"skill": "name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "<%d chars>"},`, slen),
			`    ...`,
			`  ]`,
			`}`,
		}, "\n")

	default: // "detailed"
		base := []string{
			systemFull,
			"",
			sprintf("Return at most %d matched skills and at most %d missing skills.", mm, mms),
			sprintf("Snippets must be verbatim phrases copied from the provided text, max %d characters.", slen),
			`Do NOT fabricate or paraphrase snippets. If no direct phrase exists, set match_type to "inferred" and omit resume_snippet.`,
			"",
			"Exactly this JSON shape:",
			`{`,
			`  "reasoning": "<2-4 sentence honest assessment>",`,
			`  "score": <integer 1-5>,`,
			`  "matched_skills": [`,
			sprintf(`    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>", "resume_snippet": "<%d chars>"},`, slen, slen),
			`    ...`,
			`  ],`,
			`  "missing_skills": [`,
			sprintf(`    {"skill": "name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "<%d chars>"},`, slen),
			`    ...`,
			`  ],`,
		}

		if mcfg.Suggestions {
			base = append(base,
				`  "suggestions": [`,
				`    {"title": "short label", "detail": "specific actionable text", "job_requirement": "verbatim JD phrase"},`,
				`    ...`,
				`  ]`,
				`}`,
				severityDefs,
				scoringRubric,
				"",
				"Suggestion rules — you MUST follow these exactly:",
				"  - Generate exactly 3 resume improvement suggestions",
				"  - ONLY suggest clarifying, repositioning, or expanding EXISTING resume content",
				"  - NEVER suggest adding skills the candidate does not already have",
				"  - Each suggestion must cite the specific job requirement it addresses",
			)
		} else {
			base = append(base,
				`}`,
				severityDefs,
				scoringRubric,
			)
		}

		return strings.Join(base, "\n")
	}
}

// sprintf is a package-local alias to keep call sites short.
func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)  //nolint:govet
}
