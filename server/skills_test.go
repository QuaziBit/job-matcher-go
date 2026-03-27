package server

import "testing"

func TestNormalizeSkill_KnownAlias(t *testing.T) {
	cases := []struct{ input, want string }{
		{"js", "JavaScript"},
		{"k8s", "Kubernetes"},
		{"postgres", "PostgreSQL"},
		{"ci/cd", "CI/CD"},
		{"node.js", "Node.js"},
		{"react.js", "React"},
	}
	for _, c := range cases {
		got := NormalizeSkill(c.input)
		if got != c.want {
			t.Errorf("NormalizeSkill(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestNormalizeSkill_UnknownPassthrough(t *testing.T) {
	got := NormalizeSkill("SomeObscureTool")
	if got != "SomeObscureTool" {
		t.Errorf("expected passthrough, got %q", got)
	}
}

func TestNormalizeSkill_CaseInsensitive(t *testing.T) {
	cases := []string{"JS", "Js", "jS", "javascript", "JavaScript", "JAVASCRIPT"}
	for _, input := range cases {
		got := NormalizeSkill(input)
		if got != "JavaScript" {
			t.Errorf("NormalizeSkill(%q) = %q, want 'JavaScript'", input, got)
		}
	}
}

func TestNormalizeSkill_Trimming(t *testing.T) {
	got := NormalizeSkill("  js  ")
	if got != "JavaScript" {
		t.Errorf("expected 'JavaScript' after trim, got %q", got)
	}
}

func TestGetSkillCategory_KnownSkill(t *testing.T) {
	cases := []struct{ skill, want string }{
		{"JavaScript", "frontend"},
		{"Python", "backend"},
		{"PostgreSQL", "database"},
		{"AWS", "cloud"},
		{"Docker", "devops"},
		{"Splunk", "security"},
		{"LLMs", "ai"},
	}
	for _, c := range cases {
		got := GetSkillCategory(c.skill)
		if got != c.want {
			t.Errorf("GetSkillCategory(%q) = %q, want %q", c.skill, got, c.want)
		}
	}
}

func TestGetSkillCategory_UnknownSkill_ReturnsOther(t *testing.T) {
	got := GetSkillCategory("SomeRandomThing")
	if got != "other" {
		t.Errorf("expected 'other' for unknown skill, got %q", got)
	}
}

func TestGetSkillCategory_NormalizesAlias(t *testing.T) {
	// "k8s" should normalize to "Kubernetes" which is "devops"
	got := GetSkillCategory("k8s")
	if got != "devops" {
		t.Errorf("expected 'devops' for 'k8s', got %q", got)
	}
}
