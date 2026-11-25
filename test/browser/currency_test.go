//go:build browser

package browser

import (
	"testing"

	"github.com/chromedp/chromedp"
)

// A project's currency is picked from the ones in use rather than typed, each
// offered as its code and its name in the reader's language, in the order of the
// codes. A typed code was held to its shape alone, so a slip saved cleanly as a
// currency that does not exist.
//
// And a project saved with a code the list does not carry - typed while the field
// was a box - opens showing it, keeps it when it is corrected, and does not hand
// it on to the next project.
func TestAProjectsCurrencyIsPickedFromTheOnesInUse(t *testing.T) {
	t.Parallel()

	const picker = `#form-project select[name="currency"]`

	p := open(t)
	p.readyWorker()

	p.run("open Projects", p.click(`.tab[data-view="projects"]`),
		chromedp.WaitVisible("#form-project", chromedp.ByID))

	if n := p.count(picker + ` option`); n < 150 {
		t.Errorf("the picker offers %d currencies, want every one in use", n)
	}

	if got := p.value(picker); got != "EUR" {
		t.Errorf("a new project starts in %q, want the server's EUR", got)
	}

	if got := p.text(picker + ` option[value="EUR"]`); got != "EUR – Euro" {
		t.Errorf("the euro is offered as %q, want its code and its name", got)
	}

	// In the order of the codes, which a code nobody lists has to keep as well.
	inOrder := func(when string) {
		t.Helper()

		var ordered bool

		p.evalJSON(`JSON.stringify([...document.querySelectorAll('`+picker+` option')]
			.every((option, i, all) => i === 0 || all[i - 1].value < option.value))`, &ordered)

		if !ordered {
			t.Errorf("%s the currencies are not in the order of their codes", when)
		}
	}

	inOrder("at the start")

	p.run("make a project charged in Swiss francs",
		chromedp.SendKeys(`#form-project input[name="name"]`, "Zurich", chromedp.ByQuery),
		p.click(`#form-project label.switch .switch-track`),
		chromedp.SetValue(`#form-project input[name="hourlyRate"]`, "120", chromedp.ByQuery),
		chromedp.SetValue(picker, "CHF", chromedp.ByQuery),
		p.click("#project-submit"))
	p.waitForText("#table-projects tbody", "Zurich")

	var currency string

	p.awaitJSON(`api('/projects').then((list) => list.items.find((project) => project.name === 'Zurich').currency)`, &currency)

	if currency != "CHF" {
		t.Errorf("the project was saved in %q, want the CHF picked", currency)
	}

	if got := p.value(picker); got != "EUR" {
		t.Errorf("after the save the form offers %q for the next project, want EUR again", got)
	}

	// A choice not yet saved survives the change of language that renames it.
	p.run("pick the yen", chromedp.SetValue(picker, "JPY", chromedp.ByQuery))
	p.chooseLanguage("de")
	p.waitEvaluates("the Swiss franc's German name",
		`document.querySelector('`+picker+` option[value="CHF"]').textContent`, "CHF – Schweizer Franken")

	if got := p.value(picker); got != "JPY" {
		t.Errorf("the change of language left the picker on %q, want the JPY picked before it", got)
	}

	var older bool

	// Made in euros and given its code in the database, as a project typed in
	// while the field was a box holds one: the server refuses a code ISO 4217
	// does not list to anybody sending one now.
	p.awaitJSON(`(async () => {
		await api('/projects', { method: 'POST', body: JSON.stringify({
			name: 'Altbestand', startDate: todayISO(), billable: true, hourlyRate: 50, currency: 'EUR',
		}) });
		return true;
	})()`, &older)

	if _, err := p.app.DB(t).Exec(`UPDATE projects SET currency = 'XYZ' WHERE name = 'Altbestand'`); err != nil {
		t.Fatalf("give the project a code from before the list: %v", err)
	}

	p.awaitJSON(`refreshAll().then(() => true)`, &older)

	p.waitForText("#table-projects tbody", "Altbestand")

	// Left both ways, and abandoned first: a save is followed by a reload that
	// draws the picker again, which would hide a reset leaving the code behind.
	for _, leave := range []struct{ how, button string }{
		{"cancel editing", "#project-cancel"},
		{"save it unchanged", "#project-submit"},
	} {
		p.run("open the older project", chromedp.Evaluate(`[...document.querySelectorAll('#table-projects tbody tr')]
			.find((row) => row.textContent.includes('Altbestand'))
			.querySelector('button[data-action="edit"]').click()`, nil),
			chromedp.WaitVisible("#project-cancel", chromedp.ByID))

		if got := p.value(picker); got != "XYZ" {
			t.Errorf("the project saved in XYZ opens on %q", got)
		}

		inOrder("with the older project open")

		p.run(leave.how, p.click(leave.button))
		p.waitEvaluates("the form back to making one", `String(document.querySelector('#project-cancel').hidden)`, "true")

		if got := p.value(picker); got != "EUR" || p.count(picker+` option[value="XYZ"]`) != 0 {
			t.Errorf("after %s the next project is offered %q, or the older project's code is still on offer",
				leave.how, got)
		}
	}

	p.awaitJSON(`api('/projects').then((list) => list.items.find((project) => project.name === 'Altbestand').currency)`, &currency)

	if currency != "XYZ" {
		t.Errorf("correcting the project changed its currency to %q", currency)
	}
}
