//go:build browser

package browser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// measureButtonSpace answers with every button on the open screen that stands
// further from or nearer to a neighbour than the button space, which app.css
// declares as --button-space.
//
// Beside a button: the nearest element in the same row of its own container.
// Above it: the nearest element above the row the button stands in - or above
// the button itself where it stands alone in a card, a form or a tile - and
// not one beside it, as a card's title is beside the arrows in its head. A
// button that ends level with a labelled field beside it, as Book does, is
// measured from the top of its row, because what stands above the button there
// is the field's own caption.
//
// Left out, each for its reason: a screen's tabs and the bar's controls, which
// are navigation; a table's buttons, which are a row's; a pager's arrows and a
// chart's legend, which are one control of small parts each; the icons inside
// fields, on tiles and in toasts; and the tour. A link at a tile's foot stands
// there to line up with the tiles beside it, so it is held to at least the
// space rather than to the space.
const measureButtonSpace = `JSON.stringify((() => {
	const space = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--button-space'));
	// Without it every comparison below is with NaN, which is false, so a page
	// with no token at all would pass.
	if (!Number.isFinite(space)) return ['the stylesheet declares no --button-space to measure against'];
	const shown = (el) => el.offsetParent !== null && getComputedStyle(el).visibility !== 'hidden';
	// What a reader sees beside or above a button: not an empty paragraph waiting
	// for a verdict, whose margins collapse through it so it stands flush on what
	// follows, nor a file input kept out of sight at no size.
	const seen = (el) => {
		const box = el.getBoundingClientRect();
		return shown(el) && box.width > 0 && box.height > 0;
	};
	const skip = '.screen-tab, .tab, .topbar *, .nav-toggle, .password-toggle, .date-open, .tile-grip, .tile-hide, '
		+ '.toast-close, .sort-toggle, .pager *, td *, th *, #tour *, .tour-bubble *, .chart-legend *';
	const holders = '.card, form, .tile, .login-fields';
	const problems = [];

	for (const button of document.querySelectorAll('.view:not([hidden]) button, #login-screen:not([hidden]) button')) {
		if (!shown(button) || button.matches(skip)) continue;

		const name = button.id || button.dataset.i18n || button.textContent.trim().slice(0, 30);
		const box = button.getBoundingClientRect();

		// The element before it in its container, where that stands beside it -
		// each pair of neighbours is measured once, from its right-hand one.
		let before = button.previousElementSibling;
		while (before && !seen(before)) before = before.previousElementSibling;
		if (before) {
			const r = before.getBoundingClientRect();
			if (r.bottom > box.top + 1 && r.top < box.bottom - 1 && r.right <= box.left + 1
				&& Math.abs(box.left - r.right - space) > 1) {
				problems.push(name + ': ' + Math.round(box.left - r.right) + 'px beside ' + (before.id || before.className || before.tagName));
			}
		}

		// Above: the row the button stands in, or the button where it stands alone.
		const standing = button.parentElement.matches(holders) ? button : button.parentElement;
		if (!standing.parentElement?.matches(holders)) continue;

		const top = standing.getBoundingClientRect().top;
		let above = standing.previousElementSibling;
		while (above && (!seen(above) || above.getBoundingClientRect().bottom > top + 1)) above = above.previousElementSibling;
		if (!above) continue;

		const gap = top - above.getBoundingClientRect().bottom;
		const atATilesFoot = button.closest('.glance-tile') && button === button.parentElement.lastElementChild;

		if (atATilesFoot ? gap < space - 1 : Math.abs(gap - space) > 1) {
			problems.push(name + ': ' + Math.round(gap) + 'px below ' + (above.id || above.className || above.tagName));
		}
	}

	return problems;
})())`

// TestEveryButtonStandsTheButtonSpaceFromItsNeighbours walks every screen of
// somebody who administers and of somebody who records time, every tab of each,
// and the sign-in screen, measuring the space round every button.
//
// Asked for by the maintainer on 2026-11-25: every button the same distance
// from everything else, left, right, above and below. Measured before the
// change, a row's buttons stood 8, 10 or 12 pixels apart, and a button or a
// row of them 13, 14, 16, 18, 20 or 22 pixels under what it followed - each
// figure the work of whichever rule applied there. One token decides it now.
func TestEveryButtonStandsTheButtonSpaceFromItsNeighbours(t *testing.T) {
	t.Parallel()

	for _, who := range []struct {
		name  string
		ready func(*page)
		views []string
	}{
		{"administrator", (*page).readyAdmin, []string{"welcome", "users", "roles", "settings", "admin"}},
		{"worker", (*page).readyWorker, []string{"welcome", "timesheets", "week", "calendar", "overtime", "projects", "report", "settings"}},
	} {
		t.Run(who.name, func(t *testing.T) {
			t.Parallel()

			p := open(t)
			p.run("a desktop", chromedp.EmulateViewport(1280, 900))
			p.waitShown("#form-login")
			p.check(t, "the sign-in screen")

			who.ready(p)

			for _, view := range who.views {
				p.run("open "+view, chromedp.Evaluate(`switchView('`+view+`')`, nil))
				p.atRest()

				var tabs []string

				p.run("read its tabs", chromedp.Evaluate(
					`[...document.querySelectorAll('#view-`+view+` .screen-tab')].map((tab) => tab.dataset.cardTab)`, &tabs))

				if len(tabs) == 0 {
					p.check(t, view)

					continue
				}

				for _, tab := range tabs {
					p.run("open "+tab, chromedp.Evaluate(
						`document.querySelector('#view-`+view+` .screen-tab[data-card-tab="`+tab+`"]').click()`, nil))
					p.atRest()
					p.check(t, view+", "+tab)
				}
			}
		})
	}
}

// check reports every button on the open screen out of the button space.
func (p *page) check(t *testing.T, where string) {
	t.Helper()

	var raw string

	p.run("measure the buttons of "+where, chromedp.Evaluate(measureButtonSpace, &raw))

	var problems []string

	if err := json.Unmarshal([]byte(raw), &problems); err != nil {
		t.Fatalf("measuring %s answered %q: %v", where, raw, err)
	}

	if len(problems) > 0 {
		t.Errorf("on %s, out of the button space: %s", where, strings.Join(problems, "; "))
	}
}
