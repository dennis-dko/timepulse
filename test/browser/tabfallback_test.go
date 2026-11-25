//go:build browser

package browser

import "testing"

// A tab that only stands in for an empty one is not remembered as a choice.
//
// Before anybody signs in, the cards on the time entries' first tab are hidden by
// their rights, so the screen shows the next tab that holds something - the list
// of entries. That was remembered as though somebody had chosen it, and the
// screen opened on the list for the rest of the session instead of on the form
// that books an entry.
func TestATabStandingInForAnEmptyOneIsNotRemembered(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	p.run("open the time entries", p.click(`.tab[data-view="timesheets"]`))
	p.settled()

	if selected := p.attr(`#timesheets-tabs [data-card-tab="record"]`, "aria-selected"); selected != "true" || !p.visible("#form-timesheet") {
		t.Errorf("the time entries opened on another tab than their first (Record says aria-selected=%q)", selected)
	}

	// And a tab somebody did choose is still remembered.
	p.run("choose the list", p.click(`#timesheets-tabs [data-card-tab="entries"]`),
		p.click(`.tab[data-view="week"]`), p.click(`.tab[data-view="timesheets"]`))
	p.settled()

	if !p.visible("#entries-card") {
		t.Error("the time entries did not come back on the tab that was chosen")
	}
}
