package rest

import (
	"cmp"

	"gofr.dev/pkg/gofr"

	"github.com/dennis-dko/timepulse/internal/infrastructure/selfupdate"
	"github.com/dennis-dko/timepulse/internal/support/qrcode"
)

// AndroidResponse is where the Android app this installation offers is
// downloaded: the newest one that fits it, see appFor.
type AndroidResponse struct {
	// Release is the page of the release the app comes from.
	Release string `json:"release"`

	// Download is the app itself, where this installation runs a release.
	Download string `json:"download,omitempty"`

	// Version is the release the app at Download is, beside it and only where it
	// is: what the app compares its own with, at every start, to offer itself
	// anew when this installation has moved on.
	Version string `json:"version,omitempty"`

	// QR is a code of Download, or of Release where there is no file to name.
	QR *string `json:"qr,omitempty"`
}

// Android handles GET /api/v1/me/android: where the Android app this
// installation offers is downloaded - the newest that fits it, see appFor - with
// a code a phone's camera opens, so that the card offering the app leads a phone
// straight to the file. The app asks the same at every start. Drawn
// here because the page has no QR encoder of its own, which is also why the
// address is worked out here rather than beside the code in the page.
func (h *UpdateHandler) Android(c *gofr.Context) (any, error) {
	if _, err := h.authz.Principal(c); err != nil {
		return nil, err
	}

	answer := androidAnswer(h.version)

	// Only where the installation may call out, and from what the update card
	// last learned where it can: a feed not answering leaves the phone with this
	// installation's own app, which fits.
	if h.enabled && answer.Download != "" {
		latest, err := h.latest(c)
		if err != nil {
			c.Logger.Debugf("the release feed did not answer, so the Android app on offer is this installation's own: %v", err)
		}

		answer = appFor(h.version, latest)
	}

	// Not essential: without a code, the card has its link alone.
	if code, err := qrcode.SVGDataURI(cmp.Or(answer.Download, answer.Release)); err == nil {
		answer.QR = &code
	} else {
		c.Logger.Errorf("could not render the Android app's QR code: %v", err)
	}

	return answer, nil
}

// appFor is the app an installation running version offers its phones, given
// latest, the newest release it knows of.
//
// That release's app wherever it is newer than this installation's own and
// speaks a contract served here, so that a phone gets the newest app that fits
// before the installation itself is updated, as the maintainer chose on
// 2026-11-26. Otherwise this installation's own release's app, which always fits
// because it was built with it. An installation that is no release offers what
// it always did, since it has no version to compare with.
func appFor(version string, latest selfupdate.Release) AndroidResponse {
	own := androidAnswer(version)

	if own.Download == "" || latest.AppDownload() == "" ||
		!selfupdate.Newer(version, latest.Version) || !servesAppContract(latest.AppContract()) {
		return own
	}

	return AndroidResponse{Release: latest.URL, Download: latest.AppDownload(), Version: latest.Version}
}

// androidAnswer is where the app that belongs to version is downloaded, and
// which release that app is.
func androidAnswer(version string) AndroidResponse {
	page, download := selfupdate.AndroidApp(version)
	answer := AndroidResponse{Release: page, Download: download}

	if download != "" {
		answer.Version = version
	}

	return answer
}
