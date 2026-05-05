package server

// company_crawler.go — Best-effort web crawlers for company vetting.
//
// Each crawler is independent: returns a partial CompanyCrawlResult on
// success, empty struct on any failure. Results are merged by the caller.
//
// Sources: Glassdoor, LinkedIn, BBB.

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// crawlerClient is a shared HTTP client for all crawlers.
var crawlerClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

var crawlerHeaders = map[string]string{
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Accept-Language": "en-US,en;q=0.9",
	"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
}

// CompanyCrawlResult holds all fields collected across crawlers.
type CompanyCrawlResult struct {
	GlassdoorURL         string
	GlassdoorRating      float64
	GlassdoorReviewCount int
	LinkedInURL          string
	LinkedInEmployees    string
	LinkedInFounded      string
	BBBURL               string
	BBBRating            string
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func crawlerGet(rawURL string) (*html.Node, string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	for k, v := range crawlerHeaders {
		req.Header.Set(k, v)
	}
	resp, err := crawlerClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
	}
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return doc, resp.Request.URL.String(), nil
}

// ── HTML helpers ──────────────────────────────────────────────────────────────

func htmlAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func htmlText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(nd *html.Node) {
		if nd.Type == html.TextNode {
			b.WriteString(nd.Data)
		}
		for c := nd.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}

// findNode returns the first node matching selector logic via BFS.
func findNode(root *html.Node, match func(*html.Node) bool) *html.Node {
	queue := []*html.Node{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if match(n) {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			queue = append(queue, c)
		}
	}
	return nil
}

func hasAttrContaining(n *html.Node, key, substr string) bool {
	return strings.Contains(htmlAttr(n, key), substr)
}

func hasClass(n *html.Node, cls string) bool {
	return hasAttrContaining(n, "class", cls)
}

func nodeText(root *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return strings.TrimSpace(b.String())
}

var reFloat = regexp.MustCompile(`[\d]+\.?\d*`)
var reInt = regexp.MustCompile(`[\d,]+`)
var reYear = regexp.MustCompile(`\b(19|20)\d{2}\b`)
var reEmployees = regexp.MustCompile(`(?i)([\d,]+)\s+employee`)
var reGrade = regexp.MustCompile(`[A-F][+\-]?`)

func parseFloat(s string) float64 {
	m := reFloat.FindString(s)
	if m == "" {
		return 0
	}
	var f float64
	fmt.Sscanf(m, "%f", &f)
	return f
}

func parseInt(s string) int {
	m := reInt.FindString(s)
	if m == "" {
		return 0
	}
	m = strings.ReplaceAll(m, ",", "")
	var i int
	fmt.Sscanf(m, "%d", &i)
	return i
}

// ── crawlGlassdoor ────────────────────────────────────────────────────────────

func crawlGlassdoorURL(companyName, searchURL string) (result CompanyCrawlResult) {
	doc, _, err := crawlerGet(searchURL)
	if err != nil {
		log.Printf("crawlGlassdoor(%q) failed: %v", companyName, err)
		return
	}

	linkNode := findNode(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "a" &&
			(hasAttrContaining(n, "data-test", "cell-Employment-Overview-company-name") ||
				hasAttrContaining(n, "href", "/Overview/Working-at"))
	})
	if linkNode != nil {
		href := htmlAttr(linkNode, "href")
		if strings.HasPrefix(href, "http") {
			result.GlassdoorURL = href
		} else if href != "" {
			result.GlassdoorURL = "https://www.glassdoor.com" + href
		}
	}

	ratingNode := findNode(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode &&
			(hasAttrContaining(n, "data-test", "rating") ||
				hasClass(n, "ratingNumber"))
	})
	if ratingNode != nil {
		if f := parseFloat(nodeText(ratingNode)); f > 0 && f <= 5 {
			result.GlassdoorRating = f
		}
	}

	reviewNode := findNode(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode &&
			(hasAttrContaining(n, "data-test", "reviewsCount") ||
				hasClass(n, "reviewCount"))
	})
	if reviewNode != nil {
		if i := parseInt(nodeText(reviewNode)); i > 0 {
			result.GlassdoorReviewCount = i
		}
	}
	return
}

func crawlGlassdoor(companyName string) CompanyCrawlResult {
	return crawlGlassdoorURL(companyName,
		"https://www.glassdoor.com/Search/results.htm?keyword="+url.QueryEscape(companyName))
}

