package server

import "fmt"

// ── Chunk prompt builders (used by callOllamaChunked) ─────────────────────────

// buildChunk1Prompt returns the system prompt for chunk 1: score + reasoning only.
// Reasoning is capped at 2 sentences MAX so small models don't overrun num_predict=350.
func buildChunk1Prompt(_ string) string {
	return `You are a technical recruiter evaluating a candidate's resume against a job description.
Return ONLY valid JSON — no prose, no markdown, no code fences.

Exactly this JSON shape:
{
  "reasoning": "<2 sentences MAX — direct and honest assessment>",
  "score": <integer 1-5>
}

Scoring: 1=poor 2=weak 3=moderate 4=strong 5=excellent
Reasoning MUST be 2 sentences MAX. No more.`
}

// buildChunk2Prompt returns the system prompt for chunk 2: matched_skills only.
// fast mode: jd_snippet only. standard/detailed: jd_snippet + resume_snippet.
// #25: num_predict is scaled by the caller (800 fast/standard, 1400 detailed).
func buildChunk2Prompt(mcfg ModeConfig, mode string) string {
	slen := mcfg.SnippetLen
	mm := mcfg.MaxMatched
	if mode == "fast" {
		return fmt.Sprintf(
			`You are a technical recruiter. List only matched skills from the resume vs the job description.
Return ONLY valid JSON — no prose, no markdown, no code fences.

Return at most %d skills. Skill names max 4 words. Snippets verbatim, max %d chars.
Exactly this JSON shape:
{
  "matched_skills": [
    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>"},
    ...
  ]
}
match_type: exact=verbatim in both, partial=related term, inferred=implied by context`, mm, slen, slen)
	}
	// standard and detailed: include resume_snippet
	return fmt.Sprintf(
		`You are a technical recruiter. List only matched skills from the resume vs the job description.
Return ONLY valid JSON — no prose, no markdown, no code fences.

Return at most %d skills. Snippets must be verbatim phrases, max %d chars. Do NOT fabricate snippets.
If no direct phrase exists: set match_type to "inferred" and omit resume_snippet.
Exactly this JSON shape:
{
  "matched_skills": [
    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>", "resume_snippet": "<%d chars>"},
    ...
  ]
}
match_type: exact=verbatim in both, partial=related term, inferred=implied by context`, mm, slen, slen, slen)
}

// buildChunk3Prompt returns the system prompt for chunk 3: missing_skills only.
func buildChunk3Prompt(mcfg ModeConfig, _ string) string {
	slen := mcfg.SnippetLen
	mms := mcfg.MaxMissing
	return fmt.Sprintf(
		`You are a technical recruiter. List only skills/requirements that are missing from the resume.
Return ONLY valid JSON — no prose, no markdown, no code fences.

Return at most %d skills. Snippets verbatim from the job description, max %d chars.
Exactly this JSON shape:
{
  "missing_skills": [
    {"skill": "name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "<%d chars>"},
    ...
  ]
}
severity: blocker=eliminates candidacy, major=significant gap, minor=nice-to-have
requirement_type: hard=required/must have, preferred=preferred/desired, bonus=nice to have`, mms, slen, slen)
}

// buildChunk4Prompt returns the system prompt for chunk 4: suggestions only.
// Only called in detailed mode when mcfg.Suggestions is true.
func buildChunk4Prompt() string {
	return `You are a technical recruiter. Given a resume and job description, provide exactly 3 actionable suggestions to improve the resume for this role.
Return ONLY valid JSON — no prose, no markdown, no code fences.

Exactly this JSON shape:
{
  "suggestions": [
    {"title": "short label", "detail": "specific actionable text", "job_requirement": "verbatim JD phrase"},
    {"title": "...", "detail": "...", "job_requirement": "..."},
    {"title": "...", "detail": "...", "job_requirement": "..."}
  ]
}
CRITICAL rules:
- Generate EXACTLY 3 suggestions
- ONLY suggest clarifying, repositioning, or expanding EXISTING resume content
- NEVER suggest adding skills the candidate does not already have
- Each suggestion must cite the specific job requirement it addresses`
}

func buildUserPrompt(resume, jobDescription string) string {
	return fmt.Sprintf("## RESUME\n%s\n\n---\n\n## JOB DESCRIPTION\n%s\n\n---\n\nEvaluate the match and return ONLY the JSON object described in your instructions.", resume, jobDescription)
}

