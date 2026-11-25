package web_test

import (
	"regexp"
	"strings"
	"testing"
)

// voidElements are the tags that never close, which a walk keeping track of what
// is open must not wait for.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true,
	"track": true, "wbr": true,
}

var (
	cardClass        = regexp.MustCompile(`\bclass="(?:[^"]*\s)?card(?:\s[^"]*)?"`)
	fileActionsClass = regexp.MustCompile(`\bclass="file-actions"`)
	fileControl      = regexp.MustCompile(
		`data-i18n="(?:action\.exportPdf|wb\.export|wb\.importFrom)"|type="file"[^>]*accept="[^"]*\.xlsx`)
)

// TestEveryExportAndImportStandsInItsCardsLastRow holds every export and every
// import to one place on its card: the row at its foot, div.file-actions, with
// nothing after it.
//
// They stood wherever the card that grew them had room - eight pairs in a row
// under their table, the working time's beside its heading, the spreadsheet's
// under its description - so whoever had found one had learnt nothing about
// where the next one was.
func TestEveryExportAndImportStandsInItsCardsLastRow(t *testing.T) {
	markup := withoutComments(asset(t, "/"))

	type open struct {
		name    string
		card    int // which card this is, counted from one; zero for anything else
		actions bool
	}

	var (
		stack       []open
		cards       int
		controls    int
		rowsPerCard = map[int]int{}
		// Set when a file-actions row has closed: the very next tag has to close
		// its card.
		mustCloseCard bool
	)

	cardOf := func() int {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].card > 0 {
				return i
			}
		}

		return -1
	}

	for _, tag := range markupTag.FindAllStringSubmatch(markup, -1) {
		closing, name, attrs := tag[1] == "/", strings.ToLower(tag[2]), tag[3]

		if mustCloseCard {
			mustCloseCard = false

			if !closing || len(stack) == 0 || stack[len(stack)-1].card == 0 {
				t.Errorf("a card goes on after its file-actions row with <%s%s %s>: the row "+
					"is the card's last, so its exports and imports are found at its foot "+
					"whatever else it holds", tag[1], name, summarise(strings.TrimSpace(attrs)))
			}
		}

		if closing {
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].name == name {
					mustCloseCard = stack[i].actions
					stack = stack[:i]

					break
				}
			}

			continue
		}

		if fileControl.MatchString(attrs) {
			controls++

			inRow := false

			for _, element := range stack {
				inRow = inRow || element.actions
			}

			if !inRow {
				t.Errorf("<%s %s> exports or imports outside a div.file-actions: on every card "+
					"those stand together in its last row", name, summarise(strings.TrimSpace(attrs)))
			}
		}

		if voidElements[name] || strings.HasSuffix(attrs, "/") {
			continue
		}

		element := open{name: name, actions: fileActionsClass.MatchString(attrs)}

		if cardClass.MatchString(attrs) {
			cards++
			element.card = cards
		}

		if element.actions {
			switch card := cardOf(); {
			case card < 0:
				t.Error("a div.file-actions stands outside any card")
			case card != len(stack)-1:
				t.Errorf("a div.file-actions stands inside a <%s> rather than directly in its card",
					stack[len(stack)-1].name)
			default:
				rowsPerCard[stack[card].card]++
			}
		}

		stack = append(stack, element)
	}

	for _, rows := range rowsPerCard {
		if rows > 1 {
			t.Errorf("a card has %d file-actions rows; its exports and imports belong in one", rows)
		}
	}

	// Ten cards with a PDF and a workbook each, and the time entries' import: fewer
	// is the walk no longer finding them, not a card gone.
	if controls < 22 {
		t.Fatalf("found %d exports and imports in the markup, want at least 22", controls)
	}

	// The tables' own cards are built rather than written, and build the same row
	// last.
	built := functionSource(t, asset(t, "/app.js"), "buildSheetCard")

	foot := regexp.MustCompile(`(?s)return el\('div', \{ class: 'card'[^;]*,\s*el\('div', \{ class: 'file-actions' \},\s*([^;()]*)\)\);`).
		FindStringSubmatch(built)
	if foot == nil {
		t.Fatal("buildSheetCard does not end its card with a div.file-actions row")
	}

	for _, part := range []string{"pdfButton", "exportButton", "picker"} {
		if !regexp.MustCompile(`\b` + part + `\b`).MatchString(foot[1]) {
			t.Errorf("the row at the foot of a table's card holds %q and not %s", foot[1], part)
		}
	}
}
