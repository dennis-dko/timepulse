//go:build browser

package browser

import (
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// The greeting shows the figures somebody comes back for, a tile each: the
// running balance as the server counts it, the days of leave left this year,
// and the projects nearest their budget - a tile only where it has something to
// say, each leading to the card that keeps its figure, and none of them left
// behind a sign-out.
func TestTheGreetingShowsTheFiguresSomebodyComesBackFor(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	// Nothing booked, no leave given and no budget anywhere: nothing to show, so
	// no card - and none of the installation's either, which is not this
	// account's to run.
	var before struct {
		Glance    bool `json:"glance"`
		Operation bool `json:"operation"`
	}

	p.awaitJSON(`(async () => {
		await Promise.all([renderGlance(), renderOperation()]);
		return {
			glance: document.querySelector('#welcome-glance').hidden,
			operation: document.querySelector('#welcome-operation').hidden,
		};
	})()`, &before)

	if !before.Glance || !before.Operation {
		t.Errorf("with nothing to show, the figures card is hidden: %v, the installation's: %v",
			before.Glance, before.Operation)
	}

	// Thirty days of leave this year, nine hours booked today on a project with a
	// budget of ten, and a project without one, which has no budget to near.
	var expected struct {
		Balance string `json:"balance"`
		Year    string `json:"year"`
	}

	p.awaitJSON(`(async () => {
		const year = todayISO().slice(0, 4);
		await api('/me/vacation', { method: 'PUT', body: JSON.stringify({ fromYear: Number(year), days: 30 }) });
		const near = await api('/projects', { method: 'POST', body: JSON.stringify({
			name: 'Website relaunch', startDate: year + '-01-01', budgetHours: 10 }) });
		await api('/projects', { method: 'POST', body: JSON.stringify({
			name: 'Unbudgeted errands', startDate: year + '-01-01' }) });
		await api('/timesheets', { method: 'POST', body: JSON.stringify({
			projectId: near.id, date: todayISO(), durationHours: 9 }) });
		return { balance: fmtSignedHours((await api('/me/notices')).balance.hours), year };
	})()`, &expected)

	p.run("open the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#welcome-glance", chromedp.ByID))

	p.waitForText("#glance-balance-value", expected.Balance)

	if label := p.text("#glance-leave-label"); !strings.Contains(label, expected.Year) {
		t.Errorf("the leave tile is labelled %q, want this year, %s", label, expected.Year)
	}

	if left := p.text("#glance-leave-value"); left != "30" {
		t.Errorf("the leave tile reads %q, want the 30 days given and none taken", left)
	}

	budgets := p.text("#glance-budgets-list")
	if !strings.Contains(budgets, "Website relaunch") || !strings.Contains(budgets, "90 %") {
		t.Errorf("the budget tile reads %q, want the project at 90 %% of its budget", budgets)
	}

	if strings.Contains(budgets, "Unbudgeted errands") {
		t.Errorf("the budget tile lists a project without a budget: %q", budgets)
	}

	// The tile leads to the card its figure comes from, on its own tab.
	p.run("open the leave", p.click("#glance-leave-open"))
	p.waitShown("#vacation-card")

	p.run("back to the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#welcome-glance", chromedp.ByID))

	p.chooseLanguage("de")
	p.waitForText("#glance-leave-label", "Resttage")

	// And nothing of it stays for whoever signs in next at the same desk.
	p.run("sign out", p.click("#logout"), chromedp.WaitVisible("#form-login", chromedp.ByID))

	var left []string

	p.run("read what the next account would find", chromedp.Evaluate(`(() => {
		const left = [];

		if (document.querySelector('#glance-budgets-list').childElementCount) left.push('the budgets are still listed');
		for (const said of ['#glance-balance-value', '#glance-leave-value', '#glance-leave-label']) {
			if (document.querySelector(said).textContent.trim()) left.push(said + ' says ' + document.querySelector(said).textContent);
		}
		if (!document.querySelector('#welcome-glance').hidden) left.push('the card is still shown');

		return left;
	})()`, &left))

	for _, what := range left {
		t.Errorf("after the sign-out %s", what)
	}
}

// Whoever runs the installation is shown how it stands on the greeting: the
// version it runs, its newest backup and the latest changes in the words of the
// log, each tile leading to the card that keeps it.
func TestTheGreetingShowsWhoeverRunsTheInstallationHowItStands(t *testing.T) {
	t.Parallel()

	p := openWith(t, "BACKUP_PASSPHRASE=the installation's own passphrase")
	p.readyAdmin()
	p.createAccount(t, "erika@example.com", "erika-password-1", "user")

	var running string

	p.awaitJSON(`(async () => {
		await api('/settings/backups', { method: 'POST' });
		return (await api('/settings/update')).running;
	})()`, &running)

	p.run("open the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#welcome-operation", chromedp.ByID))

	p.waitForText("#glance-changes-list", "Made a backup")

	if changes := p.text("#glance-changes-list"); !strings.Contains(changes, "Created the account Erika") {
		t.Errorf("the latest changes read %q, want the account made before the backup", changes)
	}

	if version := p.text("#glance-version-value"); version != running {
		t.Errorf("the version tile reads %q, want the %q the installation runs", version, running)
	}

	if newest := p.text("#glance-backup-value"); newest == "" || strings.Contains(newest, "None") {
		t.Errorf("after a backup was made, the backup tile reads %q", newest)
	}

	// The built-in administrator records no time, so has no figures of their own.
	var noFigures bool

	p.awaitJSON(`(async () => { await renderGlance(); return document.querySelector('#welcome-glance').hidden; })()`,
		&noFigures)

	if !noFigures {
		t.Error("an account that records no time is shown a card of its own figures")
	}

	p.run("open the backups", p.click("#glance-backup-open"))
	p.waitShown("#backup-card")

	p.run("back to the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#welcome-operation", chromedp.ByID))

	p.chooseLanguage("de")
	p.waitForText("#welcome-operation-title", "Betrieb auf einen Blick")
}
