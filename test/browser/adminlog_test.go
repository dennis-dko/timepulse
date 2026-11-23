//go:build browser

package browser

import (
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// An account made by the administrator stands in the log on the Settings screen
// as a sentence naming it, under the administrator who made it - in English, and
// in German once the screen is.
func TestTheSettingsScreenShowsWhoChangedWhat(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyAdmin()
	p.createAccount(t, "erika@example.com", "erika-password-1", "user")

	// Made past the interface, which reloads what a save of its own changed and
	// nothing else; the log is read again as the account screen's save would.
	var read bool

	p.awaitJSON(`(async () => { await loadChanges(); return true; })()`, &read)

	p.run("open Settings", p.click(`.tab[data-view="admin"]`),
		p.click(`#admin-tabs [data-card-tab="logs"]`),
		chromedp.WaitVisible("#changes-card", chromedp.ByID))

	p.waitForText("#table-changes tbody", "Created the account Erika")

	row := p.text("#table-changes tbody tr:first-child")
	if !strings.Contains(row, "Created the account Erika") {
		t.Errorf("the newest change reads %q, want the account Erika created", row)
	}

	p.chooseLanguage("de")
	p.waitForText("#table-changes tbody", "Konto Erika angelegt")
}

// The log on its card is searched, narrowed to a part of the installation and
// paged - by the server, over every change it holds, so a name finds its change
// however far back it is.
func TestTheChangeLogIsSearchedNarrowedAndPagedOnItsCard(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyAdmin()

	const (
		rows   = `#table-changes tbody tr`
		nav    = `#changes-card .pager`
		range_ = nav + ` .pager-range`
	)

	// Twelve roles and an account, each a change of its own.
	var total int

	p.awaitJSON(`(async () => {
		for (let i = 1; i <= 12; i++) {
			await api('/roles', { method: 'POST', body: JSON.stringify({
				name: 'reviewer-' + String(i).padStart(2, '0'), description: 'Reads what others booked',
				permissions: ['timesheets:read:own'],
			}) });
		}
		await api('/users', { method: 'POST', body: JSON.stringify({
			name: 'Greta Grün', email: 'greta@example.com', role: 'user', password: 'greta-password-1',
		}) });
		await loadChanges();
		return (await api('/admin/changes?limit=1')).totalCount;
	})()`, &total)

	if total < 13 {
		t.Fatalf("the log counts %d changes after thirteen were made", total)
	}

	p.run("open Settings", p.click(`.tab[data-view="admin"]`),
		p.click(`#admin-tabs [data-card-tab="logs"]`),
		chromedp.WaitVisible("#changes-card", chromedp.ByID))

	p.waitShown(nav)

	// Ten a page, and the steps to the rest.
	p.run("show ten a page", p.chooseOption(nav+` select.pager-size`, "10"))
	p.waitForText(nav+` .pager-page`, "Page 1 of 2")

	if got := p.count(rows); got != 10 {
		t.Errorf("a page of ten shows %d changes", got)
	}

	p.run("go to the next page", p.click(nav+` [data-page="next"]`))
	p.waitForText(nav+` .pager-page`, "Page 2 of 2")

	if got := p.count(rows); got != total-10 {
		t.Errorf("the second page shows %d changes, want the remaining %d", got, total-10)
	}

	// Narrowed to the roles, from the first page again, counted by the server.
	p.run("show the role changes", p.chooseOption(`#filter-changes-area`, "roles"))
	p.waitForText(range_, "1–10 of 12")

	if got := p.text(rows); strings.Contains(got, "Greta") {
		t.Errorf("narrowed to the roles, the log shows %q", truncateText(got, 200))
	}

	// A name found in every change, not in the page on screen, whatever its case.
	p.run("show every kind again", p.chooseOption(`#filter-changes-area`, ""))
	// Waited for by the count rather than by her name: as the newest change she
	// is on the first page before the search has been asked at all.
	p.run("search for her", chromedp.SendKeys(`#filter-changes-q`, "grün", chromedp.ByQuery))
	p.waitForRows(rows, 1)

	if got := p.text(rows); !strings.Contains(got, "Created the account Greta Grün") || p.visible(nav) {
		t.Errorf("searching for her shows %q, and a pager: %v", got, p.visible(nav))
	}

	// And a search nothing matches says so, rather than that nothing was changed.
	p.run("search for nobody", chromedp.SendKeys(`#filter-changes-q`, "-nobody", chromedp.ByQuery))
	p.waitForText(rows, "No change matches")
}

// The log is found by the words its sentences say on screen, in the reader's
// language - "Konto" finds the accounts made, "Rolle" the roles - and not only by
// the names in them and the codes the server keeps.
//
// The server keeps user.created and words nothing; the screen says "Konto Greta
// Grün angelegt". A search answered by the server alone found the names and
// nothing else, so the words somebody reads in the log were the words that could
// not be searched for.
func TestTheChangeLogIsFoundByTheWordsItsSentencesSay(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyAdmin()

	var made bool

	p.awaitJSON(`(async () => {
		await api('/roles', { method: 'POST', body: JSON.stringify({
			name: 'reviewer-01', description: 'Reads what others booked', permissions: ['timesheets:read:own'],
		}) });
		await api('/users', { method: 'POST', body: JSON.stringify({
			name: 'Greta Grün', email: 'greta@example.com', role: 'user', password: 'greta-password-1',
		}) });
		await loadChanges();
		return true;
	})()`, &made)

	p.chooseLanguage("de")

	p.run("open Settings", p.click(`.tab[data-view="admin"]`),
		p.click(`#admin-tabs [data-card-tab="logs"]`),
		chromedp.WaitVisible("#changes-card", chromedp.ByID))

	const rows = `#table-changes tbody`

	search := func(term string) {
		p.run("search for "+term, chromedp.Evaluate(`(() => {
			const field = document.querySelector('#filter-changes-q');
			field.value = '`+term+`';
			field.dispatchEvent(new Event('input', { bubbles: true }));
		})()`, nil))
	}

	// Each search is waited for by what it leaves out, which only the answer can
	// do: what it should find is on the first page before the search is asked.
	answered := func(term, leftOut string) string {
		search(term)
		p.waitFor("the search for "+term+" to leave out "+leftOut, func() bool {
			return !strings.Contains(p.text(rows), leftOut)
		})

		return p.text(rows)
	}

	if got := answered("Konto", "reviewer-01"); !strings.Contains(got, "Konto Greta Grün angelegt") {
		t.Errorf("searching for Konto found %q, want the account made", truncateText(got, 200))
	}

	if got := answered("Rolle", "Greta"); !strings.Contains(got, "Rolle reviewer-01 angelegt") {
		t.Errorf("searching for Rolle found %q, want the role made", truncateText(got, 200))
	}

	// After the search for the role, what this one should add is what tells it
	// has answered: the account made is not on screen until it does.
	search("angelegt")
	p.waitForText(rows, "Konto Greta Grün angelegt")

	if got := p.text(rows); !strings.Contains(got, "Rolle reviewer-01 angelegt") || strings.Contains(got, "Einrichtung") {
		t.Errorf("searching for angelegt found %q, want the role and the account made and nothing else", truncateText(got, 200))
	}
}
