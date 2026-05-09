package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// MXResult aggregates email domain / MX lookup outcome for API responses.
type MXResult struct {
	Email     string   `json:"email"`
	Domain    string   `json:"domain"`
	Valid     bool     `json:"valid"`
	HasMX     bool     `json:"has_mx"`
	MXRecords []string `json:"mx_records"`
	Cached    bool     `json:"cached"`
	Error     string   `json:"error,omitempty"`
}

var (
	winMXRegex   = regexp.MustCompile(`(?i)MX\s+preference\s*=\s*\d+\s*,\s*mail\s+exchanger\s*=\s*(\S+)`)
	linuxMXRegex = regexp.MustCompile(`(?i)mail\s+exchanger\s*=\s*\d+\s+(\S+)`)
)

// mxLookupTestHook, when non-nil, replaces live DNS lookups (tests).
var mxLookupTestHook func(context.Context, string) []string

func extractDomain(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || !strings.Contains(email, "@") {
		return ""
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	dom := strings.TrimSpace(parts[1])
	if dom == "" || !strings.Contains(dom, ".") {
		return ""
	}
	return dom
}

func stripMxHostTrailingDot(host string) string {
	return strings.TrimSuffix(strings.TrimSpace(host), ".")
}

// parseNslookupMXOutput parses nslookup stdout/err for MX hostnames.
func parseNslookupMXOutput(text string) []string {
	seen := map[string]struct{}{}
	add := func(h string) {
		h = stripMxHostTrailingDot(h)
		if h != "" {
			seen[h] = struct{}{}
		}
	}
	for _, m := range winMXRegex.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}
	for _, m := range linuxMXRegex.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}
	list := make([]string, 0, len(seen))
	for h := range seen {
		list = append(list, h)
	}
	sort.Strings(list)
	return list
}

func nslookupMX(domain string, timeout int) []string {
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nslookup", "-type=MX", domain)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if showMoreLogs() {
		log.Printf("→ nslookup MX domain=%q err=%v\n%s", domain, err, text)
	}
	if err != nil {
		return nil
	}
	return parseNslookupMXOutput(text)
}

func lookupMX(ctx context.Context, domain string) []string {
	if mxLookupTestHook != nil {
		return mxLookupTestHook(ctx, domain)
	}
	timeout := 30
	if dl, ok := ctx.Deadline(); ok {
		if d := time.Until(dl); d > 0 {
			sec := int(d / time.Second)
			if sec < 1 {
				sec = 1
			}
			timeout = sec
		}
	}
	return nslookupMX(domain, timeout)
}

func getCachedMX(db *sql.DB, domain string) (*MXResult, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil, nil
	}
	var hasMX int
	var rawJSON string
	err := db.QueryRow(`
		SELECT has_mx, COALESCE(mx_records,'')
		FROM domain_mx_cache
		WHERE domain = ?
		  AND datetime(checked_at) > datetime('now', '-24 hours')`, domain).
		Scan(&hasMX, &rawJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []string
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON != "" {
		_ = json.Unmarshal([]byte(rawJSON), &recs)
	}
	if recs == nil {
		recs = []string{}
	}
	return &MXResult{
		Domain:    domain,
		Valid:     true,
		HasMX:     hasMX != 0,
		MXRecords: recs,
		Cached:    true,
	}, nil
}

func upsertMXCache(dbConn *sql.DB, domain string, hasMX bool, records []string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return fmt.Errorf("empty domain")
	}
	if records == nil {
		records = []string{}
	}
	jsonBytes, err := json.Marshal(records)
	if err != nil {
		return err
	}
	h := 0
	if hasMX {
		h = 1
	}
	_, err = dbConn.Exec(`
		INSERT INTO domain_mx_cache (domain, has_mx, mx_records, checked_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(domain) DO UPDATE SET
			has_mx = excluded.has_mx,
			mx_records = excluded.mx_records,
			checked_at = CURRENT_TIMESTAMP`,
		domain, h, string(jsonBytes))
	return err
}

func validateEmailDomain(dbConn *sql.DB, email string) MXResult {
	dom := extractDomain(email)
	if dom == "" {
		return MXResult{
			Email: email,
			Valid: false,
			Error: "invalid email",
		}
	}
	cached, err := getCachedMX(dbConn, dom)
	if err != nil {
		log.Printf("✗ getCachedMX(%q): %v", dom, err)
	}
	if cached != nil {
		cached.Email = email
		return *cached
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	recs := lookupMX(ctx, dom)
	has := len(recs) > 0
	if err := upsertMXCache(dbConn, dom, has, recs); err != nil {
		log.Printf("✗ upsertMXCache(%q): %v", dom, err)
	}
	return MXResult{
		Email:     email,
		Domain:    dom,
		Valid:     true,
		HasMX:     has,
		MXRecords: recs,
		Cached:    false,
	}
}
