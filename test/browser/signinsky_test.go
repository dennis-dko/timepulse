//go:build browser

package browser

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/dennis-dko/timepulse/test/harness"
)

// skyAt is where a screen's sky put the sun or the moon at one moment, and
// whether the stars were out.
type skyAt struct {
	Tint  string  `json:"tint"`
	Sun   bool    `json:"sun"`
	Moon  bool    `json:"moon"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Night string  `json:"night"`
}

// drawSkyAt draws the sky in the svg a selector names at an hour of today, and
// answers with what it drew.
func drawSkyAt(selector string, hour float64) string {
	return `((sky, hour) => {
		const now = new Date();
		now.setHours(Math.floor(hour), Math.round((hour % 1) * 60), 0, 0);
		drawTheSky(sky, now);
		const sun = sky.querySelector('.sky-sun');
		const moon = sky.querySelector('.sky-moon');
		const shown = (body) => getComputedStyle(body).display !== 'none';
		const at = shown(sun) ? sun : moon;
		return JSON.stringify({
			tint: sky.style.getPropertyValue('--sky'),
			sun: shown(sun),
			moon: shown(moon),
			x: Number(at.getAttribute('cx')),
			y: Number(at.getAttribute('cy')),
			night: sky.style.getPropertyValue('--night'),
		});
	})(document.querySelector('` + selector + `'), ` + strconv.FormatFloat(hour, 'f', -1, 64) + `)`
}

// checkTheSky holds a screen's sky to the clock: at one in the afternoon the sun
// at the top of its arc and no stars, at eleven at night the moon three tenths
// of the way along it and every star out.
func checkTheSky(t *testing.T, where string, read func(hour float64) skyAt) {
	t.Helper()

	for _, at := range []struct {
		hour       float64
		sun        bool
		x, y       float64
		night, why string
	}{
		{13, true, 800, 90, "0", "at one in the afternoon"},
		{23, false, 353.3, 239, "1", "at eleven at night"},
	} {
		sky := read(at.hour)

		if sky.Sun != at.sun || sky.Moon == at.sun {
			t.Errorf("%s %s: the sun is out: %v, the moon: %v", where, at.why, sky.Sun, sky.Moon)
		}

		if math.Abs(sky.X-at.x) > 1 || math.Abs(sky.Y-at.y) > 1 {
			t.Errorf("%s %s: it stands at %.1f,%.1f, want %.1f,%.1f", where, at.why, sky.X, sky.Y, at.x, at.y)
		}

		if sky.Night != at.night {
			t.Errorf("%s %s: the stars are out to %s, want %s", where, at.why, sky.Night, at.night)
		}

		if sky.Tint == "" {
			t.Errorf("%s %s: the sky has no colour", where, at.why)
		}
	}
}

// TestTheSignInSkyFollowsTheClock opens the sign-in screen and reads its sky:
// drawn as it opens, behind the card rather than over it, and at the hour the
// reader is in - the background the maintainer chose on 2026-11-25 from seven
// drawn side by side, for every screen somebody signs in, sets up or connects on.
func TestTheSignInSkyFollowsTheClock(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.waitShown("#form-login")

	var opened skyAt

	p.evalJSON(`JSON.stringify({ tint: document.querySelector('#login-sky').style.getPropertyValue('--sky') })`, &opened)

	if opened.Tint == "" {
		t.Error("the sky is not drawn when the sign-in screen opens")
	}

	var behind struct {
		Card bool `json:"card"`
		Sky  bool `json:"sky"`
	}

	// The sky takes no pointer, so the browser's own answer to "what is here"
	// passes over it; it is asked with the sky taking one for the moment.
	p.evalJSON(`JSON.stringify((() => {
		const sky = document.querySelector('#login-sky');
		sky.style.pointerEvents = 'auto';
		const card = document.querySelector('#form-login').getBoundingClientRect();
		const middle = document.elementFromPoint(card.left + card.width / 2, card.top + card.height / 2);
		const corner = document.elementFromPoint(4, 4);
		sky.style.pointerEvents = '';
		return { card: Boolean(middle?.closest('#form-login')), sky: Boolean(corner?.closest('#login-sky')) };
	})())`, &behind)

	if !behind.Card || !behind.Sky {
		t.Errorf("the sky is not behind the card: the card is on top in its middle: %v, the sky in a corner: %v",
			behind.Card, behind.Sky)
	}

	checkTheSky(t, "the sign-in screen", func(hour float64) skyAt {
		var sky skyAt

		p.evalJSON(drawSkyAt("#login-sky", hour), &sky)

		return sky
	})
}

// TestTheInstallerDrawsTheSameSky opens the installer, which a new installation
// shows before it has a database, and reads the same sky there.
func TestTheInstallerDrawsTheSameSky(t *testing.T) {
	t.Parallel()

	app := harness.StartUnconfigured(t)

	ctx, done := installerInGerman(t, app)
	defer done()

	read := func(expression string, into any) {
		t.Helper()

		var raw string

		if err := chromedp.Run(ctx, chromedp.Evaluate(expression, &raw)); err != nil {
			t.Fatalf("reading the installer's sky: %v", err)
		}

		if err := json.Unmarshal([]byte(raw), into); err != nil {
			t.Fatalf("reading the installer's sky answered %q: %v", raw, err)
		}
	}

	var opened skyAt

	read(`JSON.stringify({ tint: document.querySelector('#install-sky').style.getPropertyValue('--sky') })`, &opened)

	if opened.Tint == "" {
		t.Error("the installer's sky is not drawn when it opens")
	}

	checkTheSky(t, "the installer", func(hour float64) skyAt {
		var sky skyAt

		read(drawSkyAt("#install-sky", hour), &sky)

		return sky
	})
}
