package selfupdate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The Android app is downloaded from the release this installation runs, because
// the app belongs to the release it was built with: its page, and the APK itself.
// A build that is no release's has no release of its own, and is sent to the
// newest one's page with no file named.
func TestTheAndroidAppIsTheOneOfThisRelease(t *testing.T) {
	for version, want := range map[string][2]string{
		"v0.4.6": {Releases + "/tag/v0.4.6", Releases + "/download/v0.4.6/timepulse-mobile_v0.4.6.apk"},
		"v1.0.0": {Releases + "/tag/v1.0.0", Releases + "/download/v1.0.0/timepulse-mobile_v1.0.0.apk"},

		// What a build from a source tree says - how far past its last tag, a
		// changed tree, no tag at all - and what a build says without a version.
		"v0.4.6-3-g30c92cb":       {Releases + "/latest", ""},
		"v0.4.6-3-g30c92cb-dirty": {Releases + "/latest", ""},
		"v0.4.6-dirty":            {Releases + "/latest", ""},
		"30c92cb":                 {Releases + "/latest", ""},
		"0.0.0-dev":               {Releases + "/latest", ""},
		"0.4.6":                   {Releases + "/latest", ""},
		"dev":                     {Releases + "/latest", ""},
		"":                        {Releases + "/latest", ""},
	} {
		page, download := AndroidApp(version)

		if page != want[0] || download != want[1] {
			t.Errorf("the app of %q is at %q, downloaded from %q; want %q and %q", version, page, download, want[0], want[1])
		}
	}
}

// The release publishes the app under the name this version sends a phone to.
// Named anywhere else, the code on the card would download nothing at all.
func TestTheReleasePublishesTheAndroidAppUnderTheNameItIsAskedFor(t *testing.T) {
	raw, err := os.ReadFile("../../../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("reading the release workflow: %v", err)
	}

	asked := androidAssetName("${VERSION}")

	if !regexp.MustCompile(`(?m)^\s+cp "\$\{apk\}" "dist/` + regexp.QuoteMeta(asked) + `"\s*$`).Match(raw) {
		t.Errorf("the release copies no APK to dist/%s, the name this version asks for", asked)
	}

	if !regexp.MustCompile(`(?m)^\s+dist/timepulse-mobile_\*\s*$`).Match(raw) {
		t.Error("the release publishes nothing called timepulse-mobile_...")
	}
}

// The newest release says which Android app it carries and which contract with
// an installation that app speaks. An installation of an earlier release passes
// the app on to its phones only where that contract is one it serves. A release
// that does not say speaks contract zero, which no installation serves, and so
// does every release from before the contract was named. Only the marker counts:
// a release note that happens to mention a contract is prose.
func TestTheNewestReleaseNamesItsAppAndTheContractItSpeaks(t *testing.T) {
	newest := func(notes string) Release {
		t.Helper()

		feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v9.9.9",
				"body":     notes,
				"html_url": "https://example.invalid/release",
				"assets": []map[string]string{
					{"name": "timepulse-mobile_v9.9.8.apk", "browser_download_url": "https://example.invalid/older.apk"},
					{"name": androidAssetName("v9.9.9"), "browser_download_url": "https://example.invalid/app.apk"},
				},
			})
		}))
		defer feed.Close()

		release, err := New(feed.URL, "").Latest(context.Background())
		if err != nil {
			t.Fatalf("reading the feed: %v", err)
		}

		return release
	}

	release := newest("## What changed\n\n- a thing\n\n<!-- timepulse-app-contract: 3 -->\n")

	if got := release.AppDownload(); got != "https://example.invalid/app.apk" {
		t.Errorf("the release's app is %q, want the file named after its own version", got)
	}

	if got := release.AppContract(); got != 3 {
		t.Errorf("the release says its app speaks contract 3, and was read as %d", got)
	}

	for _, notes := range []string{"## What changed\n\n- a thing\n", "The app now speaks contract 3."} {
		if got := newest(notes).AppContract(); got != 0 {
			t.Errorf("a release saying no contract in the marker was read as speaking %d: %q", got, notes)
		}
	}
}

// The release writes its app's contract into its text in the form Latest reads
// it, taken from the app's own declaration, so the two cannot drift apart. The
// workflow reads the line the app declares the contract on; a contract written
// into the workflow a second time is how a release would come to claim one its
// app does not speak.
func TestTheReleaseWritesTheContractItsAppSpeaks(t *testing.T) {
	raw, err := os.ReadFile("../../../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("reading the release workflow: %v", err)
	}

	workflow := string(raw)
	marker := "<!-- timepulse-app-contract: ${{ steps.contract.outputs.level }} -->"

	if !strings.Contains(workflow, marker) {
		t.Errorf("the release text does not carry %s, so no installation can tell which contract the app speaks", marker)
	}

	if got := appContractIn(strings.ReplaceAll(marker, "${{ steps.contract.outputs.level }}", "7")); got != 7 {
		t.Errorf("the marker the release writes, carrying 7, is read as %d", got)
	}

	app, err := os.ReadFile("../../interface/mobile/src/main/kotlin/io/github/dennisdko/timepulse/Installation.kt")
	if err != nil {
		t.Fatalf("reading the app's declaration: %v", err)
	}

	declaration := regexp.MustCompile(`(?m)^const val appContract = ([0-9]+)$`).FindSubmatch(app)
	if declaration == nil {
		t.Fatal("Installation.kt does not declare appContract on a line of its own, which is the line the release reads")
	}

	if !strings.Contains(workflow, `s/^const val appContract = \([0-9][0-9]*\)$/\1/p`) {
		t.Error("the release does not read the contract from the app's declaration")
	}
}
