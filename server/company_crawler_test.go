package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── mock server helper ────────────────────────────────────────────────────────

func newCrawlMock(t *testing.T, htmlContent string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(statusCode)
		w.Write([]byte(htmlContent))
	}))
}

// ── HTML helper unit tests ────────────────────────────────────────────────────

func TestParseFloat_ValidFloat(t *testing.T) {
	if f := parseFloat("4.2 stars"); f != 4.2 {
		t.Errorf("expected 4.2, got %f", f)
	}
}

func TestParseFloat_EmptyString(t *testing.T) {
	if f := parseFloat(""); f != 0 {
		t.Errorf("expected 0, got %f", f)
	}
}

func TestParseInt_WithCommas(t *testing.T) {
	if i := parseInt("1,234 reviews"); i != 1234 {
		t.Errorf("expected 1234, got %d", i)
	}
}

func TestParseInt_EmptyString(t *testing.T) {
	if i := parseInt(""); i != 0 {
		t.Errorf("expected 0, got %d", i)
	}
}

// ── crawlGlassdoorURL ─────────────────────────────────────────────────────────

const testGlassdoorHTML = `
<html><body>
  <a data-test="cell-Employment-Overview-company-name" href="/Overview/Working-at-Acme-EI_IE123.htm">Acme</a>
  <span data-test="rating">4.2</span>
  <span data-test="reviewsCount">1,234 reviews</span>
</body></html>`

func TestCrawlGlassdoor_ReturnsURLRatingReviewCount(t *testing.T) {
	srv := newCrawlMock(t, testGlassdoorHTML, 200)
	defer srv.Close()

	result := crawlGlassdoorURL("Acme", srv.URL+"/search")
	if result.GlassdoorURL == "" {
		t.Error("expected glassdoor_url")
	}
	if !strings.Contains(result.GlassdoorURL, "Working-at") {
		t.Errorf("expected Working-at in URL, got %q", result.GlassdoorURL)
	}
	if result.GlassdoorRating != 4.2 {
		t.Errorf("expected rating 4.2, got %f", result.GlassdoorRating)
	}
	if result.GlassdoorReviewCount != 1234 {
		t.Errorf("expected 1234 reviews, got %d", result.GlassdoorReviewCount)
	}
}

func TestCrawlGlassdoor_EmptyHTMLReturnsEmptyResult(t *testing.T) {
	srv := newCrawlMock(t, `<html><body><p>No results.</p></body></html>`, 200)
	defer srv.Close()

	result := crawlGlassdoorURL("Unknown", srv.URL+"/search")
	if result.GlassdoorURL != "" || result.GlassdoorRating != 0 {
		t.Error("expected empty result for no-results page")
	}
}

func TestCrawlGlassdoor_HTTP403ReturnsEmptyResult(t *testing.T) {
	srv := newCrawlMock(t, "", 403)
	defer srv.Close()

	result := crawlGlassdoorURL("Acme", srv.URL+"/search")
	if result.GlassdoorURL != "" {
		t.Error("expected empty result for 403 response")
	}
}

func TestCrawlGlassdoor_NetworkErrorReturnsEmptyResult(t *testing.T) {
	result := crawlGlassdoorURL("Acme", "http://127.0.0.1:19997/search")
	if result.GlassdoorURL != "" {
		t.Error("expected empty result for unreachable server")
	}
}

// ── crawlLinkedInURL ──────────────────────────────────────────────────────────

const testLinkedInHTML = `
<html><head>
  <meta name="description" content="Acme Corp | 1,200 employees on LinkedIn">
</head><body>
  <span>Founded 2010</span>
</body></html>`

func TestCrawlLinkedIn_ReturnsURLEmployeeCountFounded(t *testing.T) {
	srv := newCrawlMock(t, testLinkedInHTML, 200)
	defer srv.Close()

	result := crawlLinkedInURL("Acme", srv.URL+"/company/acme")
	if result.LinkedInURL == "" {
		t.Error("expected linkedin_url")
	}
	if result.LinkedInEmployees != "1200" {
		t.Errorf("expected '1200' employees, got %q", result.LinkedInEmployees)
	}
	if result.LinkedInFounded != "2010" {
		t.Errorf("expected founded 2010, got %q", result.LinkedInFounded)
	}
}

func TestCrawlLinkedIn_HTTP404ReturnsEmptyResult(t *testing.T) {
	srv := newCrawlMock(t, "", 404)
	defer srv.Close()

	result := crawlLinkedInURL("Ghost Corp", srv.URL+"/company/ghost")
	if result.LinkedInURL != "" {
		t.Error("expected empty result for 404")
	}
}

