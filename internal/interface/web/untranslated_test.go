package web_test

import (
	"regexp"
	"strings"
	"testing"
)

// literalPlaceholders are placeholders that show a value the field takes as it
// is written, rather than words: translating them would make them wrong.
var literalPlaceholders = map[string]string{
	"https://":                           "the start of an address",
	"disable":                            "PostgreSQL's sslmode, as the connection string spells it",
	"dc=example,dc=com":                  "a directory's base, in LDAP's own notation",
	"entryUUID":                          "an LDAP attribute's name",
	"https://id.example.com/realms/acme": "a provider's address",
	"Keycloak":                           "a provider's name",
	"jaeger:4317":                        "a collector's host and port",
}

// TestEveryWordForAScreenReaderIsTranslated holds the words the markup gives an
// element without showing them - its aria-label, its title, its alt - to a key
// of their own, as the visible words are held by data-i18n.
//
// A missing key fails silently, and here it fails where nobody looking at the
// screen would notice: the log viewer's row of levels told a German screen
// reader "Log levels" in English. A placeholder may show a value the field
// takes literally instead, and those are named with the reason.
func TestEveryWordForAScreenReaderIsTranslated(t *testing.T) {
	markup := withoutComments(asset(t, "/"))

	tags := regexp.MustCompile(`<[a-z][a-z0-9-]*(\s[^>]*)>`).FindAllStringSubmatch(markup, -1)
	if len(tags) < 100 {
		t.Fatalf("found %d tags in the markup, so this reads almost nothing", len(tags))
	}

	for _, tag := range tags {
		attrs := tag[1]

		for _, words := range []struct{ attribute, key string }{
			{"aria-label", "data-i18n-aria"},
			{"title", "data-i18n-title"},
			{"alt", "data-i18n-alt"},
			{"placeholder", "data-i18n-placeholder"},
		} {
			value := regexp.MustCompile(`\s` + words.attribute + `="([^"]*)"`).FindStringSubmatch(attrs)
			if value == nil || !regexp.MustCompile(`[A-Za-z]{2,}`).MatchString(value[1]) {
				continue
			}

			if strings.Contains(attrs, words.key+"=") {
				continue
			}

			if _, literal := literalPlaceholders[value[1]]; literal && words.attribute == "placeholder" {
				continue
			}

			t.Errorf("%s=%q has no %s, so it is read in English whatever the page is read in",
				words.attribute, value[1], words.key)
		}
	}
}

// TestNoScriptWritesWordsPastTheDictionary holds every word the script writes
// on screen to a lookup: a title, a text, a placeholder or an aria-label given a
// literal that begins with a word is English to every reader.
//
// The log viewer gave each line a title of "trace " and the trace's id, English
// on a German screen, beside lines whose every other word was looked up.
func TestNoScriptWritesWordsPastTheDictionary(t *testing.T) {
	js := withoutLineComments(asset(t, "/app.js"))

	written := regexp.MustCompile(
		`\.(?:title|textContent|innerText|placeholder)\s*=\s*['` + "`" + `]([A-Za-z]{3,}[^'` + "`" + `]*)` +
			`|setAttribute\('(?:aria-label|title|placeholder)',\s*['` + "`" + `]([A-Za-z]{3,}[^'` + "`" + `]*)`)

	for _, line := range strings.Split(js, "\n") {
		found := written.FindStringSubmatch(line)
		if found == nil || strings.Contains(line, "t(") {
			continue
		}

		t.Errorf("the script writes %q on screen without looking it up: %s",
			found[1]+found[2], strings.TrimSpace(line))
	}
}
