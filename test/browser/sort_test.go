//go:build browser

package browser

import (
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// A heading sorts its table: ascending, then descending, then back to the
// list's own order - by what the column holds, read the way the reader writes
// it, so a period sorts by its first day rather than by its characters.
func TestATableSortsByTheHeadingPressed(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	// Three projects whose names, start days and order of creation disagree.
	var made int

	p.awaitJSON(`(async () => {
		for (const [name, startDate] of [['Kiwi', '2026-02-10'], ['apple', '2025-12-01'], ['Banana', '2026-01-15']]) {
			await api('/projects', { method: 'POST', body: JSON.stringify({ name, startDate }) });
		}
		await loadProjects();
		return document.querySelectorAll('#table-projects tbody tr').length;
	})()`, &made)

	p.run("open the projects", p.click(`.tab[data-view="projects"]`),
		chromedp.WaitVisible("#table-projects", chromedp.ByID))

	names := func() string {
		var out string

		p.evalJSON(`JSON.stringify([...document.querySelectorAll('#table-projects tbody tr')]
			.map((row) => cellsOf(row)[0].textContent.trim().split(/\s/)[0]).join(','))`, &out)

		return out
	}

	heading := func(label string) string {
		return `#table-projects thead th.sortable:nth-of-type(` + label + `) .sort-toggle`
	}

	original := names()

	// The name is the second heading after the tick box's column.
	byName := heading("2")

	p.run("sort by name", p.click(byName))
	p.waitEvaluates("ascending by name", `JSON.stringify([...document.querySelectorAll('#table-projects tbody tr')]
		.map((row) => cellsOf(row)[0].textContent.trim().split(/\s/)[0]).join(','))`, `"apple,Banana,Kiwi"`)

	if got := p.attr(`#table-projects thead th.sortable:nth-of-type(2)`, "aria-sort"); got != "ascending" {
		t.Errorf("the name heading says aria-sort=%q while sorted ascending", got)
	}

	if label := p.attr(byName, "aria-label"); label != "Sort by Name" {
		t.Errorf("the name heading's button is called %q, want Sort by Name", label)
	}

	p.run("sort by name the other way", p.click(byName))
	p.waitEvaluates("descending by name", `JSON.stringify([...document.querySelectorAll('#table-projects tbody tr')]
		.map((row) => cellsOf(row)[0].textContent.trim().split(/\s/)[0]).join(','))`, `"Kiwi,Banana,apple"`)

	p.run("back to the list's own order", p.click(byName))

	if got := names(); got != original {
		t.Errorf("the third press left the order %q, want the list's own %q", got, original)
	}

	if got := p.attr(`#table-projects thead th.sortable:nth-of-type(2)`, "aria-sort"); got != "" {
		t.Errorf("unsorted, the name heading still says aria-sort=%q", got)
	}

	// The period sorts by its first day: as text, 12/01/2025 would follow 02/10/2026.
	p.run("sort by period", p.click(heading("3")))
	p.waitEvaluates("ascending by start day", `JSON.stringify([...document.querySelectorAll('#table-projects tbody tr')]
		.map((row) => cellsOf(row)[0].textContent.trim().split(/\s/)[0]).join(','))`, `"apple,Banana,Kiwi"`)

	// Figures sort as figures, signed and in the reader's own conventions, and a
	// dash comes last whichever way.
	var figures struct {
		English string `json:"english"`
	}

	p.evalJSON(`(() => {
		const texts = ['+0.25 h', '-4.00 h', '–', '10.00 h', '0.00 h'];
		const { keyOf, compare } = sortKeys(texts);
		const sorted = texts.map((text) => ({ text, key: keyOf(text) }))
			.sort((a, b) => (a.key === null) - (b.key === null) || (a.key === null ? 0 : compare(a.key, b.key)))
			.map((entry) => entry.text);
		return JSON.stringify({ english: sorted.join(' | ') });
	})()`, &figures)

	if figures.English != "-4.00 h | 0.00 h | +0.25 h | 10.00 h | –" {
		t.Errorf("signed hours sorted as %q", figures.English)
	}

	p.chooseLanguage("de")

	var german string

	p.evalJSON(`(() => {
		const texts = ['1.234,50 €', '999,00 €', '12,00 €'];
		const { keyOf, compare } = sortKeys(texts);
		return JSON.stringify(texts.map((text) => ({ text, key: keyOf(text) }))
			.sort((a, b) => compare(a.key, b.key)).map((entry) => entry.text).join(' | '));
	})()`, &german)

	if german != "12,00 € | 999,00 € | 1.234,50 €" {
		t.Errorf("German amounts sorted as %q", german)
	}
}

// The entry list sorts on the server, so "oldest first" is the oldest of every
// page rather than of the page on screen.
func TestTheEntryListSortsByDayAcrossPages(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	seedEntries(t, p, 26)
	showTimeView(p)

	const (
		nav   = `#entries-card .pager`
		byDay = `#table-timesheets thead th[data-sort="date"] .sort-toggle`
		first = `#table-timesheets tbody tr:first-child`
	)

	p.waitShown(nav)

	// Seeded from 1 January onwards, the oldest is "seeded 0" - on the second
	// page, newest first.
	if got := p.text(first); strings.Contains(got, "seeded 0") {
		t.Fatalf("newest first, the first row already reads %q", got)
	}

	p.run("sort by day", p.click(byDay))
	p.waitForTextWithin(first, "seeded 0", waitPatience)

	if got := p.attr(`#table-timesheets thead th[data-sort="date"]`, "aria-sort"); got != "ascending" {
		t.Errorf("the day heading says aria-sort=%q while sorted ascending", got)
	}

	p.run("the other way", p.click(byDay))
	p.waitForTextWithin(first, "seeded 25", waitPatience)

	// The project cannot be sorted by on the server, so it offers no button.
	if p.count(`#table-timesheets thead th:nth-of-type(3) .sort-toggle`) != 0 {
		t.Error("the project heading offers to sort a list the server cannot sort by project")
	}
}
