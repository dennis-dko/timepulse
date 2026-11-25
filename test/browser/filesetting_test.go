//go:build browser

package browser

import (
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// What a configuration file supplies stands in the field as the value in force,
// as the database connection's does - not as an empty field with a placeholder,
// which read as "not configured" - and saving it unchanged keeps it following
// the file, so the file stays the place to change it.
func TestTheFilesSettingsAreShownAsTheirValues(t *testing.T) {
	t.Parallel()

	p := openWith(t, "SESSION_LIFETIME=8h", "MAX_DAILY_HOURS=10", "LDAP_SYNC_SCHEDULE=0 3 * * *",
		"TRACER_URL=collector.internal:4317", "TRACER_RATIO=0.25")
	p.readyAdmin()

	p.run("open Settings", p.click(`.tab[data-view="admin"]`))
	p.showTabOf("#form-operational")
	p.atRest()

	for field, want := range map[string]string{"sessionLifetimeHours": "8", "maxDailyHours": "10"} {
		if got := p.value(`#form-operational input[name="` + field + `"]`); got != want {
			t.Errorf("the limit %s shows %q, want the file's %s as its value", field, got, want)
		}
	}

	stored := func() map[string]any {
		var configured map[string]any

		p.awaitJSON(`api('/settings/operational').then((s) => Object.fromEntries(
			Object.entries(s.configured ?? {}).filter(([, value]) => value !== null && value !== undefined)))`, &configured)

		return configured
	}

	p.run("save the limits as they stand", p.click(`#form-operational button[type="submit"]`))
	p.atRest()

	if saved := stored(); len(saved) != 0 {
		t.Errorf("saving the file's values unchanged stored %v, which would stop them following the file", saved)
	}

	p.run("lower one limit", chromedp.SetValue(`#form-operational input[name="maxDailyHours"]`, "9", chromedp.ByQuery),
		p.click(`#form-operational button[type="submit"]`))
	p.atRest()

	if saved := stored(); len(saved) != 1 || saved["maxDailyHours"] != float64(9) {
		t.Errorf("lowering one limit stored %v, want that one alone", saved)
	}

	// The collector and its share, and what the log level follows.
	p.showTabOf("#form-telemetry")

	if got := p.value(`#form-telemetry input[name="tracerUrl"]`); got != "collector.internal:4317" {
		t.Errorf("the collector shows %q, want the file's as its value", got)
	}

	if got := p.value(`#form-telemetry input[name="tracerRatio"]`); got != "0.25" {
		t.Errorf("the share of traces shows %q, want the file's 0.25", got)
	}

	var fileLevel string

	p.awaitJSON(`api('/settings/telemetry').then((s) => s.file.logLevel)`, &fileLevel)

	if level := p.text(`#form-telemetry select[name="logLevel"] option:checked`); fileLevel == "" || !strings.Contains(level, fileLevel) {
		t.Errorf("the log level reads %q, which does not say what the file sets (%q)", level, fileLevel)
	}

	p.run("save the telemetry as it stands", p.click(`#form-telemetry button[type="submit"]`))
	p.atRest()

	var configured map[string]any

	p.awaitJSON(`api('/settings/telemetry').then((s) => Object.fromEntries(
		Object.entries(s.configured ?? {}).filter(([, value]) => value !== null && value !== undefined)))`, &configured)

	if len(configured) != 0 {
		t.Errorf("saving the telemetry unchanged stored %v, which would stop it following the file", configured)
	}

	// And the directory's schedule, which the card said did not exist.
	p.showTabOf("#form-sync-schedule")

	if got := p.value(`#form-sync-schedule input[name="syncSchedule"]`); got != "0 3 * * *" {
		t.Errorf("the schedule shows %q, want the file's", got)
	}

	if said := p.text("#sync-schedule-active"); !strings.Contains(said, "0 3 * * *") {
		t.Errorf("the card says %q about a reconciliation the file schedules every night", said)
	}

	p.run("save the schedule as it stands", p.click(`#form-sync-schedule button[type="submit"]`))
	p.atRest()

	var saved string

	p.awaitJSON(`api('/settings/ldap').then((s) => s.syncSchedule)`, &saved)

	if saved != "" {
		t.Errorf("saving the file's schedule unchanged stored %q, which would go on running after the file dropped it", saved)
	}
}
