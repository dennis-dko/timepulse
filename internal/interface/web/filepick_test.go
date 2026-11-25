package web_test

import (
	"regexp"
	"strings"
	"testing"
)

// TestEveryFileChooserSpeaksTheApplicationsLanguage holds every file chooser to a
// button of the application's own, with the browser's control out of sight.
//
// The browser draws its own file control - a button and a line saying no file has
// been chosen - in its own UI language, whatever the page is written in: measured
// on the settings screen, a German page showed "Choose File" and "No file chosen"
// beside the logo, on a browser started in English. It is the same reason the
// date fields write their own box. So the input carries class file-input, which
// keeps it out of sight, and a button.secondary naming it in data-file opens it,
// with words of its own. A label dressed as a button did that job before, and was
// drawn as a box among pills: the import looked like nothing else on its row.
func TestEveryFileChooserSpeaksTheApplicationsLanguage(t *testing.T) {
	markup := withoutComments(asset(t, "/"))

	inputs := regexp.MustCompile(`<input[^>]*type="file"[^>]*>`).FindAllString(markup, -1)
	if len(inputs) == 0 {
		t.Fatal("no file inputs found in the markup, so this guard is checking nothing")
	}

	for _, input := range inputs {
		id := regexp.MustCompile(`\bid="([^"]+)"`).FindStringSubmatch(input)
		if id == nil {
			t.Errorf("%s has no id for the button that opens it to name", input)

			continue
		}

		if !strings.Contains(input, `class="file-input"`) {
			t.Errorf("%s is drawn by the browser, in the browser's language rather than the "+
				"reader's: give it class file-input and a button that opens it", input)
		}

		opener := regexp.MustCompile(`<button\b[^>]*\bdata-file="` + regexp.QuoteMeta(id[1]) + `"[^>]*>`).
			FindString(markup)

		switch {
		case opener == "":
			t.Errorf("nothing opens #%s: a button.secondary naming it in data-file is what "+
				"the reader sees and presses", id[1])
		case !strings.Contains(opener, `class="secondary"`) || !strings.Contains(opener, `data-i18n="`):
			t.Errorf("the button opening #%s is not a button.secondary with words of its own: %s",
				id[1], opener)
		}
	}

	// The tables' cards build theirs in the script, and open it the same way.
	built := functionSource(t, asset(t, "/app.js"), "buildSheetCard")

	if !regexp.MustCompile(`const file = el\('input', \{[^}]*class: 'file-input'`).MatchString(built) {
		t.Error("the file input a table's card builds is not kept out of sight with class file-input")
	}

	if !regexp.MustCompile(`const picker = el\('button', \{[^}]*class: 'secondary'[^}]*onclick: \(\) => file\.click\(\)`).
		MatchString(built) {
		t.Error("a table's card does not open its file input from a button.secondary of its own")
	}
}
