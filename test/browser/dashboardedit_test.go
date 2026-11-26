//go:build browser

package browser

import (
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// TestTheTilesAreArrangedOnlyWhileTheDashboardIsBeingCustomized walks the
// greeting's tiles through the way they are arranged now: nothing to arrange
// them with until "Customize dashboard" is pressed, that link itself only where
// the pointer or the keyboard is - and always on a touch screen, which has no
// pointer resting anywhere - and what a put-away said kept for a screen reader
// rather than written under the tiles.
//
// Every tile carried its handle and its cross all the time, with a paragraph on
// how to use them under the tiles, on a screen somebody opens to read figures.
func TestTheTilesAreArrangedOnlyWhileTheDashboardIsBeingCustomized(t *testing.T) {
	t.Parallel()

	// With a mouse, whatever the machine: on Linux without a display the browser
	// reports none, and the link is then drawn as on a touch screen.
	p := openWithBrowser(t, []chromedp.ExecAllocatorOption{aMouse})
	p.readyWorker()

	p.run("open the greeting", chromedp.Evaluate(`switchView('welcome')`, nil))
	p.waitShown("#glance-balance")

	for _, arranging := range []string{"#tile-grip-week", "#tile-hide-week", "#dashboard-foot"} {
		if p.visible(arranging) {
			t.Errorf("%s is on screen before anybody asked to customize the dashboard", arranging)
		}
	}

	p.pointAt("#site-footer")
	p.waitForOpacity("#dashboard-edit", "0")

	p.pointAt("#dashboard")
	p.waitForOpacity("#dashboard-edit", "1")

	p.run("customize the dashboard", p.click("#dashboard-edit"))

	for _, arranging := range []string{"#tile-grip-week", "#tile-hide-week", "#dashboard-foot"} {
		if !p.visible(arranging) {
			t.Errorf("%s is not on screen while the dashboard is being customized", arranging)
		}
	}

	var done string

	p.run("read what it says now", chromedp.Evaluate(`t('dashboard.done', 'Done')`, &done))

	if got := p.text("#dashboard-edit"); got != done {
		t.Errorf("while customizing, the link says %q, want %q", got, done)
	}

	p.run("put the leave away", p.click("#tile-hide-leave"))

	var said struct {
		Text   string  `json:"text"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}

	p.evalJSON(`JSON.stringify((() => {
		const said = document.querySelector('#dashboard-said');
		const box = said.getBoundingClientRect();
		return { text: said.textContent, width: box.width, height: box.height };
	})())`, &said)

	if said.Text == "" {
		t.Error("putting a tile away is said to nobody: the status line is empty")
	}

	if said.Width > 1 || said.Height > 1 {
		t.Errorf("what putting a tile away did is written on screen (%.0fx%.0f): the tile going is "+
			"what a reader sees, and the sentence is for a screen reader", said.Width, said.Height)
	}

	p.run("finish customizing", p.click("#dashboard-edit"))

	for _, arranging := range []string{"#tile-grip-week", "#tile-hide-week", "#dashboard-foot", "#glance-leave"} {
		if p.visible(arranging) {
			t.Errorf("%s is still on screen once customizing is done", arranging)
		}
	}

	// The keyboard reaches it where the pointer is not.
	p.pointAt("#site-footer")
	p.waitForOpacity("#dashboard-edit", "0")
	p.run("reach it from the keyboard", chromedp.KeyEvent(kb.Tab),
		chromedp.Evaluate(`document.querySelector('#dashboard-edit').focus()`, nil))
	p.waitForOpacity("#dashboard-edit", "1")

	// A touch screen has no pointer resting on the tiles.
	p.run("a telephone", chromedp.EmulateViewport(390, 844, chromedp.EmulateMobile, chromedp.EmulateTouch),
		chromedp.Evaluate(`document.activeElement.blur()`, nil))

	var touch bool

	p.run("is it a touch screen", chromedp.Evaluate(`matchMedia('(hover: none)').matches`, &touch))

	if !touch {
		t.Fatal("the telephone is not told it has no pointer to hover with, so this proves nothing")
	}

	p.waitForOpacity("#dashboard-edit", "1")
}

// pointAt moves the mouse to the middle of what selector names.
func (p *page) pointAt(selector string) {
	p.t.Helper()

	var at struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}

	p.evalJSON(fmt.Sprintf(`JSON.stringify((() => {
		const element = document.querySelector(%q);
		element.scrollIntoView({ block: 'center' });
		const r = element.getBoundingClientRect();
		return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
	})())`, selector), &at)

	p.run("point at "+selector, chromedp.MouseEvent(input.MouseMoved, at.X, at.Y))
}

// waitForOpacity waits until what selector names is drawn at the opacity given,
// past any transition on the way there.
func (p *page) waitForOpacity(selector, want string) {
	p.t.Helper()

	var got string

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		p.run("read the opacity of "+selector, chromedp.Evaluate(
			fmt.Sprintf(`getComputedStyle(document.querySelector(%q)).opacity`, selector), &got))

		if got == want {
			return
		}
	}

	p.t.Errorf("%s is drawn at opacity %s, want %s", selector, got, want)
}
