package server

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
)

// repairTruncatedJSON attempts to close a truncated JSON object by counting
// unclosed braces and brackets and appending the necessary closing characters.
func repairTruncatedJSON(raw string) string {
	open := strings.Count(raw, "{") - strings.Count(raw, "}")
	openArr := strings.Count(raw, "[") - strings.Count(raw, "]")
	if open <= 0 && openArr <= 0 {
		return raw // not truncated
	}
	// Strip trailing comma or partial token
	stripped := strings.TrimRight(raw, " \t\r\n,:{[")
	closing := strings.Repeat("]", openArr) + strings.Repeat("}", open)
	log.Printf("→ repaired truncated JSON: appended %q", closing)
	return stripped + closing
}

// escapeControlChars escapes literal control characters (tab, newline, CR)
// inside JSON string values. Handles PDF copy-paste artifacts where raw
// control bytes appear inside strings and break standard JSON parsing.
func escapeControlChars(raw string) string {
	var out strings.Builder
	inString := false
	i := 0
	n := len(raw)
	for i < n {
		c := raw[i]
		if inString {
			switch c {
			case '\\':
				out.WriteByte(c)
				i++
				if i < n {
					out.WriteByte(raw[i])
					i++
				}
			case '"':
				out.WriteByte(c)
				i++
				inString = false
			case '\t':
				out.WriteString(`\t`)
				i++
			case '\n':
				out.WriteString(`\n`)
				i++
			case '\r':
				out.WriteString(`\r`)
				i++
			default:
				out.WriteByte(c)
				i++
			}
		} else {
			if c == '"' {
				inString = true
			}
			out.WriteByte(c)
			i++
		}
	}
	return out.String()
}

// splitTopLevelObjects finds all top-level JSON objects in raw by tracking
// brace depth. Used as fallback when greedy first{..last} extraction fails —
// handles models that concatenate two JSON objects in one response (e.g. llama3.2:3b).
func splitTopLevelObjects(raw string) []string {
	var results []string
	depth := 0
	start := -1
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start != -1 {
				results = append(results, raw[start:i+1])
				start = -1
			}
		}
	}
	return results
}

// sanitizeJSON escapes unescaped double quotes inside JSON string values.
// Uses a state machine to distinguish structural quotes from inner content quotes.
func sanitizeJSON(raw string) string {
	// Normalize smart quotes
	raw = strings.NewReplacer(
		"\u201c", `"`, "\u201d", `"`,
		"\u2018", "'", "\u2019", "'",
	).Replace(raw)

	var out strings.Builder
	i := 0
	n := len(raw)
	for i < n {
		c := raw[i]
		if c != '"' {
			out.WriteByte(c)
			i++
			continue
		}
		// Opening quote of a token
		out.WriteByte(c)
		i++
		// Read token contents
		for i < n {
			c = raw[i]
			if c == '\\' {
				out.WriteByte(c)
				i++
				if i < n {
					out.WriteByte(raw[i])
					i++
				}
				continue
			}
			if c == '"' {
				out.WriteByte(c)
				i++
				break
			}
			out.WriteByte(c)
			i++
		}
		// Check if followed by colon → this was a key, now parse value
		j := i
		for j < n && (raw[j] == ' ' || raw[j] == '\t' || raw[j] == '\r' || raw[j] == '\n') {
			j++
		}
		if j < n && raw[j] == ':' {
			// Write colon and whitespace
			for i <= j {
				out.WriteByte(raw[i])
				i++
			}
			// Skip whitespace after colon
			for i < n && (raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\r' || raw[i] == '\n') {
				out.WriteByte(raw[i])
				i++
			}
			// Parse value with inner-quote fixing
			if i < n && raw[i] == '"' {
				out.WriteByte('"')
				i++
				for i < n {
					c = raw[i]
					if c == '\\' {
						out.WriteByte(c)
						i++
						if i < n {
							out.WriteByte(raw[i])
							i++
						}
						continue
					}
					if c == '"' {
						// Is this the real closing quote?
						k := i + 1
						for k < n && (raw[k] == ' ' || raw[k] == '\t' || raw[k] == '\r' || raw[k] == '\n') {
							k++
						}
						var next byte
						if k < n {
							next = raw[k]
						}
						if next == ',' || next == '}' || next == ']' || next == 0 || next == '"' {
							break // real closing quote
						}
						// Unescaped inner quote — escape it
						out.WriteString(`\"`)
						i++
						continue
					}
					out.WriteByte(c)
					i++
				}
				out.WriteByte('"')
				i++ // skip real closing quote
				continue
			}
		}
	}
	return out.String()
}

