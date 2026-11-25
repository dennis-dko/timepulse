package web_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dennis-dko/timepulse/internal/domain/model"
)

// TestThePickerOffersTheCurrenciesTheServerTakes holds the project form's
// currencies to the server's: one list, offered where a currency is chosen and
// enforced where it is stored, whoever sends it.
//
// The form picked from ISO 4217 while the server took any three capital
// letters, so a script - or a slip like EUO in an older client - stored a
// currency nobody can be paid in.
func TestThePickerOffersTheCurrenciesTheServerTakes(t *testing.T) {
	block := regexp.MustCompile(`(?s)const CURRENCIES = \[(.*?)\];`).FindStringSubmatch(asset(t, "/app.js"))
	if block == nil {
		t.Fatal("app.js declares no CURRENCIES, so the picker's list is not where this looks")
	}

	var offered []string

	for _, code := range regexp.MustCompile(`'([A-Z]{3})'`).FindAllStringSubmatch(block[1], -1) {
		offered = append(offered, code[1])
	}

	taken := model.Currencies()

	if len(offered) < 100 || strings.Join(offered, " ") != strings.Join(taken, " ") {
		t.Errorf("the picker offers %d currencies and the server takes %d, or not the same ones:\n  picker: %v\n  server: %v",
			len(offered), len(taken), offered, taken)
	}

	if !model.IsCurrency(model.DefaultCurrency) {
		t.Errorf("the default currency %s is not one the server takes", model.DefaultCurrency)
	}

	// Found by a binary search, which finds nothing in a list out of order.
	for _, code := range taken {
		if !model.IsCurrency(code) {
			t.Errorf("%s is in the server's list and IsCurrency does not find it: the list is out of order", code)
		}
	}
}
