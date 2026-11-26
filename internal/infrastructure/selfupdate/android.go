package selfupdate

import (
	"fmt"
	"regexp"
	"strconv"
)

// Releases is where this project's releases are shown, each carrying the
// Android app built from the same commit.
const Releases = "https://github.com/dennis-dko/timepulse/releases"

// AndroidApp is where the Android app that belongs to version is downloaded:
// the page of its release, and the APK that release carries. The app belongs to
// the release it was built with, so a release's version is sent to its own; any
// other - "dev", or a source tree some commits past its last tag - has no
// release of its own and is sent to the newest one's page with no file named,
// because the newest one's file carries a version this one cannot know.
func AndroidApp(version string) (page, download string) {
	if !isRelease(version) {
		return Releases + "/latest", ""
	}

	return Releases + "/tag/" + version, Releases + "/download/" + version + "/" + androidAssetName(version)
}

// androidAssetName is what the Android app of version is called in its release.
func androidAssetName(version string) string {
	return "timepulse-mobile_" + version + ".apk"
}

// appContractIn is the contract a release's text says its Android app speaks,
// zero where it says none.
//
// The release workflow writes it as an HTML comment, which GitHub does not show
// on the release page, because the text is also what people read. It is read
// from the text rather than from a file of its own so that an installation
// learns it from the answer it already asks for, with no second request.
func appContractIn(notes string) int {
	found := appContractMarker.FindStringSubmatch(notes)
	if found == nil {
		return 0
	}

	level, err := strconv.Atoi(found[1])
	if err != nil {
		return 0
	}

	return level
}

// appContractMarker is the comment appContractIn reads, as release.yml writes it.
var appContractMarker = regexp.MustCompile(`<!-- timepulse-app-contract: ([0-9]{1,6}) -->`)

// isRelease says whether version is the tag a release is named by and nothing
// more: parseVersion also reads what a build from a source tree says about how
// far it has moved on from its last tag, which names no release.
func isRelease(version string) bool {
	numbers, ok := parseVersion(version)

	return ok && version == fmt.Sprintf("v%d.%d.%d", numbers[0], numbers[1], numbers[2])
}