type llmRawResponse struct {
	Score         json.Number       `json:"score"` // accept int or float (e.g. 4.5 → 5)
	MatchedSkills []json.RawMessage `json:"matched_skills"`
	MissingSkills []json.RawMessage `json:"missing_skills"`
	Reasoning     string            `json:"reasoning"`
	Suggestions   []json.RawMessage `json:"suggestions"`
}

// tryParsePasses runs the five-pass parse loop on a single candidate string.
// Returns (resp, nil) on first successful parse, or ("", lastErr) if all fail.
func tryParsePasses(candidate string) (llmRawResponse, error) {
	passes := []struct {
		name string
		fn   func(string) string
	}{
		{"raw", func(s string) string { return s }},
		{"escape", escapeControlChars},
		{"repair", repairTruncatedJSON},
		{"sanitize", sanitizeJSON},
		{"repair+sanitize", func(s string) string { return sanitizeJSON(repairTruncatedJSON(s)) }},
	}
	var resp llmRawResponse
	var lastErr error
	for _, p := range passes {
		if err := json.Unmarshal([]byte(p.fn(candidate)), &resp); err == nil {
			return resp, nil
		} else {
			lastErr = err
		}
	}
	return llmRawResponse{}, fmt.Errorf("all %d parse passes failed: %w", len(passes), lastErr)
}

// ── Skill list builders ───────────────────────────────────────────────────────
// These are shared by parseLLMResponse (single-shot) and callOllamaChunked.

// buildMatchedSkills parses raw matched-skill JSON items, normalizes, and caps
// at mcfg.MaxMatched. Accepts both v2 {skill,match_type,...} and v1 ["skill"] shapes.
// Falls back to "name" key when "skill" is absent (phi3.5 emits "name").
func buildMatchedSkills(items []json.RawMessage, mcfg ModeConfig) []MatchedSkill {
	var matched []MatchedSkill
	for _, r := range items {
		var v2 MatchedSkill
		if err := json.Unmarshal(r, &v2); err == nil {
			if v2.Skill == "" {
				var m map[string]interface{}
				if json.Unmarshal(r, &m) == nil {
					if name, ok := m["name"].(string); ok {
						v2.Skill = name
					}
				}
			}
			if v2.Skill != "" {
				v2.MatchType = normalizeMatchType(v2.MatchType)
				matched = append(matched, v2)
				continue
			}
		}
		var flat string
		if err := json.Unmarshal(r, &flat); err == nil && flat != "" {
			matched = append(matched, MatchedSkill{Skill: flat, MatchType: "exact"})
		}
	}
	for i := range matched {
		matched[i].Skill = NormalizeSkill(matched[i].Skill)
		matched[i].Category = GetSkillCategory(matched[i].Skill)
	}
	if mcfg.MaxMatched > 0 && len(matched) > mcfg.MaxMatched {
		matched = matched[:mcfg.MaxMatched]
	}
	return matched
}

// buildMissingSkills parses raw missing-skill JSON items, normalizes, caps at
// mcfg.MaxMissing, and applies keyword boost. Accepts v2 and v1 shapes.
func buildMissingSkills(items []json.RawMessage, jd string, mcfg ModeConfig) []MissingSkill {
	var missing []MissingSkill
	for _, r := range items {
		var structured MissingSkill
		if err := json.Unmarshal(r, &structured); err == nil {
			if structured.Skill == "" {
				var m map[string]interface{}
				if json.Unmarshal(r, &m) == nil {
					if name, ok := m["name"].(string); ok {
						structured.Skill = name
					}
				}
			}
			if structured.Skill != "" {
				structured.Severity = normalizeSeverity(structured.Severity)
				structured.RequirementType = normalizeRequirementType(structured.RequirementType)
				missing = append(missing, structured)
				continue
			}
		}
		var flat string
		if err := json.Unmarshal(r, &flat); err == nil && flat != "" {
			missing = append(missing, MissingSkill{Skill: flat, Severity: "minor", RequirementType: "preferred"})
		}
	}
	for i := range missing {
		missing[i].Skill = NormalizeSkill(missing[i].Skill)
		missing[i].ClusterGroup = GetSkillCategory(missing[i].Skill)
	}
	if mcfg.MaxMissing > 0 && len(missing) > mcfg.MaxMissing {
		missing = missing[:mcfg.MaxMissing]
	}
	if jd != "" {
		missing = keywordBoost(missing, jd)
	}
	return missing
}

