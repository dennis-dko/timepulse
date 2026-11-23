//go:build browser

package browser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// The leave of the year the card shows leaves as a document: the days away in
// that year, and what is left and what was taken in the card's own words.
func TestTheLeaveLeavesAsADocumentOfItsYear(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	var recorded int

	p.awaitJSON(`(async () => {
		const year = Number(todayISO().slice(0, 4));
		await api('/me/vacation', { method: 'PUT', body: JSON.stringify({ fromYear: year, days: 30 }) });
		const days = await api('/me/absences', { method: 'POST', body: JSON.stringify({
			from: year + '-03-02', to: year + '-03-06', kind: 'vacation',
		}) });
		await loadVacation();
		await loadAbsences();
		return (days?.items ?? []).length;
	})()`, &recorded)

	if recorded == 0 {
		t.Fatal("no working day of that week was recorded as away, so there is nothing to export")
	}

	p.run("open the leave", p.click(`.tab[data-view="calendar"]`),
		p.click(`#calendar-tabs [data-card-tab="leave"]`),
		chromedp.WaitVisible("#vacation-card", chromedp.ByID))

	p.catchDocuments()

	before := p.documentsAsked()
	p.run("export the leave", p.click("#vacation-pdf"))

	asked := p.documentAsked(before, "#vacation-pdf")
	rows := asked.table(t, 0).Rows

	if len(rows) != recorded {
		t.Errorf("the document holds %d days away, want the %d recorded", len(rows), recorded)
	}

	if !strings.Contains(fmt.Sprint(rows), "Leave") {
		t.Errorf("the days away do not say why: %v", rows)
	}

	if !strings.Contains(asked.Subtitle, "Days left") {
		t.Errorf("the line under the title is %q, which does not say how many days are left", asked.Subtitle)
	}

	var title string

	p.evalJSON(`JSON.stringify(window.__documents.at(-1).title)`, &title)

	if !strings.HasPrefix(title, "Leave 20") {
		t.Errorf("the document is called %q, want the leave of its year", title)
	}
}

// The working time leaves as a spreadsheet of the month on screen, in the
// language the screen is in.
func TestTheWorkingTimeLeavesAsASpreadsheetOfTheMonthOnScreen(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	p.run("open the working time", p.click(`.tab[data-view="calendar"]`),
		p.click(`#calendar-tabs [data-card-tab="worktime"]`),
		chromedp.WaitVisible("#workday-card", chromedp.ByID))
	p.waitEvaluates("the month is loaded", `String(Boolean(workDays?.from))`, "true")

	// The download is caught where the page asks for it, so the case reads the
	// request rather than a file the browser saved somewhere.
	p.run("catch the spreadsheet", chromedp.Evaluate(`(() => {
		window.__sheets = [];
		const real = window.fetch;
		window.fetch = async (input, init) => {
			const url = typeof input === 'string' ? input : input.url;
			if (url.includes('/me/workdays/export')) {
				window.__sheets.push(url);
				return new Response(new Blob(['PK']), { status: 200 });
			}
			return real(input, init);
		};
	})()`, nil))

	p.run("export the month", p.click("#workday-xlsx"))
	p.waitEvaluates("the spreadsheet was asked for", `String(window.__sheets.length)`, "1")

	var asked struct {
		URL  string `json:"url"`
		From string `json:"from"`
		To   string `json:"to"`
	}

	p.evalJSON(`JSON.stringify({ url: window.__sheets[0], from: workDays.from, to: workDays.to })`, &asked)

	for _, want := range []string{"from=" + asked.From, "to=" + asked.To, "lang=en"} {
		if !strings.Contains(asked.URL, want) {
			t.Errorf("the spreadsheet was asked for at %q, which does not say %s", asked.URL, want)
		}
	}
}
