package server

import "strings"

// SkillAliases maps variant forms (lowercase) to canonical names.
var SkillAliases = map[string]string{
	"js":             "JavaScript",
	"javascript":     "JavaScript",
	"ts":             "TypeScript",
	"typescript":     "TypeScript",
	"postgres":       "PostgreSQL",
	"postgresql":     "PostgreSQL",
	"psql":           "PostgreSQL",
	"k8s":            "Kubernetes",
	"kubernetes":     "Kubernetes",
	"rest api":       "REST APIs",
	"rest apis":      "REST APIs",
	"restful":        "REST APIs",
	"restful api":    "REST APIs",
	"ci/cd":          "CI/CD",
	"cicd":           "CI/CD",
	"node":           "Node.js",
	"node.js":        "Node.js",
	"nodejs":         "Node.js",
	"react.js":       "React",
	"reactjs":        "React",
	"vue.js":         "Vue",
	"vuejs":          "Vue",
	"ml":             "Machine Learning",
	"ai/ml":          "AI/ML",
	"llms":           "LLMs",
	"large language": "LLMs",
	"gcp":            "Google Cloud",
	"aws":            "AWS",
	"amazon web":     "AWS",
	"azure":          "Azure",
	"docker":         "Docker",
	"containers":     "Docker",
	"terraform":      "Terraform",
	"iac":            "Infrastructure as Code",
	"mongo":          "MongoDB",
	"mongodb":        "MongoDB",
	"redis":          "Redis",
	"elasticsearch":  "Elasticsearch",
	"elastic":        "Elasticsearch",
	"splunk":         "Splunk",
	"security+":      "CompTIA Security+",
	"sec+":           "CompTIA Security+",
}

// SkillCategories maps canonical skill names to categories.
var SkillCategories = map[string]string{
	"JavaScript":        "frontend",
	"TypeScript":        "frontend",
	"React":             "frontend",
	"Angular":           "frontend",
	"Vue":               "frontend",
	"HTML":              "frontend",
	"CSS":               "frontend",
	"Python":            "backend",
	"Go":                "backend",
	"Java":              "backend",
	"Node.js":           "backend",
	"Flask":             "backend",
	"FastAPI":           "backend",
	"Spring Boot":       "backend",
	"REST APIs":         "backend",
	"PostgreSQL":        "database",
	"MySQL":             "database",
	"SQLite":            "database",
	"MongoDB":           "database",
	"Redis":             "database",
	"Elasticsearch":     "database",
	"AWS":               "cloud",
	"Azure":             "cloud",
	"Google Cloud":      "cloud",
	"Docker":            "devops",
	"Kubernetes":        "devops",
	"CI/CD":             "devops",
	"Jenkins":           "devops",
	"Terraform":         "devops",
	"Splunk":            "security",
	"CompTIA Security+": "security",
	"IAM":               "security",
	"Zero Trust":        "security",
	"LLMs":              "ai",
	"Machine Learning":  "ai",
	"Anthropic":         "ai",
	"Ollama":            "ai",
	"RAG":               "ai",
}

// NormalizeSkill returns the canonical form of a skill name.
// If the skill is not in the alias map it is returned as-is.
func NormalizeSkill(skill string) string {
	lower := strings.ToLower(strings.TrimSpace(skill))
	if canonical, ok := SkillAliases[lower]; ok {
		return canonical
	}
	return skill
}

// GetSkillCategory returns the category for a skill, or "other".
func GetSkillCategory(skill string) string {
	normalized := NormalizeSkill(skill)
	if cat, ok := SkillCategories[normalized]; ok {
		return cat
	}
	return "other"
}
