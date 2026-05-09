package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func resetMXLookupHook() func() {
	prev := mxLookupTestHook
	return func() { mxLookupTestHook = prev }
}

func TestExtractDomain_ValidEmail(t *testing.T) {
	got := extractDomain("  Hello@Example.COM ")
	if got != "example.com" {
		t.Errorf("got %q want example.com", got)
	}
}

func TestExtractDomain_NoDotReturnsEmpty(t *testing.T) {
	if extractDomain("a@nodot") != "" {
		t.Fail()
	}
}

func TestExtractDomain_NoAtSignReturnsEmpty(t *testing.T) {
	if extractDomain("nosign") != "" {
		t.Fail()
	}
}

func TestExtractDomain_EmptyStringReturnsEmpty(t *testing.T) {
	if extractDomain("") != "" {
		t.Fail()
	}
}

func TestExtractDomain_NormalizesLowercase(t *testing.T) {
	if extractDomain("U@SUb.EXAMPLE.Co.Uk") != "sub.example.co.uk" {
		t.Errorf("got %q", extractDomain("U@SUb.EXAMPLE.Co.Uk"))
	}
}

func TestNslookupMX_ParsesLinuxFormat(t *testing.T) {
	sample := `
Server:		127.0.0.53
example.com	mail exchanger = 50 alt1.mx.example.net.
example.com	mail exchanger = 10 primary.mx.example.COM.
`
	got := parseNslookupMXOutput(sample)
	if len(got) != 2 {
		t.Fatalf("got %#v len=%d want 2 hosts", got, len(got))
	}
	if got[0] != "alt1.mx.example.net" || got[1] != "primary.mx.example.COM" {
		t.Errorf("got %#v want sorted MX hosts", got)
	}
}

func TestNslookupMX_ParsesWindowsFormat(t *testing.T) {
	sample := `example.com	MX preference = 10, mail exchanger = winmx01.example.net.
something else
example.com	MX preference = 20, mail exchanger = winmx02.example.net.
`
	got := parseNslookupMXOutput(sample)
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
	if got[0] != "winmx01.example.net" || got[1] != "winmx02.example.net" {
		t.Errorf("got %#v", got)
	}
}

func TestNslookupMX_ParsesMultipleRecords(t *testing.T) {
	text := `line1
MX preference = 5, mail exchanger = mx1.zone.
mail exchanger = 10 mx2.zone.
mail exchanger = 20 mx3.zone.tail.
`
	got := parseNslookupMXOutput(text)
	if len(got) != 3 {
		t.Fatalf("got %#v len %d", got, len(got))
	}
}

func TestNslookupMX_TrailingDotStripped(t *testing.T) {
	text := `mail exchanger = 1 host.with.dot.example.`
	got := parseNslookupMXOutput(text)
	if len(got) != 1 || got[0] != "host.with.dot.example" {
		t.Fatalf("got %#v", got)
	}
}

func TestNslookupMX_EmptyOutputReturnsEmpty(t *testing.T) {
	if len(parseNslookupMXOutput("")) != 0 {
		t.Fail()
	}
}

func TestNslookupMX_InvalidDomainReturnsEmpty(t *testing.T) {
	txt := "** server can't find nonexistent.invalid: NXDOMAIN\n"
	got := parseNslookupMXOutput(txt)
	if len(got) != 0 {
		t.Errorf("want empty, got %#v", got)
	}
}

func TestValidateEmailDomain_InvalidEmailReturnsError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	r := validateEmailDomain(db, "not-an-email")
	if r.Error != "invalid email" || r.Valid || r.Domain != "" {
		t.Fatalf("want invalid email result, got %+v", r)
	}
}

func TestValidateEmailDomain_CacheHit(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	d := resetMXLookupHook()
	defer d()
	mxLookupTestHook = func(context.Context, string) []string {
		t.Fatal("live lookup must not run on cache hit")
		return nil
	}

	if err := upsertMXCache(db, "cached.example.net", true, []string{"mx.cached.example.net"}); err != nil {
		t.Fatal(err)
	}
	r := validateEmailDomain(db, "x@Cached.Example.NET")
	if !r.Cached || !r.Valid || !r.HasMX || r.Domain != "cached.example.net" {
		t.Fatalf("%+v", r)
	}
	if len(r.MXRecords) != 1 || r.MXRecords[0] != "mx.cached.example.net" {
		t.Fatalf("%+v", r)
	}
}

func TestValidateEmailDomain_LiveLookup(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	d := resetMXLookupHook()
	defer d()
	mxLookupTestHook = func(_ context.Context, dom string) []string {
		if dom != "dyn.example.test" {
			t.Fatalf("domain %q", dom)
		}
		return []string{"live.mx.test"}
	}
	r := validateEmailDomain(db, "u@Dyn.Example.Test")
	if r.Cached {
		t.Error("expected live path")
	}
	if !r.Valid || !r.HasMX || r.Domain != "dyn.example.test" {
		t.Fatalf("%+v", r)
	}
	if len(r.MXRecords) != 1 || r.MXRecords[0] != "live.mx.test" {
		t.Fatalf("%+v", r)
	}
}

func TestValidateEmailDomain_ResultHasRequiredFields(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	d := resetMXLookupHook()
	defer d()
	mxLookupTestHook = func(context.Context, string) []string { return []string{"a.mx"} }

	r := validateEmailDomain(db, "who@ReqFields.example")
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	json.Unmarshal(b, &raw)
	for _, k := range []string{"email", "domain", "valid", "has_mx", "mx_records", "cached"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("missing key %q in %s", k, string(b))
		}
	}
}

func TestHandleMXValidateDomain_EmptyEmailReturns422(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	form := strings.NewReader(url.Values{}.Encode())
	req := httptest.NewRequest(http.MethodPost, "/api/email/validate-domain", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleEmailValidateDomain(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d", w.Code)
	}
}

func TestHandleMXValidateDomain_ValidResponse(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	d := resetMXLookupHook()
	defer d()
	mxLookupTestHook = func(context.Context, string) []string { return []string{"mx.ok"} }

	form := strings.NewReader(url.Values{"email": {"User@Handled.Test"}}.Encode())
	req := httptest.NewRequest(http.MethodPost, "/api/email/validate-domain", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleEmailValidateDomain(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var mr MXResult
	if err := json.NewDecoder(w.Body).Decode(&mr); err != nil {
		t.Fatal(err)
	}
	if mr.Email != "User@Handled.Test" || mr.Domain != "handled.test" || !mr.Valid || !mr.HasMX {
		t.Fatalf("%+v", mr)
	}
}

func TestHandleMXCache_EmptyReturnsEmptyObject(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/email/mx-cache", nil)
	w := httptest.NewRecorder()
	handleEmailMXCache(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	body := w.Body.String()
	if strings.TrimSpace(body) != "{}" {
		t.Errorf("want {}, got %q", body)
	}
}

func TestHandleMXCache_ReturnsInsertedEntry(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	if err := upsertMXCache(db, "shown.example", true, []string{"a.mx", "b.mx"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/email/mx-cache", nil)
	w := httptest.NewRecorder()
	handleEmailMXCache(w, req)

	var m map[string]map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	row, ok := m["shown.example"]
	if !ok {
		t.Fatalf("missing key, got %v", m)
	}
	if row["has_mx"] != true {
		t.Fatal(row)
	}
	recs, _ := json.Marshal(row["mx_records"])
	if string(recs) != `["a.mx","b.mx"]` {
		t.Fatalf("unexpected mx_records: %v", row["mx_records"])
	}
}
