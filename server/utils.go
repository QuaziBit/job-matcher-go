package server

import "fmt"

// hasBlocker returns true if any missing skill has severity "blocker".
func hasBlocker(skills []MissingSkill) bool {
	for _, s := range skills {
		if s.Severity == "blocker" {
			return true
		}
	}
	return false
}

// determineBetterFit compares two analyses and returns (resumeLabel, reason).
func determineBetterFit(a, b Analysis) (string, string) {
	aHasBlocker := hasBlocker(a.MissingSkills)
	bHasBlocker := hasBlocker(b.MissingSkills)

	if aHasBlocker && !bHasBlocker {
		return b.ResumeLabel, "No hard blockers vs " + a.ResumeLabel + " which has blockers"
	}
	if bHasBlocker && !aHasBlocker {
		return a.ResumeLabel, "No hard blockers vs " + b.ResumeLabel + " which has blockers"
	}
	if a.AdjustedScore > b.AdjustedScore {
		return a.ResumeLabel, fmt.Sprintf("Higher adjusted score (%d vs %d)", a.AdjustedScore, b.AdjustedScore)
	}
	if b.AdjustedScore > a.AdjustedScore {
		return b.ResumeLabel, fmt.Sprintf("Higher adjusted score (%d vs %d)", b.AdjustedScore, a.AdjustedScore)
	}
	return "Tie", "Both resumes score equally for this role"
}

// buildComparison builds a side-by-side comparison from two most recent analyses
// with different resume IDs. Returns nil if fewer than 2 distinct resumes exist.
func buildComparison(analyses []Analysis) *ResumeComparison {
	if len(analyses) < 2 {
		return nil
	}
	// Find two most recent analyses with different resume IDs
	seen := map[int64]Analysis{}
	for _, a := range analyses {
		if _, exists := seen[a.ResumeID]; !exists {
			seen[a.ResumeID] = a
		}
		if len(seen) == 2 {
			break
		}
	}
	if len(seen) < 2 {
		return nil
	}
	var ids []int64
	for id := range seen {
		ids = append(ids, id)
	}
	ra, rb := seen[ids[0]], seen[ids[1]]
	better, reason := determineBetterFit(ra, rb)
	return &ResumeComparison{ResumeA: ra, ResumeB: rb, BetterFit: better, BetterReason: reason}
}
