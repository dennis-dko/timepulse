//go:build integration

package integration

import (
	"net/http"
	"testing"
)

// The settings a configuration file supplies are told to the screen as values.
//
// The screens that administer them showed a field left to the file as empty -
// the limits with the file's value as a placeholder, the collector likewise, the
// directory's schedule not at all - so an installation configured through its
// file looked unconfigured, and the schedule card said a reconciliation ran only
// when its button was pressed while the file's schedule ran every night.
func TestTheFilesSettingsAreToldAsValues(t *testing.T) {
	t.Parallel()

	a := start(t, "LDAP_SYNC_SCHEDULE=0 3 * * *", "TRACER_URL=collector.internal:4317", "TRACER_RATIO=0.25")
	admin := a.signInAsAdmin("a-much-better-password")

	var ldap struct {
		SyncSchedule     string `json:"syncSchedule"`
		FileSyncSchedule string `json:"fileSyncSchedule"`
	}

	admin.must(admin.api(http.MethodGet, "/settings/ldap", nil), http.StatusOK).Data(t, &ldap)

	if ldap.SyncSchedule != "" || ldap.FileSyncSchedule != "0 3 * * *" {
		t.Errorf("the directory settings say %q is saved and %q is the file's, want nothing saved and the file's schedule",
			ldap.SyncSchedule, ldap.FileSyncSchedule)
	}

	var telemetry struct {
		File struct {
			LogLevel    string  `json:"logLevel"`
			TracerURL   string  `json:"tracerUrl"`
			TracerRatio float64 `json:"tracerRatio"`
		} `json:"file"`
	}

	admin.must(admin.api(http.MethodGet, "/settings/telemetry", nil), http.StatusOK).Data(t, &telemetry)

	if telemetry.File.TracerURL != "collector.internal:4317" || telemetry.File.TracerRatio != 0.25 || telemetry.File.LogLevel == "" {
		t.Errorf("the telemetry settings give the file's as %+v, want its collector, its share and a log level",
			telemetry.File)
	}
}
