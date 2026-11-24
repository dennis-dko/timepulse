package web_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// fixedLength names every table that does not page, with the reason its length
// cannot grow. A table filled through fillTable pages unless it says
// data-pager="off", so a list nobody expected to grow cannot become an endless
// card - and a table that says it should have a reason a reader can check.
var fixedLength = map[string]string{
	"table-year": "a row for each of the twelve months",
}

// Every table that is filled from data sits in the box it scrolls in.
//
// A long table scrolls inside that box rather than lengthening the page, and the
// selection bar and the search are put above it. The backup table stood in its
// card without one, so the bar meant to stand above the box stood above the
// whole card, outside it.
func TestEveryTableSitsInTheBoxItScrollsIn(t *testing.T) {
	markup := withoutComments(readAsset(t, "index.html"))

	tables := tableOpening.FindAllStringSubmatchIndex(markup, -1)
	if len(tables) < 10 {
		t.Fatalf("found %d tables with an id; the pattern no longer matches the markup", len(tables))
	}

	for _, at := range tables {
		id := markup[at[2]:at[3]]
		before := strings.TrimSpace(markup[:at[0]])

		opening := strings.LastIndex(before, "<")
		if opening < 0 || !tableWrap.MatchString(before[opening:]) {
			t.Errorf("%s is not the first thing inside a div.table-wrap", id)
		}
	}
}

// A table that does not page says why, and every reason given is for a table
// that still says so.
func TestATableThatDoesNotPageSaysWhy(t *testing.T) {
	markup := withoutComments(readAsset(t, "index.html"))

	var off []string

	for _, tag := range tableTag.FindAllString(markup, -1) {
		if !strings.Contains(tag, `data-pager="off"`) {
			continue
		}

		id := elementID.FindStringSubmatch(tag)
		if id == nil {
			t.Errorf("a table without an id says it does not page: %s", tag)

			continue
		}

		off = append(off, id[1])

		if _, named := fixedLength[id[1]]; !named {
			t.Errorf(`%s says data-pager="off" without a reason in fixedLength`, id[1])
		}
	}

	for id := range fixedLength {
		if !slices.Contains(off, id) {
			t.Errorf(`fixedLength gives a reason for %s, which no longer says data-pager="off"`, id)
		}
	}
}

// A filter is offered only where it works: on a table that pages in the
// browser. A server-paged table is searched by its own form, and one that does
// not page shows no filter at all, so a data-filter on either would promise a
// control that never appears.
func TestAFilterIsOnlyNamedWhereTheTablePagesInTheBrowser(t *testing.T) {
	markup := withoutComments(readAsset(t, "index.html"))

	for _, block := range tableBlock.FindAllString(markup, -1) {
		if !strings.Contains(block, "data-filter") {
			continue
		}

		opening := tableTag.FindString(block)
		if strings.Contains(opening, `data-pager="off"`) || strings.Contains(opening, `data-pager="server"`) {
			t.Errorf("a heading names a filter in a table that does not page in the browser: %s", opening)
		}

		if strings.Count(block, "data-filter") > 1 {
			t.Errorf("more than one heading names a filter in %s; a table offers one", opening)
		}
	}
}

func withoutComments(markup string) string {
	return markupComment.ReplaceAllString(markup, "")
}

var (
	tableOpening = regexp.MustCompile(`<table\s[^>]*\bid="([^"]+)"[^>]*>`)
	tableTag     = regexp.MustCompile(`<table(?:\s[^>]*)?>`)
	tableBlock   = regexp.MustCompile(`(?s)<table(?:\s[^>]*)?>.*?</table>`)
	tableWrap    = regexp.MustCompile(`^<div\s[^>]*\bclass="(?:[^"]*\s)?table-wrap(?:\s[^"]*)?"[^>]*>$`)
)

// A table paged on the server has a loader its page buttons call. Without one,
// a press on "next" changes the page number the table keeps and nothing else.
func TestEveryServerPagedTableHasItsLoader(t *testing.T) {
	markup := withoutComments(readAsset(t, "index.html"))
	script := readAsset(t, "app.js")

	var paged int

	for _, tag := range tableTag.FindAllString(markup, -1) {
		if !strings.Contains(tag, `data-pager="server"`) {
			continue
		}

		paged++

		id := elementID.FindStringSubmatch(tag)
		if id == nil {
			t.Errorf("a table without an id is paged on the server: %s", tag)

			continue
		}

		if !strings.Contains(script, "SERVER_PAGE_LOADERS['"+id[1]+"'] = ") {
			t.Errorf(`%s says data-pager="server" and app.js registers no loader for it in SERVER_PAGE_LOADERS`, id[1])
		}
	}

	if paged == 0 {
		t.Fatal(`no table says data-pager="server", so this test is no longer reading the markup`)
	}
}

// A heading names the server's word for its column only in a table the server
// pages. Everywhere else the browser sorts by what a column holds, and a word
// for a server that is never asked would read as a promise that it is.
func TestASortWordIsOnlyNamedWhereTheServerSorts(t *testing.T) {
	markup := withoutComments(readAsset(t, "index.html"))

	var named int

	for _, block := range tableBlock.FindAllString(markup, -1) {
		if !strings.Contains(block, "data-sort=") {
			continue
		}

		named++

		if opening := tableTag.FindString(block); !strings.Contains(opening, `data-pager="server"`) {
			t.Errorf("a heading names a sort word in a table the server does not page: %s", opening)
		}
	}

	if named == 0 {
		t.Fatal("no heading names a sort word, so this test is no longer reading the markup")
	}
}
