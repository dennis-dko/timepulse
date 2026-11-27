//go:build browser

package browser

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// TestEveryFoldLooksAlike holds every fold, a summary that opens what it heads,
// to one look: the muted colour with a pointer, bold, at the size of the text
// around it. There are four: how to write a booking in one line, what each
// right allows, the two-factor key to type in by hand and the technical details
// under a refusal. Before, they came in three sizes, two colours and two
// weights, and so read as different kinds of thing.
func TestEveryFoldLooksAlike(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyAdmin()

	var looks struct {
		Folds map[string]map[string]string `json:"folds"`
		Text  string                       `json:"text"`
		Muted string                       `json:"muted"`
	}

	p.evalJSON(`JSON.stringify((() => {
		const look = (summary) => {
			const style = getComputedStyle(summary);

			return { color: style.color, cursor: style.cursor, fontSize: style.fontSize, fontWeight: style.fontWeight };
		};
		// The technical details are made when a refusal carries some, so one is
		// made here, in a card as a refusal under a form would stand, and taken
		// away again.
		const refusal = detailDisclosure('connection refused');
		document.querySelector('#form-role').append(refusal);

		const folds = {
			'how to write a line': look(document.querySelector('#form-quick .quick-how > summary')),
			'the rights legend': look(document.querySelector('#permission-legend > summary')),
			'the two-factor key': look(document.querySelector('#totp-manual > summary')),
			'a refusal\'s details': look(refusal.querySelector(':scope > summary')),
		};

		refusal.remove();

		return {
			folds,
			text: getComputedStyle(document.body).fontSize,
			// The line under the quick entry is written in the muted colour.
			muted: getComputedStyle(document.querySelector('#quick-preview')).color,
		};
	})())`, &looks)

	if len(looks.Folds) != 4 {
		t.Fatalf("measured %d folds, want the four this was written for", len(looks.Folds))
	}

	for name, look := range looks.Folds {
		if weight, err := strconv.Atoi(look["fontWeight"]); err != nil || weight < 600 {
			t.Errorf("%s is folded under a summary of weight %q, want bold", name, look["fontWeight"])
		}

		if look["fontSize"] != looks.Text {
			t.Errorf("%s is folded under a summary of %s, want the %s of the text around it", name, look["fontSize"], looks.Text)
		}

		if look["color"] != looks.Muted || look["cursor"] != "pointer" {
			t.Errorf("%s is folded under a summary in %s with a %s cursor, want the muted %s and a pointer",
				name, look["color"], look["cursor"], looks.Muted)
		}
	}
}

// measureFoldSpace answers with every fold on the open screen that stands
// further from or nearer to what is around it than the button space: the
// element above it, the element below it, and, opened, what it unfolds under
// its line. A fold at the foot of its container has the container's padding
// under it rather than the space. A closed fold is opened for the measuring and
// closed again.
const measureFoldSpace = `JSON.stringify((() => {
	const space = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--button-space'));
	// Without it every comparison below is with NaN, which is false, so a page
	// with no token at all would pass.
	if (!Number.isFinite(space)) return ['the stylesheet declares no --button-space to measure against'];

	const seen = (el) => {
		const box = el.getBoundingClientRect();

		return el.offsetParent !== null && getComputedStyle(el).visibility !== 'hidden' && box.width > 0 && box.height > 0;
	};
	const named = (el) => el.id || el.className || el.tagName;
	const problems = [];

	for (const fold of document.querySelectorAll('.view:not([hidden]) details, #login-screen:not([hidden]) details')) {
		if (!seen(fold)) continue;

		const wasOpen = fold.open;
		fold.open = true;

		const box = fold.getBoundingClientRect();
		const summary = fold.querySelector(':scope > summary');

		let above = fold.previousElementSibling;
		while (above && !seen(above)) above = above.previousElementSibling;

		if (above && Math.abs(box.top - above.getBoundingClientRect().bottom - space) > 1) {
			problems.push(named(fold) + ': ' + Math.round(box.top - above.getBoundingClientRect().bottom) + 'px below ' + named(above));
		}

		let first = summary?.nextElementSibling;
		while (first && !seen(first)) first = first.nextElementSibling;

		if (summary && first && Math.abs(first.getBoundingClientRect().top - summary.getBoundingClientRect().bottom - space) > 1) {
			problems.push(named(fold) + ': what it unfolds starts '
				+ Math.round(first.getBoundingClientRect().top - summary.getBoundingClientRect().bottom) + 'px under its line');
		}

		fold.open = wasOpen;

		const closed = fold.getBoundingClientRect();

		let below = fold.nextElementSibling;
		while (below && !seen(below)) below = below.nextElementSibling;

		if (below && Math.abs(below.getBoundingClientRect().top - closed.bottom - space) > 1) {
			problems.push(named(fold) + ': ' + Math.round(below.getBoundingClientRect().top - closed.bottom) + 'px above ' + named(below));
		}
	}

	return problems;
})())`

// TestEveryFoldStandsTheButtonSpaceFromItsNeighbours measures the space round
// every fold on every screen walkEveryScreen opens, and between a fold's line
// and what it unfolds.
//
// Asked for by the maintainer on 2026-11-27: a fold the same distance from
// everything around it as a button. Measured before the change, folds stood 12
// or 14 pixels under what they followed, and what they unfolded 6 or 10 pixels
// under their line.
func TestEveryFoldStandsTheButtonSpaceFromItsNeighbours(t *testing.T) {
	t.Parallel()

	walkEveryScreen(t, (*page).checkFolds)
}

// checkFolds reports every fold on the open screen out of the button space.
func (p *page) checkFolds(t *testing.T, where string) {
	t.Helper()

	var raw string

	p.run("measure the folds of "+where, chromedp.Evaluate(measureFoldSpace, &raw))

	var problems []string

	if err := json.Unmarshal([]byte(raw), &problems); err != nil {
		t.Fatalf("measuring %s answered %q: %v", where, raw, err)
	}

	if len(problems) > 0 {
		t.Errorf("on %s, folds out of the button space: %s", where, strings.Join(problems, "; "))
	}
}
