package server

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ── DOCX extraction ───────────────────────────────────────────────────────────
// A .docx file is a ZIP archive. The main content lives in word/document.xml.
// We parse it directly using encoding/xml — no external dependency needed.

func extractDocxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("not a valid DOCX (ZIP) file: %w", err)
	}

	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("failed to open word/document.xml: %w", err)
		}
		defer rc.Close()

		raw, err := io.ReadAll(rc)
		if err != nil {
			return "", err
		}
		return parseDocxXML(raw), nil
	}
	return "", fmt.Errorf("word/document.xml not found in DOCX archive")
}

// parseDocxXML walks the XML and collects text from <w:t> elements,
// inserting newlines at paragraph (<w:p>) boundaries.
func parseDocxXML(data []byte) string {
	var sb strings.Builder
	dec := xml.NewDecoder(bytes.NewReader(data))
	inText := false

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "p":
				sb.WriteString("\n")
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

// ── PDF extraction ────────────────────────────────────────────────────────────
// Uses github.com/ledongthuc/pdf which handles compressed streams reliably.
// A cleanup pass merges split characters that result from PDF font encoding.

func extractPDFText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %w", err)
	}

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue // skip pages that fail — partial output is better than none
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		// Fallback: reader-level plain text
		plain, err := r.GetPlainText()
		if err != nil {
			return "", fmt.Errorf("failed to extract PDF text: %w", err)
		}
		text, err := io.ReadAll(plain)
		if err != nil {
			return "", fmt.Errorf("failed to read PDF text: %w", err)
		}
		result = strings.TrimSpace(string(text))
	}
	return cleanupPDFText(result), nil
}

// cleanupPDFText merges fragmented lines that result from PDF character-level
// encoding — e.g. "O\nlexandr" → "Olexandr", "E\nm\nail" → "Email".
func cleanupPDFText(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		// Skip lone separator characters
		if line == "." || line == "-" || line == "·" || line == "•" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			i++
			continue
		}

		// Merge very short line fragments into the next line when the next
		// line continues with a lowercase letter or mid-word character.
		// This handles split like "O" + "lexandr" → "Olexandr"
		for len(line) <= 3 && i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next == "" {
				break
			}
			// Only merge if next line looks like a continuation (starts lowercase
			// or is punctuation that would naturally follow)
			firstRune := rune(next[0])
			if firstRune >= 'a' && firstRune <= 'z' {
				line = line + next
				i++
			} else {
				break
			}
		}

		out = append(out, line)
		i++
	}

	// Collapse runs of blank lines to single blank line
	var final []string
	blankRun := 0
	for _, l := range out {
		if l == "" {
			blankRun++
			if blankRun <= 1 {
				final = append(final, l)
			}
		} else {
			blankRun = 0
			final = append(final, l)
		}
	}
	return strings.TrimSpace(strings.Join(final, "\n"))
}
