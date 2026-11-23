//go:build browser

package browser

import (
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// A long table shows a page at a time, and searches, filters and changes its
// page size; a short one is shown whole.
//
// Only a browser can check this: the pages are cut from the rows the loader
// handed over, at render time, and every question below - how many rows are on
// screen, what the pager says, which rows "select all" reaches, what a document
// reads off the table - is a fact about running JavaScript.
//
// The projects table is the one used because an ordinary account can fill it
// without anything else existing, and it has a column to filter by.
func TestALongTablePagesSearchesAndFilters(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	const (
		card  = `.card:has(#table-projects)`
		rows  = `#table-projects tbody tr`
		tools = card + ` .list-tools`
		nav   = card + ` .pager`
	)

	p.run("open the projects", p.click(`.tab[data-view="projects"]`),
		chromedp.WaitVisible("#table-projects", chromedp.ByID))

	// Nothing to page yet, so nothing offers to.
	if p.visible(tools) || p.visible(nav) {
		t.Fatal("an empty projects table shows a search or a pager")
	}

	// Thirty: twenty running and ten finished, numbered so the order is known.
	var made int

	p.awaitJSON(`(async () => {
		for (let i = 1; i <= 30; i++) {
			const name = 'Project ' + String(i).padStart(2, '0');
			await api('/projects', { method: 'POST', body: JSON.stringify({
				name, startDate: '2026-01-01', status: i > 20 ? 'completed' : 'active',
			}) });
		}
		await loadProjects();
		return document.querySelectorAll('#table-projects tbody tr').length;
	})()`, &made)

	p.waitShown(nav)

	if !p.visible(tools) {
		t.Fatal("thirty projects and no search above the table")
	}

	// A page of the default size, and a pager that says which.
	if got := p.count(rows); got != 25 {
		t.Errorf("the first page holds %d rows, want the default 25", got)
	}

	if got := p.text(nav + ` .pager-range`); got != "1–25 of 30" {
		t.Errorf("the pager says %q, want 1–25 of 30", got)
	}

	if got := p.text(nav + ` .pager-page`); got != "Page 1 of 2" {
		t.Errorf("the pager says %q, want Page 1 of 2", got)
	}

	if !p.disabled(nav+` [data-page="previous"]`) || p.disabled(nav+` [data-page="next"]`) {
		t.Error("on the first page, previous should be off and next on")
	}

	// A long table scrolls in its box rather than lengthening the page.
	var bounded bool

	p.evalJSON(`JSON.stringify(document.querySelector('#table-projects').parentElement.matches('.table-wrap.paged')
		&& getComputedStyle(document.querySelector('#table-projects').parentElement).maxHeight !== 'none')`, &bounded)

	if !bounded {
		t.Error("the long table's box has no height bound, so the table lengthens the page instead of scrolling")
	}

	// The next page holds the rest.
	p.run("go to the next page", p.click(nav+` [data-page="next"]`))
	p.waitForText(nav+` .pager-range`, "26–30 of 30")

	if got := p.count(rows); got != 5 {
		t.Errorf("the second page holds %d rows, want the remaining 5", got)
	}

	// Ten a page, from the first page again.
	p.run("show ten a page", p.chooseOption(nav+` select.pager-size`, "10"))
	p.waitForText(nav+` .pager-page`, "Page 1 of 3")

	if got := p.count(rows); got != 10 {
		t.Errorf("with ten a page the table shows %d rows", got)
	}

	// Select all reaches the page on screen, not the twenty rows on the others.
	p.run("select all", p.click(`#table-projects thead input.pick-all`),
		chromedp.WaitVisible(card+` .bulk-bar.shown`, chromedp.ByQuery))

	if count := p.text(card + ` .bulk-bar.shown .bulk-count`); !strings.Contains(count, "10") {
		t.Errorf("select all on a page of ten says %q", count)
	}

	// Turning the page drops the selection, which would otherwise delete rows
	// nobody can see.
	p.run("turn the page", p.click(nav+` [data-page="next"]`))
	p.waitForText(nav+` .pager-page`, "Page 2 of 3")
	p.waitGone(card + ` .bulk-bar.shown`)

	// Including on the way back: the rows of the first page were ticked, and
	// coming back to them must not bring the selection back with them.
	p.run("go back", p.click(nav+` [data-page="previous"]`))
	p.waitForText(nav+` .pager-page`, "Page 1 of 3")

	if got := p.count(`#table-projects tbody input.row-pick:checked`); got != 0 || p.visible(card+` .bulk-bar.shown`) {
		t.Errorf("back on the first page, %d rows are still ticked", got)
	}

	p.run("forward again", p.click(nav+` [data-page="next"]`))
	p.waitForText(nav+` .pager-page`, "Page 2 of 3")

	// A document reads every row the table holds, not the page on screen, and
	// leaves the page as it was.
	var figures struct {
		Read   int  `json:"read"`
		Shown  int  `json:"shown"`
		Second bool `json:"second"`
	}

	p.evalJSON(`(() => {
		const read = tableAsFigures(document.querySelector('#table-projects')).rows.length;
		const shown = document.querySelectorAll('#table-projects tbody tr').length;
		return JSON.stringify({
			read, shown,
			second: document.querySelector('#table-projects tbody').textContent.includes('Project 11'),
		});
	})()`, &figures)

	if figures.Read != 30 {
		t.Errorf("a document read %d rows off a table of 30 on pages of ten", figures.Read)
	}

	if figures.Shown != 10 || !figures.Second {
		t.Errorf("after a document was read the table shows %d rows (second page: %v), want page 2 as it was",
			figures.Shown, figures.Second)
	}

	// The search narrows every page at once, whatever page it starts from.
	p.run("search", chromedp.SendKeys(tools+` input.pager-query`, "project 07", chromedp.ByQuery))
	p.waitForText(nav+` .pager-range`, "1–1 of 1")

	if got := p.text(rows); !strings.Contains(got, "Project 07") {
		t.Errorf("searching for project 07 shows %q", got)
	}

	// chromedp refuses to set an empty value, so the field is emptied the way
	// clearing it by hand would be: the value, then the input event.
	p.run("clear the search", chromedp.Evaluate(`(() => {
		const search = document.querySelector('`+tools+` input.pager-query');
		search.value = '';
		search.dispatchEvent(new Event('input', { bubbles: true }));
	})()`, nil))
	p.waitForText(nav+` .pager-range`, "1–10 of 30")

	// The filter offers the statuses the column holds, and narrows to one.
	p.run("show the finished ones", p.chooseOption(tools+` select.pager-choice`, "completed"))
	p.waitForText(nav+` .pager-range`, "1–10 of 10")

	if got := p.text(rows); strings.Contains(got, "Project 01") || !strings.Contains(got, "Project 21") {
		t.Errorf("filtered to the finished projects, the table shows %q", truncateText(got, 200))
	}

	// The page size is kept for the next visit, by this account.
	var kept string

	p.evalJSON(`JSON.stringify(localStorage.getItem('timepulse_pagesize_' + me.user.id + '_table-projects'))`, &kept)

	if kept != "10" {
		t.Errorf("the chosen page size was kept as %q, want 10", kept)
	}
}
