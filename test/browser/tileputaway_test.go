//go:build browser

package browser

import (
	"slices"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// A tile its owner does not need is put away with its cross, waits under the
// tiles to be brought back, stays away on the next visit and on any other
// device - it is kept on the account - and the original arrangement brings
// every tile back.
func TestATileCanBePutAwayAndBroughtBack(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	// keptAway waits for the account to keep exactly these tiles put away.
	keptAway := func(want ...string) {
		t.Helper()

		var got []string

		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
			p.awaitJSON(`api('/me').then((me) => me.user.dashboard.filter((entry) => entry.startsWith('-')))`, &got)

			if slices.Equal(got, want) {
				return
			}
		}

		t.Errorf("the account keeps %v put away, want %v", got, want)
	}

	p.run("open the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#glance-leave", chromedp.ByID))
	p.atRest()

	p.run("put the leave away", p.click("#tile-hide-leave"))

	if p.visible("#glance-leave") || !p.visible("#tile-back-leave") {
		t.Fatalf("the leave tile is still up (%v), or nothing offers it back (%v)",
			p.visible("#glance-leave"), p.visible("#tile-back-leave"))
	}

	keptAway("-leave")

	p.run("come back to the greeting", chromedp.Reload())
	p.settled()
	p.run("open the greeting", p.click("#app-title"),
		chromedp.WaitVisible("#glance-balance", chromedp.ByID))
	p.atRest()

	if p.visible("#glance-leave") {
		t.Error("the leave tile is up again after a reload, so putting it away was not kept")
	}

	p.run("bring the leave back", p.click("#tile-back-leave"),
		chromedp.WaitVisible("#glance-leave", chromedp.ByID))
	keptAway()

	p.run("put the balance away", p.click("#tile-hide-balance"))
	keptAway("-balance")

	p.run("go back to the original arrangement", p.click("#dashboard-reset"),
		chromedp.WaitVisible("#glance-balance", chromedp.ByID))

	if p.visible("#dashboard-put-away") {
		t.Error("the original arrangement still lists a tile as put away")
	}

	keptAway()
}