// buildSystemPrompt returns a mode-appropriate system prompt.
// resumeSnippet controls whether standard mode includes resume_snippet in matched_skills.
// reasoning is placed FIRST in all schemas — small models sometimes stop early after
// generating skill lists; reasoning first ensures it is always produced.
func buildSystemPrompt(mcfg ModeConfig, mode string, resumeSnippet bool) string {
	slen := mcfg.SnippetLen
	mm   := mcfg.MaxMatched
	mms  := mcfg.MaxMissing

	// Short base for fast/standard; verbose base for detailed.
	const lite = "You are an expert technical recruiter and career coach specializing in software engineering,\nDevSecOps, and cloud infrastructure roles. You evaluate how well a candidate's resume matches a job description.\n\nYou MUST respond with ONLY valid JSON — no prose, no markdown, no code fences."
	const full = "You are an expert technical recruiter and career coach specializing in software engineering,\nDevSecOps, and cloud infrastructure roles. You evaluate how well a candidate's resume matches a job description.\n\nYou MUST respond with ONLY valid JSON — no prose, no markdown, no code fences. Your analysis must be thorough and precise — reference actual phrases from the resume and job description."

	// Definitions blocks — only appended in detailed mode; fast/standard inline values.
	severityDefs := `
Severity definitions for missing_skills:
  blocker = eliminates candidacy entirely (e.g. required clearance, mandatory cert, minimum years not met)
  major   = significant gap that will hurt chances substantially
  minor   = nice-to-have or learnable gap that is unlikely to disqualify

Requirement type definitions for missing_skills:
  hard      = job uses words like: required, must have, must hold, mandatory, eligibility-blocking
  preferred = job uses words like: preferred, desired, strong plus, ideally
  bonus     = job uses words like: nice to have, is a plus, familiarity with
  If unclear, use "preferred" as the default.

match_type definitions for matched_skills:
  exact    = skill name appears verbatim in both JD and resume
  partial  = related term found (e.g. "REST" matches "REST APIs")
  inferred = implied by context, no direct phrase found`

	scoringRubric := `
Scoring rubric:
  1 = Poor match — major gaps, different domain entirely
  2 = Weak match — some overlap but significant missing requirements
  3 = Moderate match — meets roughly half the requirements
  4 = Strong match — meets most requirements with minor gaps
  5 = Excellent match — highly aligned, apply immediately`

	switch mode {
	case "fast":
		return fmt.Sprintf(`%s

Return at most %d matched skills and at most %d missing skills — only the most significant ones.
Snippets must be verbatim phrases, max %d characters. Do NOT fabricate snippets.

Exactly this JSON shape:
{
  "reasoning": "<1-2 sentence honest assessment>",
  "score": <integer 1-5: 1=poor 2=weak 3=moderate 4=strong 5=excellent>,
  "matched_skills": [
    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>"},
    ...
  ],
  "missing_skills": [
    {"skill": "name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "<%d chars>"},
    ...
  ]
}`, lite, mm, mms, slen, slen, slen)

	case "standard":
		snippetNote := `If no direct phrase exists, set match_type to "inferred" and omit resume_snippet.`
		resumeSnippetField := fmt.Sprintf(`, "resume_snippet": "<%d chars>"`, slen)
		if !resumeSnippet {
			snippetNote = "Omit resume_snippet from all matched_skills entries."
			resumeSnippetField = ""
		}
		return fmt.Sprintf(`%s

Return at most %d matched skills and at most %d missing skills.
Snippets must be verbatim phrases copied from the provided text, max %d characters.
Do NOT fabricate or paraphrase snippets. %s

Exactly this JSON shape:
{
  "reasoning": "<2-4 sentence honest assessment>",
  "score": <integer 1-5: 1=poor 2=weak 3=moderate 4=strong 5=excellent>,
  "matched_skills": [
    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>"%s},
    ...
  ],
  "missing_skills": [
    {"skill": "name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "<%d chars>"},
    ...
  ]
}`, lite, mm, mms, slen, snippetNote, slen, resumeSnippetField, slen)

	default: // "detailed"
		if mcfg.Suggestions {
			return fmt.Sprintf(`%s

Return at most %d matched skills and at most %d missing skills.
Snippets must be verbatim phrases copied from the provided text, max %d characters.
Do NOT fabricate or paraphrase snippets. If no direct phrase exists, set match_type to "inferred" and omit resume_snippet.

Exactly this JSON shape:
{
  "reasoning": "<2-4 sentence honest assessment>",
  "score": <integer 1-5>,
  "matched_skills": [
    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>", "resume_snippet": "<%d chars>"},
    ...
  ],
  "missing_skills": [
    {"skill": "name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "<%d chars>"},
    ...
  ],
  "suggestions": [
    {"title": "short label", "detail": "specific actionable text", "job_requirement": "verbatim JD phrase"},
    ...
  ]
}
%s
%s

Suggestion rules — you MUST follow these exactly:
  - Generate exactly 3 resume improvement suggestions
  - ONLY suggest clarifying, repositioning, or expanding EXISTING resume content
  - NEVER suggest adding skills the candidate does not already have
  - Each suggestion must cite the specific job requirement it addresses`, full, mm, mms, slen, slen, slen, slen, severityDefs, scoringRubric)
		}
		return fmt.Sprintf(`%s

Return at most %d matched skills and at most %d missing skills.
Snippets must be verbatim phrases copied from the provided text, max %d characters.
Do NOT fabricate or paraphrase snippets. If no direct phrase exists, set match_type to "inferred" and omit resume_snippet.

Exactly this JSON shape:
{
  "reasoning": "<2-4 sentence honest assessment>",
  "score": <integer 1-5>,
  "matched_skills": [
    {"skill": "name", "match_type": "exact|partial|inferred", "jd_snippet": "<%d chars>", "resume_snippet": "<%d chars>"},
    ...
  ],
  "missing_skills": [
    {"skill": "name", "severity": "blocker|major|minor", "requirement_type": "hard|preferred|bonus", "jd_snippet": "<%d chars>"},
    ...
  ]
}
%s
%s`, full, mm, mms, slen, slen, slen, slen, severityDefs, scoringRubric)
	}
}
