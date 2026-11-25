package web_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dennis-dko/timepulse/internal/support/i18n"
)

// A file picker that feeds an import says it imports.
//
// Each sits beside "Export as .xlsx" and said "Choose a file…", so the pair read
// as one way out and an unnamed button, and what pressing it would lead to was
// left for the file dialog to suggest. The logo's picker is an upload rather than
// an import and keeps its own words; it accepts images, which is how it is told
// apart here.
func TestEveryImportPickerSaysItImports(t *testing.T) {
	type picker struct{ where, key, english, accept string }

	var pickers []picker

	markup := regexp.MustCompile(`(?s)<label class="file-pick"[^>]*>\s*<span data-i18n="([^"]+)">([^<]+)</span>\s*<input type="file"[^>]*accept="([^"]*)"`)
	for _, m := range markup.FindAllStringSubmatch(asset(t, "/"), -1) {
		pickers = append(pickers, picker{"index.html", m[1], m[2], m[3]})
	}

	// The tables' own cards build theirs in the script, from one file input.
	script := asset(t, "/app.js")

	built := regexp.MustCompile(`(?s)const file = el\('input', \{\s*type: 'file',\s*accept: '([^']*)',.*?el\('label', \{ class: 'file-pick'[^}]*\},\s*el\('span', \{ 'data-i18n': '([^']+)', text: '([^']+)' \}\), file\)`)
	for _, m := range built.FindAllStringSubmatch(script, -1) {
		pickers = append(pickers, picker{"app.js", m[2], m[3], m[1]})
	}

	imports := 0

	for _, p := range pickers {
		if !strings.Contains(p.accept, ".xlsx") {
			continue
		}

		imports++

		if !strings.HasPrefix(p.english, "Import") {
			t.Errorf("a picker in %s that feeds an import says %q", p.where, p.english)
		}

		if german := i18n.Dictionary("de")[p.key]; !strings.Contains(german, "importieren") {
			t.Errorf("a picker in %s that feeds an import says %q in German", p.where, german)
		}
	}

	// The time entries' card and the one the three tables share: fewer found is
	// the pattern above no longer matching, not a picker gone.
	if imports < 2 {
		t.Fatalf("found %d import pickers, want the time entries' and the tables' at least", imports)
	}
}
