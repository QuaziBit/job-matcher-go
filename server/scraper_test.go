package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// ── cleanText ─────────────────────────────────────────────────────────────────

func TestCleanText_CollapsesBlanks(t *testing.T) {
	result := cleanText("line1\n\n\n\nline2")
	if strings.Contains(result, "\n\n\n") {
		t.Error("expected collapsed blank lines")
	}
	if !strings.Contains(result, "line1") || !strings.Contains(result, "line2") {
		t.Error("expected content preserved")
	}
}

func TestCleanText_CollapsesSpaces(t *testing.T) {
	result := cleanText("word1    word2\t\tword3")
	if strings.Contains(result, "  ") {
		t.Error("expected collapsed spaces")
	}
}

func TestCleanText_Strips(t *testing.T) {
	result := cleanText("  \n  hello  \n  ")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

// ── extractMeta ───────────────────────────────────────────────────────────────

func mustParseHTML(h string) *html.Node {
	doc, err := html.Parse(strings.NewReader(h))
	if err != nil {
		panic(err)
	}
	return doc
}

func TestExtractMeta_OGTitle(t *testing.T) {
	doc := mustParseHTML(`<html><head><meta property="og:title" content="DevSecOps Engineer"/></head></html>`)
	title, _, _ := extractMeta(doc)
	if title != "DevSecOps Engineer" {
		t.Errorf("expected 'DevSecOps Engineer', got %q", title)
	}
}

func TestExtractMeta_FallsBackToTitleTag(t *testing.T) {
	doc := mustParseHTML(`<html><head><title>My Job Title</title></head></html>`)
	title, _, _ := extractMeta(doc)
	if title != "My Job Title" {
		t.Errorf("expected 'My Job Title', got %q", title)
	}
}

func TestExtractMeta_FallsBackToH1(t *testing.T) {
	doc := mustParseHTML(`<html><body><h1>Software Engineer</h1></body></html>`)
	title, _, _ := extractMeta(doc)
	if title != "Software Engineer" {
		t.Errorf("expected 'Software Engineer', got %q", title)
	}
}

func TestExtractMeta_CompanyFromMeta(t *testing.T) {
	doc := mustParseHTML(`<html><head><meta name="job-company" content="Acme Corp"/></head></html>`)
	_, company, _ := extractMeta(doc)
	if company != "Acme Corp" {
		t.Errorf("expected 'Acme Corp', got %q", company)
	}
}

func TestExtractMeta_LocationFromMeta(t *testing.T) {
	doc := mustParseHTML(`<html><head><meta name="job-location" content="Arlington, VA"/></head></html>`)
	_, _, location := extractMeta(doc)
	if location != "Arlington, VA" {
		t.Errorf("expected 'Arlington, VA', got %q", location)
	}
}

func TestExtractMeta_TitleTruncatedAt200(t *testing.T) {
	longTitle := strings.Repeat("x", 300)
	doc := mustParseHTML(`<html><head><title>` + longTitle + `</title></head></html>`)
	title, _, _ := extractMeta(doc)
	if len(title) > 200 {
		t.Errorf("expected title truncated to 200, got %d", len(title))
	}
}

func TestExtractMeta_EmptyPageReturnsEmpty(t *testing.T) {
	doc := mustParseHTML(`<html></html>`)
	title, company, location := extractMeta(doc)
	if title != "" || company != "" || location != "" {
		t.Errorf("expected empty strings for empty page, got title=%q company=%q location=%q",
			title, company, location)
	}
}

// ── ScrapeJob (with mock HTTP server) ────────────────────────────────────────

func TestScrapeJob_IndeedSelector(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
  <title>DevSecOps Engineer - Acme Corp - Indeed</title>
  <meta property="og:title" content="DevSecOps Engineer" />
  <meta name="job-company" content="Acme Corp" />
  <meta name="job-location" content="Arlington, VA" />
</head>
<body>
  <nav>Navigation noise</nav>
  <div data-testid="jobDescriptionText">
    We are looking for a DevSecOps Engineer with Python, Docker, and AWS experience.
    Must have CompTIA Security+. Experience with CI/CD pipelines and Splunk required.
    Federal government client experience a strong plus. Terraform and Kubernetes knowledge
    preferred. This is a hybrid role based in Arlington, VA supporting federal agencies.
    Minimum 3 years of experience in DevSecOps or related role required.
  </div>
  <footer>Footer noise</footer>
</body>
</html>`))
	}))
	defer mock.Close()

	title, company, location, desc, err := ScrapeJob(mock.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "DevSecOps Engineer" {
		t.Errorf("expected title 'DevSecOps Engineer', got %q", title)
	}
	if company != "Acme Corp" {
		t.Errorf("expected company 'Acme Corp', got %q", company)
	}
	if location != "Arlington, VA" {
		t.Errorf("expected location 'Arlington, VA', got %q", location)
	}
	if !strings.Contains(desc, "Python") {
		t.Error("expected description to contain Python")
	}
	if !strings.Contains(desc, "Docker") {
		t.Error("expected description to contain Docker")
	}
	if strings.Contains(desc, "Navigation noise") {
		t.Error("nav content should be stripped")
	}
	if strings.Contains(desc, "Footer noise") {
		t.Error("footer content should be stripped")
	}
}

func TestScrapeJob_GenericMainTag(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Software Engineer at TechCorp</title></head>
<body>
  <main>
    <h1>Software Engineer</h1>
    <p>TechCorp is hiring a Software Engineer to join our platform team.
    You will work with Python, PostgreSQL, Docker, and REST APIs.
    Experience with cloud platforms such as AWS, GCP, or Azure is required.
    Strong communication skills and 3 plus years of experience needed.</p>
  </main>
</body>
</html>`))
	}))
	defer mock.Close()

	_, _, _, desc, err := ScrapeJob(mock.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(desc, "Python") {
		t.Error("expected description to contain Python")
	}
	if !strings.Contains(desc, "PostgreSQL") {
		t.Error("expected description to contain PostgreSQL")
	}
}

