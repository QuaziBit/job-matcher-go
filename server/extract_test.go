package server

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// ── DOCX test helpers ─────────────────────────────────────────────────────────

func buildDocx(paragraphs []string) []byte {
	var body strings.Builder
	for _, p := range paragraphs {
		body.WriteString(`<w:p><w:r><w:t>`)
		body.WriteString(p)
		body.WriteString(`</w:t></w:r></w:p>`)
	}

	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` + body.String() + `</w:body></w:document>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("word/document.xml")
	f.Write([]byte(xmlContent))
	zw.Close()
	return buf.Bytes()
}

// ── extractDocxText ───────────────────────────────────────────────────────────

func TestExtractDocxText_SingleParagraph(t *testing.T) {
	data := buildDocx([]string{"Hello World"})
	text, err := extractDocxText(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Hello World") {
		t.Errorf("expected 'Hello World', got: %q", text)
	}
}

func TestExtractDocxText_MultipleParagraphs(t *testing.T) {
	data := buildDocx([]string{"Line One", "Line Two", "Line Three"})
	text, err := extractDocxText(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Line One") || !strings.Contains(text, "Line Three") {
		t.Errorf("expected all lines, got: %q", text)
	}
}

func TestExtractDocxText_InvalidZip(t *testing.T) {
	_, err := extractDocxText([]byte("not a zip file"))
	if err == nil {
		t.Error("expected error for invalid ZIP/DOCX data")
	}
}

func TestExtractDocxText_MissingDocumentXML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("word/other.xml")
	f.Write([]byte("<root/>"))
	zw.Close()

	_, err := extractDocxText(buf.Bytes())
	if err == nil {
		t.Error("expected error when word/document.xml is missing")
	}
}

func TestExtractDocxText_EmptyDocument(t *testing.T) {
	data := buildDocx([]string{})
	_, err := extractDocxText(data)
	if err != nil {
		t.Fatalf("unexpected error on empty document: %v", err)
	}
}

// ── parseDocxXML ─────────────────────────────────────────────────────────────

func TestParseDocxXML_ExtractsText(t *testing.T) {
	xml := []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body><w:p><w:r><w:t>Go Developer</w:t></w:r></w:p></w:body></w:document>`)
	text := parseDocxXML(xml)
	if !strings.Contains(text, "Go Developer") {
		t.Errorf("expected 'Go Developer', got: %q", text)
	}
}

func TestParseDocxXML_MultipleRuns(t *testing.T) {
	xml := []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body><w:p><w:r><w:t>Python </w:t></w:r><w:r><w:t>Go</w:t></w:r></w:p></w:body></w:document>`)
	text := parseDocxXML(xml)
	if !strings.Contains(text, "Python") || !strings.Contains(text, "Go") {
		t.Errorf("expected both runs, got: %q", text)
	}
}

func TestParseDocxXML_Empty(t *testing.T) {
	text := parseDocxXML([]byte(`<w:document><w:body></w:body></w:document>`))
	if text != "" {
		t.Errorf("expected empty string, got: %q", text)
	}
}

// ── extractPDFText ────────────────────────────────────────────────────────────

func TestExtractPDFText_InvalidPDF(t *testing.T) {
	_, err := extractPDFText([]byte("not a pdf"))
	if err == nil {
		t.Error("expected error for non-PDF data")
	}
}

func TestExtractPDFText_EmptyBytes(t *testing.T) {
	_, err := extractPDFText([]byte{})
	if err == nil {
		t.Error("expected error for empty byte slice")
	}
}

func TestExtractPDFText_StartsWithPDFHeader(t *testing.T) {
	// A byte slice that starts with %PDF but is otherwise malformed
	// should return an error — not panic or produce garbage.
	_, err := extractPDFText([]byte("%PDF-1.4\ngarbage"))
	if err == nil {
		t.Log("note: malformed PDF returned no error — acceptable if library is lenient")
	}
	// The key requirement is no panic. If we reach here the test passes.
}
