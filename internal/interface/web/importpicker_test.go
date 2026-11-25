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

	// Each is the button that opens a file input out of sight, naming it in
	// data-file; what it accepts is the input's.
	html := asset(t, "/")

	opener := regexp.MustCompile(`<button\b([^>]*\bdata-file="([^"]+)"[^>]*)>([^<]+)</button>`)
	for _, m := range opener.FindAllStringSubmatch(html, -1) {
		key := regexp.MustCompile(`\bdata-i18n="([^"]+)"`).FindStringSubmatch(m[1])
		input := regexp.MustCompile(`<input[^>]*\bid="` + regexp.QuoteMeta(m[2]) + `"[^>]*>`).FindString(html)
		accept := regexp.MustCompile(`\baccept="([^"]*)"`).FindStringSubmatch(input)

		if key == nil || accept == nil {
			t.Errorf("the button opening #%s names no words or its input accepts nothing named", m[2])

			continue
		}

		pickers = append(pickers, picker{"index.html", key[1], m[3], accept[1]})
	}

	// The tables' own cards build theirs in the script, from one file input.
	script := asset(t, "/app.js")

	accepts := regexp.MustCompile(`const file = el\('input', \{[^}]*accept: '([^']*)'`).FindStringSubmatch(script)
	built := regexp.MustCompile(`const picker = el\('button', \{[^}]*'data-i18n': '([^']+)', text: '([^']+)'`).
		FindStringSubmatch(script)

	if accepts != nil && built != nil {
		pickers = append(pickers, picker{"app.js", built[1], built[2], accepts[1]})
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
