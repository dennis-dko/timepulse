package rest

import (
	"strconv"
	"strings"

	"github.com/dennis-dko/timepulse/internal/support/apperror"
)

// AppContract is the newest contract with the Android app this installation
// speaks, and OldestAppContract the oldest it still serves. The contract is what
// the app asks the installation itself, outside the web interface it shows: a
// session for a token, the account, the stopwatch, the notices and the app on
// offer. The app built from the same commit speaks AppContract, which a test in
// internal/interface/mobile holds to the app's own declaration.
//
// Raise AppContract together with the app's when one of those requests changes
// in a way an installation of the previous contract would answer wrongly. Raise
// OldestAppContract only when this installation stops answering an older app
// correctly. Until either moves, no app and no installation is ever refused.
const (
	AppContract       = 1
	OldestAppContract = 1
)

// appContractHeader is where the app says which contract it speaks, on the
// request that signs it in.
const appContractHeader = "X-TimePulse-App-Contract"

// AppContracts is the range of contracts this installation serves, answered to
// the app when it signs in so that it can refuse an installation that cannot
// serve it.
type AppContracts struct {
	Oldest int `json:"oldest"`
	Newest int `json:"newest"`
}

// servesAppContract reports whether an app speaking level is served here. A
// release that names no contract reads as zero, which is never served, so its
// app is never passed on to a phone on the strength of being newer.
func servesAppContract(level int) bool {
	return level >= OldestAppContract && level <= AppContract
}

// refuseAppContract is why an app saying header is not served here, or nil.
//
// It tells the two ways round apart, because the remedy differs: an app too old
// is updated on the phone, an installation too old on the server. A request
// saying nothing is served as before, because a script and every app from
// before the contract say nothing, and both work today.
func refuseAppContract(header string) error {
	if header == "" {
		return nil
	}

	level, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || level < 0 {
		return apperror.Invalidf("%s must be a whole number", appContractHeader)
	}

	switch {
	case level < OldestAppContract:
		return apperror.Conflictf("this app is too old for this installation; update the app").WithCode("appTooOld")
	case level > AppContract:
		return apperror.Conflictf("this installation is too old for this app; update the installation").WithCode("appTooNew")
	}

	return nil
}
