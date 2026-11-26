package mobile

import (
	"regexp"
	"strings"
	"testing"
)

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
