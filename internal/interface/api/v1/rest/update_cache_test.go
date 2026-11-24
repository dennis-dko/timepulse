package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gofr.dev/pkg/gofr"

	"github.com/dennis-dko/timepulse/internal/infrastructure/announce"
	"github.com/dennis-dko/timepulse/internal/infrastructure/selfupdate"
)

// The release feed is asked rarely, and a refusal does not erase what it said.
//
// It used to be asked at most once a minute, which is sixty times an hour - and
// GitHub allows an unauthenticated caller sixty an hour per address. Every
// administrator's sign-in starts a check and every open tab repeats it hourly,
// all from one address, so an installation with a few administrators spent its
// allowance and got a 403 that reads on screen as a permission problem.
//
// The two rules that follow from that are tested here rather than through the
// handler, because both are about time passing and neither is about HTTP.
func TestTheReleaseAnswerIsKeptAndTheFeedIsNotHammered(t *testing.T) {
	known := selfupdate.Release{Version: "v1.2.3"}

	t.Run("a fresh answer is reused", func(t *testing.T) {
		h := &UpdateHandler{cached: known, cachedAt: time.Now()}

		if !h.hasFreshAnswer() {
			t.Error("an answer taken moments ago is asked for again")
		}
	})

	t.Run("a stale answer is asked again", func(t *testing.T) {
		h := &UpdateHandler{cached: known, cachedAt: time.Now().Add(-updateCacheFor - time.Minute)}

		if h.hasFreshAnswer() {
			t.Error("an answer older than the cache is still being reused")
		}
	})

	t.Run("a refusal is waited out", func(t *testing.T) {
		h := &UpdateHandler{
			cached:   known,
			cachedAt: time.Now().Add(-updateCacheFor - time.Minute),
			cacheErr: errors.New("403"),
			failedAt: time.Now(),
		}

		if !h.waitingAfterFailure() {
			t.Error("a refusal is answered by asking again, which is how a rate " +
				"limit becomes a rate limit that never clears")
		}

		// And what it says meanwhile is still the version it knows.
		if h.cached.Version != known.Version {
			t.Error("the last good answer was thrown away with the failed attempt")
		}
	})

	t.Run("the wait ends", func(t *testing.T) {
		h := &UpdateHandler{
			cacheErr: errors.New("403"),
			failedAt: time.Now().Add(-updateRetryAfter - time.Minute),
		}

		if h.waitingAfterFailure() {
			t.Error("the feed is never asked again after one refusal")
		}
	})

	// Four requests a day rather than sixty an hour, however many people are
	// looking at the screen.
	if perDay := int(24 * time.Hour / updateCacheFor); perDay > 8 {
		t.Errorf("the feed would be asked %d times a day per instance", perDay)
	}
}

// A second press does not silence the warning the first one raised.
//
// Apply announces the update before it starts, so everybody with a screen open
// knows the application is about to restart underneath them. That announcement
// is taken back when the install fails - which is right, because then no restart
// is coming.
//
// It is wrong for the one failure that means the opposite. "Already installing"
// says another call is doing the work and the restart is still on its way; the
// press that got there second has nothing to take back. Taking it back anyway
// left everybody unwarned about an update that was very much still running, and
// the order of two statements was all that stood between the two behaviours.
func TestASecondPressLeavesTheWarningStanding(t *testing.T) {
	const version = "v1.2.3"

	t.Run("turned away, the banner stays", func(t *testing.T) {
		hub := announce.New()
		h := &UpdateHandler{hub: hub}

		hub.Publish(announce.Installing, version)

		if err := h.afterAFailedInstall(selfupdate.ErrInstalling, version); err == nil {
			t.Error("a second press was answered as though it had started an install")
		}

		last, standing := hub.Last()
		if !standing {
			t.Fatal("the second press took down the first one's notice: every open " +
				"screen now expects nothing, and the application restarts anyway")
		}

		if last.Kind != announce.Installing {
			t.Errorf("the notice now says %q, want %q - the install the first press "+
				"started is still running", last.Kind, announce.Installing)
		}
	})

	t.Run("genuinely failed, the banner comes down", func(t *testing.T) {
		hub := announce.New()
		h := &UpdateHandler{hub: hub}

		hub.Publish(announce.Installing, version)

		if err := h.afterAFailedInstall(errors.New("the download was truncated"),
			version); err == nil {
			t.Error("a failed install was answered as a success")
		}

		if last, standing := hub.Last(); standing {
			t.Errorf("a restart notice is still standing after a failed install: %q "+
				"promises something that is not coming", last.Kind)
		}
	})
}

// An administrator who signed in after the feed last answered is owed an answer
// of their own; a sign-in within signInSlack of the answer has as good as seen
// it; and a caller no sign-in brought is owed nothing, before an answer or after.
func TestASignInIsOwedOneAnswerOfItsOwn(t *testing.T) {
	answered := time.Now().Add(-time.Hour)
	h := &UpdateHandler{cached: selfupdate.Release{Version: "v1.2.3"}, cachedAt: answered}

	for name, want := range map[time.Time]bool{
		answered.Add(time.Minute):     true,
		answered.Add(-time.Minute):    false,
		answered.Add(signInSlack / 2): false,
		answered.Add(2 * signInSlack): true,
		{}:                            false,
	} {
		if got := h.owedACheck(name); got != want {
			t.Errorf("signed in %v after the answer: owed a check %v, want %v", name.Sub(answered), got, want)
		}
	}

	nothingYet := &UpdateHandler{}

	if !nothingYet.owedACheck(time.Now()) {
		t.Error("before the feed ever answered, a sign-in is not owed an answer")
	}

	if nothingYet.owedACheck(time.Time{}) {
		t.Error("before the feed ever answered, a token is owed an answer")
	}
}

// A sign-in after a refusal waits it out like the automatic path, so a feed
// turning every request away - a spent rate limit - is not asked again at each
// arrival; with nothing to wait out, the sign-in's answer is recorded.
func TestASignInWaitsOutARefusal(t *testing.T) {
	var asked atomic.Int32

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)

		_, _ = w.Write([]byte(`{"tag_name":"v1.2.4","assets":[]}`))
	}))
	defer feed.Close()

	c := &gofr.Context{Context: context.Background()}
	source := &selfupdate.Source{API: feed.URL, Client: feed.Client()}
	answered := time.Now().Add(-time.Hour)

	refused := &UpdateHandler{
		source: source, cached: selfupdate.Release{Version: "v1.2.3"}, cachedAt: answered,
		cacheErr: errors.New("403"), failedAt: time.Now(),
	}

	refused.checkForASignIn(c, time.Now())

	if got := asked.Load(); got != 0 {
		t.Errorf("a sign-in asked the feed %d times straight into the refusal it is waiting out", got)
	}

	clear := &UpdateHandler{source: source, cached: selfupdate.Release{Version: "v1.2.3"}, cachedAt: answered}

	clear.checkForASignIn(c, time.Now())

	if got := asked.Load(); got != 1 || clear.cached.Version != "v1.2.4" {
		t.Errorf("with nothing to wait out a sign-in asked %d times and keeps %q, want once and v1.2.4",
			got, clear.cached.Version)
	}
}
