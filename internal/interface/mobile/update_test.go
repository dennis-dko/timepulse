package mobile

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/dennis-dko/timepulse/internal/interface/api/v1/rest"
)

// TestTheAppSpeaksTheContractItsInstallationSpeaks holds the two halves of the
// contract together. The app built from a commit speaks the newest contract the
// installation built from it serves, so a phone and a server of one release
// always fit. It says so under the header the installation reads. And where an
// installation from before contracts cannot refuse the app itself, the app
// refuses it in that installation's own words, which both must spell alike.
func TestTheAppSpeaksTheContractItsInstallationSpeaks(t *testing.T) {
	code, _ := lexed(source(t, "Installation.kt"))

	declared := regexp.MustCompile(`(?m)^const val appContract = ([0-9]+)$`).FindStringSubmatch(code)
	if declared == nil {
		t.Fatal("Installation.kt declares no appContract")
	}

	if declared[1] != strconv.Itoa(rest.AppContract) {
		t.Errorf("the app speaks contract %s and the installation of the same commit %d", declared[1], rest.AppContract)
	}

	server := read(t, "../api/v1/rest/appcontract.go")

	header := regexp.MustCompile(`appContractHeader = "([^"]+)"`).FindStringSubmatch(server)
	if header == nil || !strings.Contains(code, `setRequestProperty("`+header[1]+`", appContract.toString())`) {
		t.Errorf("the app does not say its contract under the header the installation reads, %v", header)
	}

	theirs := regexp.MustCompile(`Conflictf\("([^"]+)"\)\.WithCode\("appTooNew"\)`).FindStringSubmatch(server)
	mine := regexp.MustCompile(`text\(\s*"mobile\.installationTooOld",\s*"([^"]+)"\)`).FindStringSubmatch(source(t, "Refusal.kt"))

	if theirs == nil || mine == nil || theirs[1] != mine[1] {
		t.Errorf("the app refuses an installation too old for it as %v, and an installation refuses such an app as %v", mine, theirs)
	}

	var german map[string]string
	if err := json.Unmarshal([]byte(read(t, "../../support/i18n/locale/de.json")), &german); err != nil {
		t.Fatal(err)
	}

	if german["mobile.installationTooOld"] == "" || german["mobile.installationTooOld"] != german["err.appTooNew"] {
		t.Errorf("in German the app says %q where an installation says %q",
			german["mobile.installationTooOld"], german["err.appTooNew"])
	}
}

// TestTheAppReplacesItselfWithoutAskingWhereAndroidLetsIt holds the two halves
// of an update without a question together. From Android 12 on, Android lets an
// app replace itself without asking whoever holds the phone, but only where the
// session says no question is needed and the manifest declares
// UPDATE_PACKAGES_WITHOUT_USER_ACTION. Either missing alone brings the question
// back, and nothing else would notice. Android ends the app to replace it and
// lets no app in the background open a screen again, so the app that replaced
// it says so in a notification. That needs a receiver hearing
// MY_PACKAGE_REPLACED that no other app may call.
func TestTheAppReplacesItselfWithoutAskingWhereAndroidLetsIt(t *testing.T) {
	code, _ := lexed(source(t, "Update.kt"))

	if !regexp.MustCompile(`setRequireUserAction\(\s*PackageInstaller\.SessionParams\.USER_ACTION_NOT_REQUIRED\s*\)`).MatchString(code) {
		t.Error("the installation session does not say that no question is needed, so Android asks to confirm every update")
	}

	manifest := read(t, "src/main/AndroidManifest.xml")

	if !strings.Contains(manifest, `android:name="android.permission.UPDATE_PACKAGES_WITHOUT_USER_ACTION"`) {
		t.Error("the app does not declare UPDATE_PACKAGES_WITHOUT_USER_ACTION, so Android asks to confirm every update whatever the session says")
	}

	receiver := regexp.MustCompile(`<receiver\s+android:name="\.ReplacedReceiver"\s+android:exported="false">\s*` +
		`<intent-filter>\s*<action\s+android:name="android\.intent\.action\.MY_PACKAGE_REPLACED"\s*/>`)
	if !receiver.MatchString(manifest) {
		t.Error("no receiver that other apps cannot call hears MY_PACKAGE_REPLACED, so nothing says the app was " +
			"updated once Android has closed it for that")
	}

	if !regexp.MustCompile(`\bclass\s+ReplacedReceiver\s*:\s*BroadcastReceiver\(\)`).MatchString(code) {
		t.Error("Update.kt declares no ReplacedReceiver, the receiver the manifest names for MY_PACKAGE_REPLACED")
	}
}
