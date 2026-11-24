//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Every administrator's sign-in asks the release feed once, whatever the cache
// holds: somebody arriving to run the installation is shown what is out now,
// not what was out up to six hours ago. A second look in the same session is
// answered from what that sign-in was told, and a personal token - a script, an
// assistant - asks nothing, because nobody signed in.
func TestEveryAdministratorsSignInAsksTheReleaseFeedOnce(t *testing.T) {
	t.Parallel()

	var asked atomic.Int32

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)

		_, _ = w.Write([]byte(`{"tag_name":"v99.0.0","body":"","html_url":"https://example.invalid/r","assets":[]}`))
	}))
	defer feed.Close()

	a := start(t, "UPDATE_FEED="+feed.URL)
	admin := a.signInAsAdmin("a-much-better-password")

	admin.must(admin.api(http.MethodGet, "/settings/update", nil), http.StatusOK)

	if got := asked.Load(); got != 1 {
		t.Fatalf("the first look after a sign-in asked the feed %d times, want once", got)
	}

	admin.must(admin.api(http.MethodGet, "/settings/update", nil), http.StatusOK)

	if got := asked.Load(); got != 1 {
		t.Errorf("a second look in the same session asked the feed again: %d times in all", got)
	}

	// A session's opening is read back from the database, which on MySQL keeps
	// whole seconds, and a sign-in within a second of an answer counts as having
	// seen it. So the next one comes clearly after.
	time.Sleep(2 * time.Second)

	again := a.newClient()
	again.signIn(adminEmail, "a-much-better-password")
	again.must(again.api(http.MethodGet, "/settings/update", nil), http.StatusOK)

	if got := asked.Load(); got != 2 {
		t.Errorf("a new sign-in was answered from the cache: the feed was asked %d times in all, want twice", got)
	}

	var token struct {
		Secret string `json:"secret"`
	}

	again.must(again.api(http.MethodPost, "/me/tokens", map[string]any{"name": "script", "expiresInDays": 0}),
		http.StatusCreated, http.StatusOK).Data(t, &token)

	time.Sleep(2 * time.Second)

	req := rawRequest(t, http.MethodGet, a.BaseURL()+"/api/v1/settings/update", "")
	req.Header.Set("Authorization", "Bearer "+token.Secret)

	if resp := sendPlain(t, req); resp.Status != http.StatusOK {
		t.Fatalf("the token was answered %d: %s", resp.Status, resp.Body)
	}

	if got := asked.Load(); got != 2 {
		t.Errorf("a personal token's look asked the feed: %d times in all, want still twice", got)
	}
}
