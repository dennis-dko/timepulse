package rest

import (
	"strconv"
	"testing"

	"github.com/dennis-dko/timepulse/internal/support/apperror"
)

// TestAnAppIsServedOnlyInsideTheContractsThisInstallationServes holds the lock
// the maintainer asked for on 2026-11-26: an app this installation cannot serve
// may not even sign in. It is refused before a session exists, with a reason
// the app can translate, and it is told which way round it is, because the
// remedy differs: an app too old is updated on the phone, an installation too
// old on the server. A request saying nothing is served as before, because a
// script and every app from before the contract say nothing.
func TestAnAppIsServedOnlyInsideTheContractsThisInstallationServes(t *testing.T) {
	t.Parallel()

	for _, header := range []string{"", strconv.Itoa(OldestAppContract), strconv.Itoa(AppContract)} {
		if err := refuseAppContract(header); err != nil {
			t.Errorf("an app saying %q is refused: %v", header, err)
		}
	}

	for header, code := range map[string]string{
		strconv.Itoa(OldestAppContract - 1): "appTooOld",
		strconv.Itoa(AppContract + 1):       "appTooNew",
	} {
		detail, ok := apperror.Detail(refuseAppContract(header))
		if !ok || detail.Kind != apperror.KindConflict || detail.Code != code {
			t.Errorf("an app saying %q is answered %+v, want a conflict named %s", header, detail, code)
		}
	}

	for _, header := range []string{"one", "-1", "1.5"} {
		if detail, ok := apperror.Detail(refuseAppContract(header)); !ok || detail.Kind != apperror.KindInvalid {
			t.Errorf("an app saying %q is answered %+v, want it refused as malformed", header, detail)
		}
	}

	if !servesAppContract(AppContract) || servesAppContract(0) || servesAppContract(AppContract+1) {
		t.Error("servesAppContract does not answer the range this installation serves")
	}
}