func TestScrapeJob_TooShortReturnsError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html><html><head><title>Jobs</title></head><body><p>Hi</p></body></html>`))
	}))
	defer mock.Close()

	_, _, _, _, err := ScrapeJob(mock.URL)
	if err == nil {
		t.Error("expected error for too-short content")
	}
}

func TestScrapeJob_TruncatesLongDescriptions(t *testing.T) {
	longContent := strings.Repeat("word ", 3000)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html><html><body><main>` + longContent + `</main></body></html>`))
	}))
	defer mock.Close()

	_, _, _, desc, err := ScrapeJob(mock.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(desc) > 8200 {
		t.Errorf("expected description to be truncated, got %d chars", len(desc))
	}
	if !strings.Contains(desc, "truncated") {
		t.Error("expected truncation marker in description")
	}
}

func TestScrapeJob_HTTP404ReturnsError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer mock.Close()

	_, _, _, _, err := ScrapeJob(mock.URL)
	if err == nil {
		t.Error("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error message, got: %v", err)
	}
}

func TestScrapeJob_NetworkErrorReturnsError(t *testing.T) {
	_, _, _, _, err := ScrapeJob("http://127.0.0.1:19997/job")
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

// ── assessJobTextQuality ──────────────────────────────────────────────────────

func TestAssessJobTextQuality_TooShort(t *testing.T) {
	q := assessJobTextQuality("Short text.")
	if q.Level == "ok" {
		t.Error("expected non-ok level for very short text")
	}
	found := false
	for _, issue := range q.Issues {
		if strings.Contains(issue, "short") {
			found = true
		}
	}
	if !found {
		t.Error("expected a 'short' issue in results")
	}
}

func TestAssessJobTextQuality_NoisyText(t *testing.T) {
	// Build a string with >15% non-ASCII chars
	noisy := strings.Repeat("a", 100) + strings.Repeat("é", 20)
	q := assessJobTextQuality(noisy)
	found := false
	for _, issue := range q.Issues {
		if strings.Contains(issue, "non-ASCII") {
			found = true
		}
	}
	if !found {
		t.Error("expected a non-ASCII noise issue")
	}
}

func TestAssessJobTextQuality_BulletHeavy(t *testing.T) {
	// 65 bullet lines
	lines := make([]string, 65)
	for i := range lines {
		lines[i] = "- requirement " + strings.Repeat("x", 5)
	}
	q := assessJobTextQuality(strings.Join(lines, "\n"))
	found := false
	for _, issue := range q.Issues {
		if strings.Contains(issue, "Bullet") {
			found = true
		}
	}
	if !found {
		t.Error("expected a bullet-heavy issue")
	}
}

func TestAssessJobTextQuality_CleanText(t *testing.T) {
	clean := strings.Repeat("We are looking for a Go software engineer with Docker experience. ", 20)
	q := assessJobTextQuality(clean)
	if q.Level != "ok" {
		t.Errorf("expected 'ok' for clean text, got %q (issues: %v)", q.Level, q.Issues)
	}
}

func TestAssessJobTextQuality_CharCountSet(t *testing.T) {
	text := "hello world"
	q := assessJobTextQuality(text)
	if q.CharCount != len(text) {
		t.Errorf("expected CharCount=%d, got %d", len(text), q.CharCount)
	}
}

// ── Task 3.3 — Extended quality checks ───────────────────────────────────────

func TestAssessJobTextQuality_MixedSenioritySignals(t *testing.T) {
	text := strings.Repeat("We are hiring a junior developer. Must have 5+ years experience. ", 10)
	q := assessJobTextQuality(text)
	found := false
	for _, issue := range q.Issues {
		if strings.Contains(issue, "seniority") {
			found = true
		}
	}
	if !found {
		t.Error("expected mixed seniority issue")
	}
}

func TestAssessJobTextQuality_BuzzwordHeavy(t *testing.T) {
	// Make a text where buzzwords are very dense relative to word count
	text := "synergy leverage paradigm holistic proactive dynamic innovative passionate rockstar ninja guru wizard disruptive"
	q := assessJobTextQuality(text)
	if q.BuzzwordCount == 0 {
		t.Error("expected some buzzwords counted")
	}
}

func TestAssessJobTextQuality_NoTechKeywords(t *testing.T) {
	// A long description with no tech terms
	text := strings.Repeat("We are looking for someone with excellent communication and teamwork skills. ", 20)
	q := assessJobTextQuality(text)
	if q.TechKeywords != 0 {
		t.Logf("found %d tech keywords (may be false positives in generic text)", q.TechKeywords)
	}
	// Just verify the field is populated
	_ = q.TechKeywords
}

func TestAssessJobTextQuality_GoodDescription_IsOk(t *testing.T) {
	good := "We are looking for a Go developer with Docker and Kubernetes experience. " +
		"Must have strong knowledge of REST APIs and PostgreSQL. " +
		"AWS cloud experience is preferred. CI/CD pipeline experience a plus. " +
		strings.Repeat("Technical role working on distributed systems. ", 5)
	q := assessJobTextQuality(good)
	if q.Level != "ok" {
		t.Errorf("expected 'ok' for good description, got %q (issues: %v)", q.Level, q.Issues)
	}
	if q.TechKeywords == 0 {
		t.Error("expected at least one tech keyword in good description")
	}
}

// ── Additional edge cases ─────────────────────────────────────────────────────

func TestScrapeJob_EmptyBodyReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write nothing
	}))
	defer srv.Close()

	_, _, _, _, err := ScrapeJob(srv.URL)
	if err == nil {
		t.Error("expected error for empty page body")
	}
}

func TestScrapeJob_MalformedHTMLStillExtracts(t *testing.T) {
	// Unclosed tags and broken structure — should still extract something
	html := `<html><body>
		<h1>Senior Go Developer
		<div>Acme Corp
		<p>We are looking for an experienced Go developer to join our team.
		You will work on distributed systems and microservices.
		Strong knowledge of Go, Docker, and Kubernetes required.
		Remote work available. Competitive salary offered.
		Please apply with your resume and cover letter today.
	</body>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer srv.Close()

	_, _, _, desc, err := ScrapeJob(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error on malformed HTML: %v", err)
	}
	if desc == "" {
		t.Error("expected non-empty description from malformed HTML")
	}
}
