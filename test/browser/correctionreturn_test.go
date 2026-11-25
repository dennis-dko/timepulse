//go:build browser

package browser

import (
	"testing"

	"github.com/chromedp/chromedp"
)

// A correction opened from the list of entries comes back to it.
//
// The list stands on a tab of its own, and the form that corrects an entry stays
// on the recording tab, where it books one. So "edit" in the list opens the
// form's tab, and saving the correction or giving it up returns to the list,
// rather than leaving somebody on a form with nothing more to do there.
func TestACorrectionOpenedFromTheListComesBackToIt(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	var booked bool

	p.awaitJSON(`(async () => {
		for (const description of ['first', 'second']) {
			await api('/timesheets', { method: 'POST', body: JSON.stringify({ date: todayISO(), durationHours: 1.5, description }) });
		}
		return true;
	})()`, &booked)

	showTimeView(p)
	p.waitForText("#table-timesheets tbody", "second")

	for _, leave := range []struct{ how, button string }{
		{"saved", "#timesheet-submit"},
		{"given up", "#timesheet-cancel"},
	} {
		p.run("correct an entry from the list",
			p.click(`#table-timesheets tbody tr:first-child .actions button.link`),
			chromedp.WaitVisible("#timesheet-cancel", chromedp.ByID))

		if p.visible("#entries-card") {
			t.Fatal("the form that corrects an entry opened beside the list, which is on a tab of its own")
		}

		p.run("leave the correction "+leave.how, p.click(leave.button))
		p.waitShown("#entries-card")

		if p.visible("#form-timesheet") {
			t.Errorf("a correction %s leaves the form on screen rather than the list it came from", leave.how)
		}
	}
}
