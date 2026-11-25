package web_test

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/dennis-dko/timepulse/internal/domain/model"
)

// The project form's figures stop where the server does.
//
// A budget and a rate are numbers somebody types, and the server refuses one past
// its bound by the field's name. A form that lets a larger one be typed has
// somebody press Save to be told what the box could have said; one that stops
// short makes a figure the server takes impossible to type.
func TestTheProjectFormStopsWhereTheServerDoes(t *testing.T) {
	html := asset(t, "/")

	for field, bound := range map[string]int{
		"budgetHours": model.MaxBudgetHours,
		"hourlyRate":  model.MaxHourlyRate,
	} {
		match := regexp.MustCompile(`name="` + field + `"[^>]*max="(\d+)"`).FindStringSubmatch(html)
		if match == nil {
			t.Errorf("the %s field has no max, so the form offers what the server refuses", field)

			continue
		}

		if written, _ := strconv.Atoi(match[1]); written != bound {
			t.Errorf("the %s field stops at %d and the server at %d", field, written, bound)
		}
	}

	// A currency is picked rather than typed, among the codes CURRENCIES in app.js
	// lists: each three capital letters, as the server takes them, none twice, and
	// the server's own default among them and named as the picker's.
	if !regexp.MustCompile(`<select name="currency"`).MatchString(html) {
		t.Error("the currency is typed rather than picked")
	}

	script := asset(t, "/app.js")

	block := regexp.MustCompile(`(?s)const CURRENCIES = \[(.*?)\];`).FindStringSubmatch(script)
	if block == nil {
		t.Fatal("app.js lists no CURRENCIES for the picker")
	}

	seen := map[string]bool{}

	for _, code := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(block[1], -1) {
		if !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(code[1]) || seen[code[1]] {
			t.Errorf("the picker offers %q, which is not a code the server takes or is offered twice", code[1])
		}

		seen[code[1]] = true
	}

	if !seen[model.DefaultCurrency] {
		t.Errorf("the picker does not offer %s, the server's default", model.DefaultCurrency)
	}

	if !regexp.MustCompile(`const DEFAULT_CURRENCY = '` + model.DefaultCurrency + `';`).MatchString(script) {
		t.Errorf("the picker's default is not %s, the server's", model.DefaultCurrency)
	}
}