// buildSuggestions parses raw suggestion JSON items, capping at 3.
func buildSuggestions(items []json.RawMessage) []ResumeSuggestion {
	var suggestions []ResumeSuggestion
	for _, raw := range items {
		var s ResumeSuggestion
		if err := json.Unmarshal(raw, &s); err == nil && s.Detail != "" {
			suggestions = append(suggestions, s)
			continue
		}
		var str string
		if err := json.Unmarshal(raw, &str); err == nil && str != "" {
			suggestions = append(suggestions, ResumeSuggestion{Title: "Suggestion", Detail: str})
			continue
		}
		log.Printf("→ skipping unparseable suggestion: %s", string(raw))
	}
	if len(suggestions) > 3 {
		suggestions = suggestions[:3]
	}
	return suggestions
}

// ── Single-shot response parser ───────────────────────────────────────────────

func parseLLMResponse(raw, jobDescription string, mcfg ModeConfig) (Analysis, error) {
	// Strip markdown fences
	raw = regexp.MustCompile("```(?:json)?").ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)

	// Greedy extraction: first { to last } (covers 99% of cases)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return Analysis{}, fmt.Errorf("no JSON object found in LLM response")
	}
	greedy := raw[start : end+1]

	// Five-pass parse on greedy candidate
	resp, parseErr := tryParsePasses(greedy)

	// Brace-depth splitter fallback — handles models that concatenate two JSON objects
	if parseErr != nil {
		candidates := splitTopLevelObjects(raw)
		for _, c := range candidates {
			if c == greedy {
				continue // already tried
			}
			if r, err := tryParsePasses(c); err == nil {
				resp = r
				parseErr = nil
				log.Printf("→ brace-depth splitter found valid JSON (%d candidates)", len(candidates))
				break
			}
		}
	}

	if parseErr != nil {
		preview := greedy
		if len(preview) > 1000 {
			preview = preview[:1000]
		}
		return Analysis{}, fmt.Errorf("failed to parse LLM JSON: %w\nRaw (first 1000 chars):\n%s", parseErr, preview)
	}

	// Float score: round(float(raw_score)) — handles "score": 4.5 → 5
	scoreFloat, err := resp.Score.Float64()
	if err != nil {
		return Analysis{}, fmt.Errorf("invalid score value %q: %v", resp.Score, err)
	}
	score := int(math.Round(scoreFloat))
	if score < 1 || score > 5 {
		return Analysis{}, fmt.Errorf("score out of range: %v", resp.Score)
	}

	matched    := buildMatchedSkills(resp.MatchedSkills, mcfg)
	missing    := buildMissingSkills(resp.MissingSkills, jobDescription, mcfg)
	adjusted, breakdown := computeAdjustedScore(score, missing)

	var suggestions []ResumeSuggestion
	if mcfg.Suggestions {
		suggestions = buildSuggestions(resp.Suggestions)
	}

	return Analysis{
		Score:            score,
		AdjustedScore:    adjusted,
		PenaltyBreakdown: breakdown,
		MatchedSkills:    matched,
		MissingSkills:    missing,
		Reasoning:        resp.Reasoning,
		Suggestions:      suggestions,
	}, nil
}

// ── Chunk parsers (used by callOllamaChunked) ─────────────────────────────────

