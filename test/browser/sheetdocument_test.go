//go:build browser

package browser

import (
	"testing"
)

// TestTheCardsThatLeftOnlyAsAWorkbookLeaveAsAPDFToo presses the PDF on the time
// entries' card and on the cards of the projects, the accounts and the roles, and
// reads the document each one asked for.
//
// Those four were the cards whose content could leave only as a workbook, on
// screens beside others offering both. The time entries' document reads every
// page the filters under Entries let through, as the workbook beside it does, so
// more entries are booked here than one page of that list holds. The tables'
// documents carry their table as its screen shows it, every page of it - and an
// account's role as the one chosen, not every role its dropdown offers.
func TestTheCardsThatLeftOnlyAsAWorkbookLeaveAsAPDFToo(t *testing.T) {
	t.Parallel()

	t.Run("worker", func(t *testing.T) {
		t.Parallel()

		p := open(t)
		p.readyWorker()
		p.catchDocuments()

		var booked bool

		p.awaitJSON(`(async () => {
			const project = await api('/projects', { method: 'POST', body: JSON.stringify({ name: 'Roof', startDate: todayISO() }) });
			for (let i = 1; i <= 12; i++) {
				await api('/timesheets', { method: 'POST', body: JSON.stringify({ projectId: project.id, date: todayISO(), durationHours: 0.5, description: 'part ' + i }) });
			}
			await refreshAll();
			return true;
		})()`, &booked)

		p.run("open the time entries' export", p.click(`.tab[data-view="timesheets"]`),
			p.click(`#timesheets-tabs [data-card-tab="entries"]`))
		p.waitShown("#wb-pdf")

		before := p.documentsAsked()
		p.run("export the entries", p.click("#wb-pdf"))

		entries := p.documentAsked(before, "#wb-pdf")
		table := entries.table(t, 0)

		if len(table.Rows) != 12 {
			t.Errorf("the entries' document carries %d rows for the 12 entries booked: a page "+
				"of the list is not the whole of what its filters show", len(table.Rows))
		}

		var expected struct {
			Title string `json:"title"`
			Total string `json:"total"`
		}

		p.evalJSON(`JSON.stringify({ title: t('nav.timesheets', 'Time entries'), total: fmtNumber(6) })`, &expected)

		if entries.Title != expected.Title {
			t.Errorf("the entries' document is titled %q, want %q", entries.Title, expected.Title)
		}

		if len(entries.Summary) != 1 || entries.Summary[0].Value != expected.Total {
			t.Errorf("the entries' document sums up as %+v, want the %s hours booked", entries.Summary, expected.Total)
		}

		p.exportsItsTable(t, "projects", "#table-projects")
	})

	t.Run("administrator", func(t *testing.T) {
		t.Parallel()

		p := open(t)
		p.readyAdmin()

		// An account other than the built-in one, whose row draws its role as the
		// dropdown it is changed from - the built-in account's is text.
		p.createOrdinaryAccount(t, "ada@example.com", "a-long-password-1")

		var reloaded bool

		p.awaitJSON(`(async () => { await refreshAll(); return true; })()`, &reloaded)
		p.catchDocuments()

		users := p.exportsItsTable(t, "users", "#table-users")

		var chosen struct {
			Heading string   `json:"heading"`
			Roles   []string `json:"roles"`
		}

		p.evalJSON(`JSON.stringify((() => {
			const headings = [...document.querySelectorAll('#table-users thead th')];
			const at = headings.findIndex((heading) => heading.dataset.i18n === 'field.role');
			return {
				heading: headings[at].textContent.trim(),
				roles: [...document.querySelectorAll('#table-users tbody tr')].map((row) => {
					const cell = row.cells[at];
					return cell.querySelector('select')?.selectedOptions[0]?.textContent.trim() ?? cell.textContent.trim();
				}),
			};
		})())`, &chosen)

		column := -1

		for i, heading := range users.Columns {
			if heading == chosen.Heading {
				column = i
			}
		}

		if column < 0 {
			t.Fatalf("the accounts' document has no column %q among %q", chosen.Heading, users.Columns)
		}

		if len(chosen.Roles) < 2 {
			t.Fatalf("the accounts' table holds %d rows, so no row with a dropdown is compared", len(chosen.Roles))
		}

		for i, row := range users.Rows {
			if i < len(chosen.Roles) && row[column] != chosen.Roles[i] {
				t.Errorf("row %d of the accounts' document says the role is %q, and the row on screen %q",
					i+1, row[column], chosen.Roles[i])
			}
		}

		p.exportsItsTable(t, "roles", "#table-roles")
	})
}

// exportsItsTable opens a table's screen, presses the PDF on the card beside it,
// and holds what it sent to the table as the screen shows it. It returns that
// table for a case to read further.
func (p *page) exportsItsTable(t *testing.T, view, table string) askedTable {
	t.Helper()

	card := `#view-` + view + ` .card:has(input[type="file"])`

	p.run("open the "+view, p.click(`.tab[data-view="`+view+`"]`))
	p.waitShown(card)

	before := p.documentsAsked()
	p.run("export the "+view, p.click(card+` [data-i18n="action.exportPdf"]`))

	asked := p.documentAsked(before, "the "+view+"' PDF").table(t, 0)

	var shown struct {
		Columns []string `json:"columns"`
		Rows    int      `json:"rows"`
	}

	p.evalJSON(`JSON.stringify((() => {
		const figures = tableAsFigures(document.querySelector('`+table+`'));
		return { columns: figures.columns, rows: figures.rows.length };
	})())`, &shown)

	if shown.Rows == 0 {
		t.Fatalf("%s shows no rows, so its document is compared with nothing", table)
	}

	if len(asked.Rows) != shown.Rows || len(asked.Columns) != len(shown.Columns) {
		t.Errorf("the %s' document carries %d rows in %d columns, the table on screen %d in %d",
			view, len(asked.Rows), len(asked.Columns), shown.Rows, len(shown.Columns))
	}

	return asked
}