// ── crawlLinkedIn ─────────────────────────────────────────────────────────────

func crawlLinkedInURL(companyName, companyURL string) (result CompanyCrawlResult) {
	doc, finalURL, err := crawlerGet(companyURL)
	if err != nil {
		log.Printf("crawlLinkedIn(%q) failed: %v", companyName, err)
		return
	}
	result.LinkedInURL = finalURL

	var walkMeta func(*html.Node)
	walkMeta = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			content := htmlAttr(n, "content")
			if m := reEmployees.FindStringSubmatch(content); len(m) > 1 {
				result.LinkedInEmployees = strings.ReplaceAll(m[1], ",", "")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkMeta(c)
		}
	}
	walkMeta(doc)

	var walkText func(*html.Node)
	walkText = func(n *html.Node) {
		if n.Type == html.TextNode {
			if strings.Contains(strings.ToLower(n.Data), "founded") {
				if m := reYear.FindString(n.Data); m != "" {
					result.LinkedInFounded = m
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkText(c)
		}
	}
	walkText(doc)
	return
}

func crawlLinkedIn(companyName string) CompanyCrawlResult {
	slug := regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(companyName), "-")
	slug = strings.Trim(slug, "-")
	return crawlLinkedInURL(companyName,
		"https://www.linkedin.com/company/"+url.QueryEscape(slug))
}

// ── crawlBBB ──────────────────────────────────────────────────────────────────

func crawlBBBURL(companyName, searchURL string) (result CompanyCrawlResult) {
	doc, _, err := crawlerGet(searchURL)
	if err != nil {
		log.Printf("crawlBBB(%q) failed: %v", companyName, err)
		return
	}

	linkNode := findNode(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "a" &&
			hasAttrContaining(n, "href", "/profile/")
	})
	if linkNode != nil {
		href := htmlAttr(linkNode, "href")
		if strings.HasPrefix(href, "http") {
			result.BBBURL = href
		} else if href != "" {
			result.BBBURL = "https://www.bbb.org" + href
		}
	}

	gradeNode := findNode(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode &&
			(hasClass(n, "dtm-rating") ||
				hasClass(n, "grade") ||
				hasClass(n, "rating-letter"))
	})
	if gradeNode != nil {
		if m := reGrade.FindString(nodeText(gradeNode)); m != "" {
			result.BBBRating = m
		}
	}
	return
}

func crawlBBB(companyName string) CompanyCrawlResult {
	return crawlBBBURL(companyName,
		"https://www.bbb.org/search?find_text="+url.QueryEscape(companyName))
}

// ── Orchestrator ──────────────────────────────────────────────────────────────

// CrawlCompany runs all crawlers concurrently and merges results.
// Always returns a result — partial on partial failure, zero on total failure.
func CrawlCompany(companyName string) CompanyCrawlResult {
	var (
		mu      sync.Mutex
		merged  CompanyCrawlResult
		wg      sync.WaitGroup
	)

	type crawlerFn func(string) CompanyCrawlResult
	crawlers := []crawlerFn{crawlGlassdoor, crawlLinkedIn, crawlBBB}

	for _, fn := range crawlers {
		wg.Add(1)
		go func(f crawlerFn) {
			defer wg.Done()
			r := f(companyName)
			mu.Lock()
			defer mu.Unlock()
			if r.GlassdoorURL != "" {
				merged.GlassdoorURL = r.GlassdoorURL
			}
			if r.GlassdoorRating > 0 {
				merged.GlassdoorRating = r.GlassdoorRating
			}
			if r.GlassdoorReviewCount > 0 {
				merged.GlassdoorReviewCount = r.GlassdoorReviewCount
			}
			if r.LinkedInURL != "" {
				merged.LinkedInURL = r.LinkedInURL
			}
			if r.LinkedInEmployees != "" {
				merged.LinkedInEmployees = r.LinkedInEmployees
			}
			if r.LinkedInFounded != "" {
				merged.LinkedInFounded = r.LinkedInFounded
			}
			if r.BBBURL != "" {
				merged.BBBURL = r.BBBURL
			}
			if r.BBBRating != "" {
				merged.BBBRating = r.BBBRating
			}
		}(fn)
	}
	wg.Wait()
	return merged
}
