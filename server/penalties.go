package server

import "strings"

func normalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "blocker", "critical", "must", "required", "mandatory":
		return "blocker"
	case "major", "high", "significant", "important":
		return "major"
	default:
		return "minor"
	}
}

func normalizeRequirementType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hard", "required", "mandatory", "must":
		return "hard"
	case "bonus", "optional", "nice-to-have", "plus":
		return "bonus"
	default:
		return "preferred"
	}
}

func normalizeMatchType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "exact", "direct", "verbatim", "full":
		return "exact"
	case "partial", "related", "similar", "close":
		return "partial"
	default:
		return "inferred"
	}
}

// penaltyForSkill returns the penalty points for a single missing skill.
// Bonus requirement type always returns 0 regardless of severity.
func penaltyForSkill(skill MissingSkill) int {
	if skill.RequirementType == "bonus" {
		return 0
	}
	switch skill.Severity {
	case "blocker":
		return 2
	case "major":
		return 1
	default: // minor
		return 0 // minors are aggregated by count in computeAdjustedScore
	}
}

// keywordBoost upgrades severity of missing skills matching hard-blocker patterns.
func keywordBoost(skills []MissingSkill, jd string) []MissingSkill {
	jdLower := strings.ToLower(jd)
	jdHasBlocker := false
	for _, kw := range blockerKeywords {
		if strings.Contains(jdLower, kw) {
			jdHasBlocker = true
			break
		}
	}
	jdHasYears := yearPattern.MatchString(jdLower)

	result := make([]MissingSkill, len(skills))
	for i, s := range skills {
		skillLower := strings.ToLower(s.Skill)
		severity := s.Severity

		for _, kw := range blockerKeywords {
			if strings.Contains(skillLower, kw) {
				severity = "blocker"
				break
			}
		}
		if yearPattern.MatchString(skillLower) && jdHasYears {
			severity = "blocker"
		}
		if severity == "major" && jdHasBlocker &&
			(strings.Contains(skillLower, "required") || strings.Contains(skillLower, "must")) {
			severity = "blocker"
		}
		// Preserve all fields; only overwrite severity
		result[i] = s
		result[i].Severity = severity
	}
	return result
}

// clusterPenaltyCap returns the maximum total penalty allowed for a skill cluster group.
func clusterPenaltyCap(group string) int {
	if group == "security" {
		return 2
	}
	return 1
}

// computeAdjustedScore applies the full penalty pipeline with per-cluster caps.
func computeAdjustedScore(rawScore int, missing []MissingSkill) (int, PenaltyBreakdown) {
	// Ensure ClusterGroup is set on all skills
	for i := range missing {
		if missing[i].ClusterGroup == "" {
			missing[i].ClusterGroup = GetSkillCategory(missing[i].Skill)
		}
	}

	// Count severity totals and group by cluster
	var blockers, majors, minors int
	type clusterData struct{ rawPenalty int }
	clusters := map[string]*clusterData{}

	for _, s := range missing {
		switch s.Severity {
		case "blocker":
			blockers++
		case "major":
			majors++
		default:
			minors++
		}
		p := penaltyForSkill(s)
		if p > 0 {
			if clusters[s.ClusterGroup] == nil {
				clusters[s.ClusterGroup] = &clusterData{}
			}
			clusters[s.ClusterGroup].rawPenalty += p
		}
	}

	// Cap each cluster and sum up
	clusterPenalties := map[string]int{}
	clusterTotal := 0
	for group, data := range clusters {
		cap := clusterPenaltyCap(group)
		capped := data.rawPenalty
		if capped > cap {
			capped = cap
		}
		clusterPenalties[group] = capped
		clusterTotal += capped
	}

	// For the breakdown display, report raw severity penalties capped globally
	bp := blockers * 2
	if bp > 3 {
		bp = 3
	}
	mp := majors * 1
	if mp > 2 {
		mp = 2
	}
	mnp := minors / 2
	if mnp > 1 {
		mnp = 1
	}
	cp := 0
	if len(missing) > 6 {
		cp = 1
	}

	total := clusterTotal + mnp + cp
	adjusted := rawScore - total
	if adjusted < 1 {
		adjusted = 1
	}

	return adjusted, PenaltyBreakdown{
		Blockers:       blockers,
		Majors:         majors,
		Minors:         minors,
		BlockerPenalty: bp,
		MajorPenalty:   mp,
		MinorPenalty:   mnp,
		CountPenalty:   cp,
		TotalPenalty:   total,
		Clusters:       clusterPenalties,
	}
}
