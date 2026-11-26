package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dennis-dko/timepulse/internal/infrastructure/selfupdate"
)

// TestAnInstallationOffersTheNewestAppThatFitsIt holds the rule the maintainer
// chose on 2026-11-26: an installation passes on to its phones the newest app
// whose contract it serves, before it is updated itself, and its own release's
// app otherwise. Every case but the first must leave the phone with that own
// app: a newest release no newer than this one, one whose app speaks a contract
// this installation does not serve, one that says no contract, which every
// release before the contract was named does, one carrying no app, and an
// installation that is no release at all.
func TestAnInstallationOffersTheNewestAppThatFitsIt(t *testing.T) {
	t.Parallel()

	newest := func(version, notes string, carriesApp bool) selfupdate.Release {
		t.Helper()

		assets := []map[string]string{}
		if carriesApp {
			assets = append(assets, map[string]string{
				"name":                 "timepulse-mobile_" + version + ".apk",
				"browser_download_url": "https://example.invalid/" + version + ".apk",
			})
		}

		feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": version, "body": notes, "html_url": "https://example.invalid/" + version, "assets": assets,
			})
		}))
		defer feed.Close()

		release, err := selfupdate.New(feed.URL, "").Latest(context.Background())
		if err != nil {
			t.Fatalf("reading the feed: %v", err)
		}

		return release
	}

	fits := fmt.Sprintf("<!-- timepulse-app-contract: %d -->", AppContract)
	beyond := fmt.Sprintf("<!-- timepulse-app-contract: %d -->", AppContract+1)

	got := appFor("v0.4.35", newest("v0.4.40", fits, true))
	if got.Version != "v0.4.40" || got.Download != "https://example.invalid/v0.4.40.apk" || got.Release != "https://example.invalid/v0.4.40" {
		t.Errorf("an installation of v0.4.35 that knows v0.4.40 with an app it serves offers %+v, want that app", got)
	}

	own := androidAnswer("v0.4.35")

	for name, latest := range map[string]selfupdate.Release{
		"the same release":         newest("v0.4.35", fits, true),
		"an older release":         newest("v0.4.30", fits, true),
		"a contract not served":    newest("v0.4.40", beyond, true),
		"a release saying nothing": newest("v0.4.40", "## What changed", true),
		"a release with no app":    newest("v0.4.40", fits, false),
		"no release known":         {},
	} {
		if got := appFor("v0.4.35", latest); got != own {
			t.Errorf("with %s, the installation offers %+v, want its own release's app %+v", name, got, own)
		}
	}

	if got := appFor("dev", newest("v0.4.40", fits, true)); got != androidAnswer("dev") {
		t.Errorf("an installation that is no release offers %+v, want what it offered before", got)
	}
}

// The answer names the release its app belongs to, which the app compares its
// own version with at every start to offer itself anew. Named only beside a
// file to download: an installation that runs no release has no app of its own
// to offer, and a version it could not offer would be one the app tried for.
func TestTheAndroidAppNamesTheReleaseItBelongsTo(t *testing.T) {
	t.Parallel()

	if answer := androidAnswer("v0.4.23"); answer.Version != "v0.4.23" ||
		!strings.HasSuffix(answer.Download, "/v0.4.23/timepulse-mobile_v0.4.23.apk") {
		t.Errorf("the app of v0.4.23 is named %q and downloaded from %q", answer.Version, answer.Download)
	}

	for _, version := range []string{"v0.4.23-3-g30c92cb", "dev", ""} {
		if answer := androidAnswer(version); answer.Version != "" || answer.Download != "" {
			t.Errorf("a build that is %q names the app %q and offers %q, want neither", version, answer.Version, answer.Download)
		}
	}
}