func TestCrawlLinkedIn_NoEmployeeDataReturnsURL(t *testing.T) {
	srv := newCrawlMock(t, `<html><body><p>Hello</p></body></html>`, 200)
	defer srv.Close()

	result := crawlLinkedInURL("Acme", srv.URL+"/company/acme")
	if result.LinkedInURL == "" {
		t.Error("expected linkedin_url even with no employee data")
	}
	if result.LinkedInEmployees != "" {
		t.Errorf("expected empty employees, got %q", result.LinkedInEmployees)
	}
}

func TestCrawlLinkedIn_NetworkErrorReturnsEmptyResult(t *testing.T) {
	result := crawlLinkedInURL("Acme", "http://127.0.0.1:19997/company/acme")
	if result.LinkedInURL != "" {
		t.Error("expected empty result for unreachable server")
	}
}

// ── crawlBBBURL ───────────────────────────────────────────────────────────────

const testBBBHTML = `
<html><body>
  <a href="/profile/acme-corp-123">Acme Corp</a>
  <span class="dtm-rating">A+</span>
</body></html>`

func TestCrawlBBB_ReturnsURLAndGrade(t *testing.T) {
	srv := newCrawlMock(t, testBBBHTML, 200)
	defer srv.Close()

	result := crawlBBBURL("Acme", srv.URL+"/search")
	if result.BBBURL == "" {
		t.Error("expected bbb_url")
	}
	if !strings.Contains(result.BBBURL, "/profile/") {
		t.Errorf("expected /profile/ in URL, got %q", result.BBBURL)
	}
	if result.BBBRating != "A+" {
		t.Errorf("expected grade A+, got %q", result.BBBRating)
	}
}

func TestCrawlBBB_EmptyResultsReturnsEmptyStruct(t *testing.T) {
	srv := newCrawlMock(t, `<html><body><p>No results.</p></body></html>`, 200)
	defer srv.Close()

	result := crawlBBBURL("Unknown", srv.URL+"/search")
	if result.BBBURL != "" || result.BBBRating != "" {
		t.Error("expected empty result for no-results page")
	}
}

func TestCrawlBBB_NetworkErrorReturnsEmptyResult(t *testing.T) {
	result := crawlBBBURL("Acme", "http://127.0.0.1:19997/search")
	if result.BBBURL != "" {
		t.Error("expected empty result for unreachable server")
	}
}

func TestCrawlBBB_HTTP403ReturnsEmptyResult(t *testing.T) {
	srv := newCrawlMock(t, "", 403)
	defer srv.Close()

	result := crawlBBBURL("Acme", srv.URL+"/search")
	if result.BBBURL != "" {
		t.Error("expected empty result for 403 response")
	}
}

// ── Combined HTML (all three crawlers) ───────────────────────────────────────

const testCombinedHTML = `
<html><head>
  <meta name="description" content="Acme | 500 employees on LinkedIn">
</head><body>
  <a data-test="cell-Employment-Overview-company-name" href="/Overview/Working-at-Acme.htm">Acme</a>
  <span data-test="rating">3.9</span>
  <a href="/profile/acme-123">Acme BBB</a>
  <span class="dtm-rating">B+</span>
  <span>Founded 2005</span>
</body></html>`

func TestCrawlCombined_AllThreeCrawlersExtractCorrectly(t *testing.T) {
	srv := newCrawlMock(t, testCombinedHTML, 200)
	defer srv.Close()

	gd := crawlGlassdoorURL("Acme", srv.URL)
	li := crawlLinkedInURL("Acme", srv.URL)
	bbb := crawlBBBURL("Acme", srv.URL)

	if gd.GlassdoorURL == "" {
		t.Error("expected glassdoor_url from combined HTML")
	}
	if gd.GlassdoorRating != 3.9 {
		t.Errorf("expected rating 3.9, got %f", gd.GlassdoorRating)
	}
	if li.LinkedInEmployees != "500" {
		t.Errorf("expected 500 employees, got %q", li.LinkedInEmployees)
	}
	if li.LinkedInFounded != "2005" {
		t.Errorf("expected founded 2005, got %q", li.LinkedInFounded)
	}
	if bbb.BBBRating != "B+" {
		t.Errorf("expected grade B+, got %q", bbb.BBBRating)
	}
}

// ── CrawlCompany orchestrator ─────────────────────────────────────────────────

func TestCrawlCompany_DoesNotPanicOnTotalFailure(t *testing.T) {
	// All real URLs will fail in test environment — just verify no panic
	result := CrawlCompany("Ghost Corp XYZ Nonexistent 99999")
	_ = result
}
