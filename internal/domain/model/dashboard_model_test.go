package model_test

import (
	"strings"
	"testing"

	"github.com/dennis-dko/timepulse/internal/domain/model"
)

// A tile its owner put away is kept in the arrangement, in its place, with a
// "-" before its name - so it comes back where it stood - and a tile is still
// one tile: named twice, once put away and once not, is refused like any other
// name twice.
func TestAnArrangementKeepsAPutAwayTileInItsPlace(t *testing.T) {
	for order, want := range map[string]bool{
		"week,-leave,balance": true,
		"-week":               true,
		"week,-week":          false,
		"--week":              false,
		"-":                   false,
		"week,-Leave":         false,
	} {
		if got := model.IsValidDashboard(strings.Split(order, ",")); got != want {
			t.Errorf("%q kept as an arrangement: %v, want %v", order, got, want)
		}
	}
}