// parseScoreChunk parses a chunk-1 response (score + reasoning).
// Applies repairTruncatedJSON first — llama3.2:3b frequently truncates before
// the closing brace. Then runs the four-pass pipeline.
func parseScoreChunk(raw string) (int, string, error) {
	raw = regexp.MustCompile("```(?:json)?").ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)
	raw = repairTruncatedJSON(raw) // handle llama3.2:3b truncation

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return 0, "", fmt.Errorf("no JSON object in score chunk")
	}
	candidate := raw[start : end+1]

	var resp struct {
		Score     json.Number `json:"score"`
		Reasoning string      `json:"reasoning"`
	}

	passes := []func(string) string{
		func(s string) string { return s },
		escapeControlChars,
		sanitizeJSON,
		func(s string) string { return sanitizeJSON(repairTruncatedJSON(s)) },
	}
	var lastErr error
	for _, pass := range passes {
		if err := json.Unmarshal([]byte(pass(candidate)), &resp); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return 0, "", fmt.Errorf("score chunk parse failed: %w", lastErr)
	}

	scoreFloat, err := resp.Score.Float64()
	if err != nil {
		return 0, "", fmt.Errorf("invalid score value %q: %w", resp.Score, err)
	}
	score := int(math.Round(scoreFloat))
	if score < 1 || score > 5 {
		return 0, "", fmt.Errorf("score %d out of range 1-5", score)
	}
	return score, resp.Reasoning, nil
}

// extractCompleteArrayItems finds all syntactically complete {…} objects within
// the array value for the given key. Used as last-resort recovery when the array
// is truncated or contains debris (e.g. phi3.5 injects garbage mid-JSON).
func extractCompleteArrayItems(raw, key string) []json.RawMessage {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*\[`)
	loc := re.FindStringIndex(raw)
	if loc == nil {
		return nil
	}

	var items []json.RawMessage
	depth := 0
	itemStart := -1

	for i := loc[1] - 1; i < len(raw); i++ {
		c := raw[i]
		if c == '[' {
			if depth == 0 {
				depth = 1 // array open
			} else {
				depth++ // nested array
			}
		} else if c == '{' && depth >= 1 {
			if depth == 1 {
				itemStart = i
			}
			depth++
		} else if c == '}' && depth >= 2 {
			depth--
			if depth == 1 && itemStart != -1 {
				items = append(items, json.RawMessage(raw[itemStart:i+1]))
				itemStart = -1
			}
		} else if c == ']' && depth >= 1 {
			depth--
			if depth == 0 {
				return items
			}
		}
	}
	return items
}

// parseChunkArray extracts a JSON array from a single-key chunk response
// (e.g. {"matched_skills": [...]}). Runs the four-pass repair pipeline then
// falls back to extractCompleteArrayItems for partially-truncated responses.
func parseChunkArray(raw, key, chunkName string) ([]json.RawMessage, error) {
	raw = regexp.MustCompile("```(?:json)?").ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)
	raw = repairTruncatedJSON(raw)

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		// Try last-resort directly on raw before giving up
		if items := extractCompleteArrayItems(raw, key); len(items) > 0 {
			log.Printf("→ %s last-resort: %d items (no outer object)", chunkName, len(items))
			return items, nil
		}
		return nil, fmt.Errorf("no JSON object in %s chunk", chunkName)
	}
	candidate := raw[start : end+1]

	passes := []func(string) string{
		func(s string) string { return s },
		escapeControlChars,
		sanitizeJSON,
		func(s string) string { return sanitizeJSON(repairTruncatedJSON(s)) },
	}

	var wrapper map[string]json.RawMessage
	var lastErr error
	for _, pass := range passes {
		if err := json.Unmarshal([]byte(pass(candidate)), &wrapper); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
		}
	}

	if lastErr == nil {
		if arr, ok := wrapper[key]; ok {
			var items []json.RawMessage
			if err := json.Unmarshal(arr, &items); err == nil {
				return items, nil
			}
		}
	}

	// Last-resort: extract only complete {…} items from array content
	if items := extractCompleteArrayItems(raw, key); len(items) > 0 {
		log.Printf("→ %s last-resort: %d complete items extracted", chunkName, len(items))
		return items, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%s chunk parse failed: %w", chunkName, lastErr)
	}
	return nil, fmt.Errorf("%s chunk: key %q not found or empty", chunkName, key)
}
