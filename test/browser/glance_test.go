//go:build browser

package browser

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// The greeting shows the figures somebody comes back for, a tile each: the
// running balance as the server counts it, the days of leave left this year,
// and the projects nearest their budget - every one of them, saying so where
// it holds nothing yet, each leading to the card that keeps its figure, and
// none of them left behind a sign-out.
func TestTheGreetingShowsTheFiguresSomebodyComesBackFor(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	// Nothing booked, no leave given and no budget anywhere: every figure tile
	// is there all the same and says what will fill it - and none of the
	// installation's, which is not this account's to run.
	var empty struct {
		Shown   []string `json:"shown"`
		Balance string   `json:"balance"`
		Leave   string   `json:"leave"`
		Budgets bool     `json:"budgets"`
	}

	p.awaitJSON(`(async () => {
		await Promise.all([renderGlance(), renderOperation()]);
		return {
			shown: [...document.querySelectorAll('#dashboard > .tile')]
				.filter((tile) => !tile.hidden && tile.classList.contains('glance-tile'))
				.map((tile) => tile.dataset.tile),
			balance: document.querySelector('#glance-balance-note').textContent,
			leave: document.querySelector('#glance-leave-note').textContent,
			budgets: !document.querySelector('#glance-budgets-empty').hidden,
		};
	})()`, &empty)

	if strings.Join(empty.Shown, ",") != "balance,leave,budgets" {
		t.Errorf("with nothing to show, the figure tiles shown are %v, want balance, leave and budgets", empty.Shown)
	}

	if !strings.Contains(empty.Balance, "Nothing counts") || !strings.Contains(empty.Leave, "No leave") || !empty.Budgets {
		t.Errorf("the empty tiles say %q, %q and the budgets' sentence shown: %v", empty.Balance, empty.Leave, empty.Budgets)
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
		chromedp.WaitVisible("#glance-balance", chromedp.ByID))

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

	if strings.Contains(budgets, "Unbudgeted errands") || p.visible("#glance-budgets-empty") {
		t.Errorf("the budget tile lists a project without a budget, or still says there is none: %q", budgets)
	}

	// The tile leads to the card its figure comes from, on its own tab.
	p.run("open the leave", p.click("#glance-leave-open"))
	p.waitShown("#vacation-card")

	p.run("back to the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#glance-leave", chromedp.ByID))

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
		for (const tile of document.querySelectorAll('#dashboard > .tile')) {
			if (!tile.hidden) left.push('the tile ' + tile.dataset.tile + ' is still shown');
		}

		return left;
	})()`, &left))

	for _, what := range left {
		t.Errorf("after the sign-out %s", what)
	}
}

// Whoever runs the installation is shown how it stands on the greeting: the
// version it runs, its newest backup and the latest changes in the words of the
// log, each tile leading to the card that keeps it - and no figure tile, for an
// account that records no time.
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
		chromedp.WaitVisible("#glance-changes", chromedp.ByID))

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

	var figures []string

	p.awaitJSON(`(async () => {
		await renderGlance();
		return ['#glance-balance', '#glance-leave', '#glance-budgets']
			.filter((tile) => !document.querySelector(tile).hidden);
	})()`, &figures)

	if len(figures) != 0 {
		t.Errorf("an account that records no time is shown the figure tiles %v", figures)
	}

	p.run("open the backups", p.click("#glance-backup-open"))
	p.waitShown("#backup-card")

	p.run("back to the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#glance-backup", chromedp.ByID))

	p.chooseLanguage("de")
	p.waitForText("#glance-backup-title", "Neueste Sicherung")
}

// The tiles are put where their owner wants them: with the arrow keys on a
// tile's handle, which says where it now stands, and by dragging the handle.
// The order is kept on the account, so it is there again after a reload; the
// way back to the shipped order is offered once there is another; and the next
// account at the same desk is shown the order the screen ships with.
func TestTheTilesArePutWhereTheirOwnerWantsThem(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	order := func() string {
		var names string

		p.run("read the order", chromedp.Evaluate(`[...document.querySelectorAll('#dashboard > .tile')]
			.filter((tile) => !tile.hidden).map((tile) => tile.dataset.tile).join(',')`, &names))

		return names
	}

	kept := func(want string) {
		t.Helper()

		var got string

		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
			p.awaitJSON(`(async () => (await api('/me')).user.dashboard.slice(0, 2).join(','))()`, &got)

			if got == want {
				return
			}
		}

		t.Errorf("the account keeps an arrangement beginning %q, want %q", got, want)
	}

	p.run("open the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#glance-leave", chromedp.ByID))
	p.waitForRows("#welcome-week-bars li", 7)
	p.run("customize the dashboard", p.click("#dashboard-edit"))

	shipped := order()

	if !strings.HasPrefix(shipped, "week,balance,leave") || p.visible("#dashboard-reset") {
		t.Fatalf("the tiles stand %q before anything was moved, and the way back is offered: %v",
			shipped, p.visible("#dashboard-reset"))
	}

	// By the keyboard: the leave tile to the front.
	p.run("move the leave to the front", chromedp.Focus("#tile-grip-leave", chromedp.ByID),
		chromedp.KeyEvent(kb.Home))

	if got := order(); !strings.HasPrefix(got, "leave,week,balance") {
		t.Errorf("after Home on the leave tile's handle, the tiles stand %q", got)
	}

	// Every reading of the account draws the greeting again, and one taken before
	// that save landed must not put the tile back where it came from. Forced
	// rather than waited for: it is lost only when a reading is in flight.
	var drawn bool

	p.awaitJSON(`(async () => {
		me.user = { ...me.user, dashboard: [] };
		await renderWelcome();
		return true;
	})()`, &drawn)

	if got := order(); !strings.HasPrefix(got, "leave,week,balance") {
		t.Errorf("a reading taken before the save put the tiles back: they stand %q", got)
	}

	p.waitForText("#dashboard-said", "place 1 of")
	kept("leave,week")

	// Kept on the account rather than in this page.
	p.run("reload", chromedp.Reload(), chromedp.WaitVisible("#tabs", chromedp.ByID))
	p.settled()
	p.run("open the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#glance-leave", chromedp.ByID))
	p.run("customize it again", p.click("#dashboard-edit"))

	if got := order(); !strings.HasPrefix(got, "leave,week,balance") || !p.visible("#dashboard-reset") {
		t.Errorf("after a reload the tiles stand %q, and the way back is offered: %v", got, p.visible("#dashboard-reset"))
	}

	// By dragging: the week's handle over the balance tile puts the week there.
	var move struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}

	p.evalJSON(`JSON.stringify((() => {
		const grip = document.querySelector('#tile-grip-week').getBoundingClientRect();
		const over = document.querySelector('#glance-balance').getBoundingClientRect();
		return { x: over.left + over.width / 2 - (grip.left + grip.width / 2),
			y: over.top + over.height / 2 - (grip.top + grip.height / 2) };
	})())`, &move)

	p.drag("#tile-grip-week", move.X, move.Y)

	if got := order(); !strings.HasPrefix(got, "leave,balance,week") {
		t.Errorf("after dragging the week over the balance, the tiles stand %q", got)
	}

	kept("leave,balance")

	// Back to the order the screen ships with.
	p.run("go back to the shipped order", p.click("#dashboard-reset"))

	if got := order(); got != shipped {
		t.Errorf("after going back, the tiles stand %q, want %q", got, shipped)
	}

	kept("")

	// Moved again, and somebody else signs in at the same desk.
	p.run("move the leave to the front again", chromedp.Focus("#tile-grip-leave", chromedp.ByID),
		chromedp.KeyEvent(kb.Home))
	kept("leave,week")

	p.run("sign out", p.click("#logout"), chromedp.WaitVisible("#form-login", chromedp.ByID))

	var after string

	p.run("read the order the next account starts from", chromedp.Evaluate(
		`[...document.querySelectorAll('#dashboard > .tile')].map((tile) => tile.dataset.tile).join(',')`, &after))

	if !strings.HasPrefix(after, "week,balance,leave") {
		t.Errorf("after the sign-out the tiles stand %q, the last account's order, want the shipped one", after)
	}
}
