//go:build browser

package browser

import (
	"testing"
)

// TestAnImportButtonIsDrawnAsTheExportButtonBesideIt compares the import with the
// export beside it, on the time entries' card and on the cards of the three
// tables, property by property.
//
// It was a label dressed as a button: a box inside a border, with the corners and
// the ground the controls had before they became pills, standing in a row of
// pills - so the one way into a table looked like nothing else on its row. It is
// a button.secondary now, opening the file input out of sight beside it.
func TestAnImportButtonIsDrawnAsTheExportButtonBesideIt(t *testing.T) {
	t.Parallel()

	type card struct{ view, tab, scope string }

	// Two accounts, because no one account sees all four: the administrator keeps
	// no time of their own, and the worker administers nothing.
	for _, who := range []struct {
		name  string
		ready func(*page)
		cards []card
	}{
		{"worker", (*page).readyWorker, []card{
			{"timesheets", `#timesheets-tabs [data-card-tab="sheet"]`, "#workbook-card"},
			{"projects", "", `#view-projects .card:has(input[type="file"])`},
		}},
		{"administrator", (*page).readyAdmin, []card{
			{"users", "", `#view-users .card:has(input[type="file"])`},
			{"roles", "", `#view-roles .card:has(input[type="file"])`},
		}},
	} {
		t.Run(who.name, func(t *testing.T) {
			t.Parallel()

			p := open(t)
			who.ready(p)

			for _, card := range who.cards {
				p.run("open "+card.view, p.click(`.tab[data-view="`+card.view+`"]`))

				if card.tab != "" {
					p.run("open its tab", p.click(card.tab))
				}

				p.waitShown(card.scope)

				var drawn struct {
					Tag    string            `json:"tag"`
					Import map[string]string `json:"import"`
					Export map[string]string `json:"export"`
				}

				p.evalJSON(`JSON.stringify((() => {
					const card = document.querySelector('`+card.scope+`');
					const importer = card.querySelector('[data-i18n="wb.importFrom"]');
					const exporter = card.querySelector('[data-i18n="wb.export"]');
					const look = (control) => {
						const style = getComputedStyle(control);
						return Object.fromEntries(['height', 'borderTopLeftRadius', 'borderTopWidth',
							'backgroundColor', 'color', 'fontSize', 'fontWeight', 'paddingLeft',
							'paddingRight'].map((name) => [name, style[name]]));
					};
					return { tag: importer.tagName, import: look(importer), export: look(exporter) };
				})())`, &drawn)

				if drawn.Tag != "BUTTON" {
					t.Errorf("%s: the import is a %s rather than a button", card.view, drawn.Tag)
				}

				for name, export := range drawn.Export {
					if got := drawn.Import[name]; got != export {
						t.Errorf("%s: the import's %s is %s, the export's beside it %s", card.view, name, got, export)
					}
				}
			}
		})
	}
}
