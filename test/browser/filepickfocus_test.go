//go:build browser

package browser

import (
	"strings"
	"testing"
	"time"

	cdppage "github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// TestAFileChooserIsAButtonTheKeyboardReachesAndOpens reaches the import from
// the keyboard, asks whether it shows the ring and whether the hidden input stays
// out of the way, and presses Enter on it.
//
// The chooser was a label round an input hidden by design, so the browser's focus
// ring went round an element of no size and Tab landed on nothing anybody could
// see, until a rule put the ring on the label. A button needs none of that: the
// input is out of the tab order, and the button opening it takes the focus, draws
// the ring every button draws, and opens the chooser on Enter.
func TestAFileChooserIsAButtonTheKeyboardReachesAndOpens(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	p.run("open the time entries", p.click(`.tab[data-view="timesheets"]`), p.click(`#timesheets-tabs [data-card-tab="sheet"]`))
	p.waitShown("#workbook-card")

	opened := make(chan struct{}, 1)

	chromedp.ListenTarget(p.ctx, func(event any) {
		if _, ok := event.(*cdppage.EventFileChooserOpened); ok {
			select {
			case opened <- struct{}{}:
			default:
			}
		}
	})

	p.run("catch the chooser rather than open it", cdppage.SetInterceptFileChooserDialog(true))

	// A key first, so the focus that follows counts as the keyboard's.
	p.run("reach the import from the keyboard", chromedp.KeyEvent(kb.Tab),
		chromedp.Evaluate(`document.querySelector('[data-file="wb-file"]').focus()`, nil))

	var state struct {
		Keyboard bool   `json:"keyboard"`
		Outline  string `json:"outline"`
		Tabbable bool   `json:"tabbable"`
	}

	p.evalJSON(`JSON.stringify((() => {
		const button = document.querySelector('[data-file="wb-file"]');
		const drawn = getComputedStyle(button);

		return {
			keyboard: button.matches(':focus-visible'),
			outline: drawn.outlineStyle + ' ' + drawn.outlineWidth,
			tabbable: document.querySelector('#wb-file').tabIndex >= 0,
		};
	})())`, &state)

	if !state.Keyboard {
		t.Fatal("the import button did not take the keyboard's focus, so this case checks nothing")
	}

	if strings.HasPrefix(state.Outline, "none") {
		t.Errorf("with the keyboard on the import button it draws no ring (%s)", state.Outline)
	}

	if state.Tabbable {
		t.Error("the file input is in the tab order, so Tab lands on an element nobody can see")
	}

	p.run("press Enter on it", chromedp.KeyEvent(kb.Enter))

	select {
	case <-opened:
	case <-time.After(10 * time.Second):
		t.Error("Enter on the import button opened no file chooser")
	}
}
