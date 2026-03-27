package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var scraperClient = &http.Client{
	Timeout: 20 * time.Second,
}

var scraperHeaders = map[string]string{
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
	"Accept-Language": "en-US,en;q=0.9",
}

// noiseTags are stripped before text extraction
var noiseTags = map[string]bool{
	"script": true, "style": true, "nav": true,
	"footer": true, "header": true, "noscript": true,
	"iframe": true, "form": true,
}

// ScrapeJob fetches a URL and extracts the job description and metadata.
func ScrapeJob(jobURL string) (title, company, location, description string, err error) {
	log.Printf("→ Scraping: %s", jobURL)
	req, err := http.NewRequest("GET", jobURL, nil)
	if err != nil {
		return "", "", "", "", fmt.Errorf("invalid URL: %w", err)
	}
	for k, v := range scraperHeaders {
		req.Header.Set(k, v)
	}

	resp, err := scraperClient.Do(req)
	if err != nil {
		return "", "", "", "", fmt.Errorf("network error fetching URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", "", "", "", fmt.Errorf("HTTP %d fetching URL: %s", resp.StatusCode, jobURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB max
	if err != nil {
		return "", "", "", "", fmt.Errorf("error reading response: %w", err)
	}
	log.Printf("→ Parsing HTML (%d bytes) from %s", len(body), jobURL)

	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return "", "", "", "", fmt.Errorf("error parsing HTML: %w", err)
	}

	title, company, location = extractMeta(doc)
	description = extractDescription(doc)
	log.Printf("→ Extracted: title=%q company=%q desc_len=%d", title, company, len(description))

	if len(description) < 100 {
		log.Printf("✗ Description too short (%d chars) for %s", len(description), jobURL)
		return "", "", "", "", fmt.Errorf(
			"could not extract job description — page may require JavaScript or login. " +
				"Try pasting the description manually using the Paste tab",
		)
	}

	if len(description) > 8000 {
		description = description[:8000] + "\n\n[...truncated for analysis]"
		log.Printf("→ Description truncated to 8000 chars")
	}

	log.Printf("✓ Scrape complete: %q (%s) desc=%d chars", title, company, len(description))
	return title, company, location, description, nil
}

// extractMeta pulls title, company, location from meta tags and page structure.
// Priority: og:title > meta name=title > <title> tag > <h1>
func extractMeta(doc *html.Node) (title, company, location string) {
	var ogTitle, metaTitle, titleTag, h1Title string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "meta":
				prop := attrVal(n, "property")
				name := strings.ToLower(attrVal(n, "name"))
				content := attrVal(n, "content")

				if prop == "og:title" {
					ogTitle = content
				}
				if strings.Contains(name, "title") {
					metaTitle = content
				}
				if strings.Contains(name, "company") || strings.Contains(name, "employer") {
					company = content
				}
				if strings.Contains(name, "location") || strings.Contains(name, "city") {
					location = content
				}

			case "title":
				if n.FirstChild != nil {
					titleTag = strings.TrimSpace(n.FirstChild.Data)
				}

			case "h1":
				if h1Title == "" {
					h1Title = cleanText(textContent(n))
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Apply priority: og:title > meta name=title > <title> > <h1>
	switch {
	case ogTitle != "":
		title = ogTitle
	case metaTitle != "":
		title = metaTitle
	case titleTag != "":
		title = titleTag
	case h1Title != "":
		title = h1Title
	}

	if len(title) > 200 {
		title = title[:200]
	}
	if len(company) > 200 {
		company = company[:200]
	}
	if len(location) > 200 {
		location = location[:200]
	}
	return
}

// contentSelectors lists CSS-like selectors tried in order of specificity.
// Since we're using golang.org/x/net/html (not a full CSS selector engine),
// we match by attribute values and element IDs manually.
func extractDescription(doc *html.Node) string {
	// Remove noise tags first
	removeNoise(doc)

	// Try targeted selectors (Indeed, LinkedIn, Greenhouse, Lever, generic)
	if el := findByAttr(doc, "div", "data-testid", "jobDescriptionText"); el != nil {
		if t := cleanText(textContent(el)); len(t) > 200 {
			return t
		}
	}
	if el := findByClass(doc, "description__text"); el != nil {
		if t := cleanText(textContent(el)); len(t) > 200 {
			return t
		}
	}
	if el := findByID(doc, "job-details"); el != nil {
		if t := cleanText(textContent(el)); len(t) > 200 {
			return t
		}
	}
	if el := findByClass(doc, "jobsearch-jobDescriptionText"); el != nil {
		if t := cleanText(textContent(el)); len(t) > 200 {
			return t
		}
	}
	for _, cls := range []string{"job-description", "jobDescription", "description"} {
		if el := findByClass(doc, cls); el != nil {
			if t := cleanText(textContent(el)); len(t) > 200 {
				return t
			}
		}
	}
	// Fallback to <main> then <article> then <body>
	for _, tag := range []string{"main", "article", "body"} {
		if el := findTag(doc, tag); el != nil {
			if t := cleanText(textContent(el)); len(t) > 100 {
				return t
			}
		}
	}
	return ""
}

// ── HTML traversal helpers ────────────────────────────────────────────────────

func removeNoise(n *html.Node) {
	var toRemove []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && noiseTags[node.Data] {
			toRemove = append(toRemove, node)
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	for _, node := range toRemove {
		if node.Parent != nil {
			node.Parent.RemoveChild(node)
		}
	}
}

func findByAttr(n *html.Node, tag, attr, val string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		for _, a := range n.Attr {
			if a.Key == attr && a.Val == val {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByAttr(c, tag, attr, val); found != nil {
			return found
		}
	}
	return nil
}

func findByClass(n *html.Node, cls string) *html.Node {
	if n.Type == html.ElementNode {
		for _, a := range n.Attr {
			if a.Key == "class" && strings.Contains(a.Val, cls) {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByClass(c, cls); found != nil {
			return found
		}
	}
	return nil
}

func findByID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode {
		for _, a := range n.Attr {
			if a.Key == "id" && a.Val == id {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func findTag(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findTag(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
			b.WriteString(" ")
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4":
				b.WriteString("\n")
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

var multiNewline = regexp.MustCompile(`\n{3,}`)
var multiSpace = regexp.MustCompile(`[ \t]{2,}`)

func cleanText(s string) string {
	s = multiNewline.ReplaceAllString(s, "\n\n")
	s = multiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

var buzzwordList = []string{
	"synergy", "leverage", "paradigm", "holistic", "proactive",
	"dynamic", "innovative", "passionate", "rockstar", "ninja",
	"guru", "wizard", "thought leader", "disruptive", "agile mindset",
}

// techKeywordList is a sample of recognizable tech terms for density checks.
var techKeywordList = []string{
	"python", "go", "java", "javascript", "typescript", "react", "angular", "vue",
	"node", "docker", "kubernetes", "aws", "azure", "gcp", "terraform", "ci/cd",
	"sql", "postgresql", "mysql", "mongodb", "redis", "elasticsearch",
	"api", "rest", "graphql", "microservices", "linux", "git", "jenkins",
	"splunk", "iam", "terraform", "ansible", "prometheus", "grafana",
}

// assessJobTextQuality runs deterministic checks on a job description and
// returns a quality level ("ok", "warn", "poor") with a list of issues found.
func assessJobTextQuality(text string) JobTextQuality {
	q := JobTextQuality{
		Level:     "ok",
		CharCount: len(text),
	}
	lower := strings.ToLower(text)

	if len(text) < 300 {
		q.Issues = append(q.Issues, "Description too short — analysis reliability may be reduced")
	}

	// Count non-ASCII characters
	total := len([]rune(text))
	if total > 0 {
		nonASCII := 0
		for _, r := range text {
			if r > 127 {
				nonASCII++
			}
		}
		if float64(nonASCII)/float64(total) > 0.15 {
			q.Issues = append(q.Issues, "High proportion of non-ASCII characters — possible scraping noise")
		}
	}

	// Count bullet lines
	bulletCount := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "•") || strings.HasPrefix(trimmed, "-") ||
			strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "·") {
			bulletCount++
		}
	}
	if bulletCount > 60 {
		q.Issues = append(q.Issues, "Bullet-heavy description — limited narrative context for analysis")
	}

	// Mixed seniority signals
	juniorSignals := strings.Contains(lower, "junior") || strings.Contains(lower, "entry level") || strings.Contains(lower, "entry-level")
	seniorSignals := strings.Contains(lower, "senior") || strings.Contains(lower, "5+ years") || strings.Contains(lower, "7+ years")
	if juniorSignals && seniorSignals {
		q.Issues = append(q.Issues, "Mixed seniority signals (e.g. 'junior' and 'senior' or high year requirements)")
	}

	// Buzzword density
	words := strings.Fields(lower)
	buzzCount := 0
	for _, bw := range buzzwordList {
		if strings.Contains(lower, bw) {
			buzzCount++
		}
	}
	q.BuzzwordCount = buzzCount
	if len(words) > 0 && float64(buzzCount)/float64(len(words)) > 0.15 {
		q.Issues = append(q.Issues, "High buzzword density — description may lack concrete technical requirements")
	}

	// Tech keyword count
	techCount := 0
	for _, kw := range techKeywordList {
		if strings.Contains(lower, kw) {
			techCount++
		}
	}
	q.TechKeywords = techCount
	if len(text) > 500 && techCount == 0 {
		q.Issues = append(q.Issues, "No recognized tech keywords detected — description may be too vague for reliable analysis")
	}

	// Determine level
	switch len(q.Issues) {
	case 0:
		q.Level = "ok"
	case 1:
		q.Level = "warn"
	default:
		q.Level = "poor"
	}

	return q
}
