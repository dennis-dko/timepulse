//go:build browser

package browser

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/xuri/excelize/v2"
)

// Every evaluation with a PDF leaves as a workbook too, and so do the year and
// the days away, which had neither: the screen's own title over its figures,
// with the hours written as numbers a sheet can add up.
func TestEveryEvaluationLeavesAsAWorkbookToo(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()
	p.catchWorkbooks()

	var booked bool

	p.awaitJSON(`(async () => {
		const project = await api('/projects', { method: 'POST', body: JSON.stringify({ name: 'Roof', startDate: todayISO() }) });
		await api('/timesheets', { method: 'POST', body: JSON.stringify({ projectId: project.id, date: todayISO(), durationHours: 2.5 }) });
		const sick = new Date(Date.UTC(new Date().getFullYear(), 0, 5)).toISOString().slice(0, 10);
		await api('/me/absences', { method: 'POST', body: JSON.stringify({ from: sick, to: sick, kind: 'sick' }) });
		await refreshAll();
		return true;
	})()`, &booked)

	for _, screen := range []struct {
		view, ask, wait, button, title string
		figures                        bool
	}{
		{"report", `#form-report button[type="submit"]`, "#report-result", "#report-xlsx", `#report-tabs [data-card-tab="report"]`, true},
		{"overtime", "#statistics-load", "#statistics-card", "#statistics-xlsx", "#statistics-card h2", true},
		{"overtime", `#form-overtime button[type="submit"]`, "#overtime-result", "#overtime-xlsx", "", true},
		{"overtime", "", "#year-card", "#year-xlsx", "#year-title", true},
		{"calendar", "", "#absence-card", "#absence-xlsx", "#absence-card h2", false},
	} {
		p.run("open "+screen.view, p.click(`.tab[data-view="`+screen.view+`"]`))
		// Through the page's own opener, which leaves a screen without tabs as it is.
		p.run("open its tab", chromedp.Evaluate(`openTheTabOf(document.querySelector('`+screen.button+`')); true`, nil))

		if screen.ask != "" {
			p.run("work it out", p.click(screen.ask))
		}

		p.waitShown(screen.wait)
		p.atRest()

		before := p.workbooksAsked()
		p.run("export "+screen.button, p.click(screen.button))

		book := p.workbookAnswered(before, screen.button)
		sheet := book.GetSheetList()[0]

		if screen.title != "" {
			title, _ := book.GetCellValue(sheet, "A1")
			if shown := p.text(screen.title); title != shown {
				t.Errorf("%s: the workbook is titled %q, and the screen %q", screen.button, title, shown)
			}
		}

		if screen.figures && !holdsANumber(book, sheet) {
			t.Errorf("%s: the workbook holds no figure written as a number", screen.button)
		}

		_ = book.Close()
	}

	// And the two that had no PDF either: the year's months, and the days away.
	p.catchDocuments()

	for _, screen := range []struct{ view, button, rows string }{
		{"overtime", "#year-pdf", "#table-year tbody tr"},
		{"calendar", "#absence-pdf", "#table-absences tbody tr"},
	} {
		p.run("open "+screen.view, p.click(`.tab[data-view="`+screen.view+`"]`))
		p.run("open its tab", chromedp.Evaluate(`openTheTabOf(document.querySelector('`+screen.button+`')); true`, nil))
		p.atRest()

		before := p.documentsAsked()
		p.run("export "+screen.button, p.click(screen.button))

		asked := p.documentAsked(before, screen.button)

		if len(asked.Sections) != 1 || asked.Sections[0].Table == nil || len(asked.Sections[0].Table.Rows) != p.count(screen.rows) {
			t.Errorf("%s: the document does not carry the table on screen: %+v", screen.button, asked.Sections)
		}
	}
}

// catchWorkbooks keeps a copy of every workbook the server answers with, while
// letting the page save it as it would.
func (p *page) catchWorkbooks() {
	p.t.Helper()

	p.run("catch the workbooks", chromedp.Evaluate(`(() => {
		window.__workbooks = [];

		const real = window.fetch;
		window.fetch = async (input, init) => {
			const url = typeof input === 'string' ? input : input.url;
			const res = await real(input, init);

			if (url.includes('/exports/workbook') && res.ok) {
				const bytes = new Uint8Array(await res.clone().arrayBuffer());
				let binary = '';
				for (const byte of bytes) binary += String.fromCharCode(byte);
				window.__workbooks.push(btoa(binary));
			}

			return res;
		};

		return 1;
	})()`, nil))
}

// workbooksAsked is how many workbooks have come back so far.
func (p *page) workbooksAsked() int {
	p.t.Helper()

	var asked int

	p.run("count the workbooks", chromedp.Evaluate(`window.__workbooks.length`, &asked))

	return asked
}

// workbookAnswered waits for a workbook after the first before, and opens it.
func (p *page) workbookAnswered(before int, button string) *excelize.File {
	p.t.Helper()

	deadline := time.Now().Add(waitPatience)

	for time.Now().Before(deadline) {
		var encoded string

		p.run("look for the workbook", chromedp.Evaluate(
			`window.__workbooks.length > `+strconv.Itoa(before)+` ? window.__workbooks.at(-1) : ''`, &encoded))

		if encoded != "" {
			raw, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				p.t.Fatalf("%s: the workbook did not come back whole: %v", button, err)
			}

			book, err := excelize.OpenReader(bytes.NewReader(raw))
			if err != nil {
				p.t.Fatalf("%s: what came back is not a workbook: %v", button, err)
			}

			return book
		}

		time.Sleep(100 * time.Millisecond)
	}

	p.t.Fatalf("%s never came back as a workbook", button)

	return nil
}

// holdsANumber says whether any cell of the sheet is written as a number.
func holdsANumber(book *excelize.File, sheet string) bool {
	rows, err := book.GetRows(sheet)
	if err != nil {
		return false
	}

	for r, row := range rows {
		for c := range row {
			name, _ := excelize.CoordinatesToCellName(c+1, r+1)

			if kind, _ := book.GetCellType(sheet, name); kind == excelize.CellTypeNumber || kind == excelize.CellTypeUnset && row[c] != "" && isNumber(book, sheet, name) {
				return true
			}
		}
	}

	return false
}

func isNumber(book *excelize.File, sheet, cell string) bool {
	raw, err := book.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		return false
	}

	_, err = strconv.ParseFloat(raw, 64)

	return err == nil
}
