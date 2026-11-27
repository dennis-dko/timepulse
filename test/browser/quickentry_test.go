//go:build browser

package browser

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// A booking written as one line is read by a fixed way of writing, so the same
// line always means the same booking: hours with their unit or as the time
// worked, a day as a word or a date, a project by its whole name or by @ and
// its beginning - among the ones that take bookings - and the rest as the
// description. And it says what keeps a line from being booked.
func TestAQuickEntryIsReadByAFixedWayOfWriting(t *testing.T) {
	t.Parallel()

	p := open(t)

	// Tuesday 24 November 2026, and an account's projects, one of them closed.
	cases := []struct {
		Line        string   `json:"line"`
		Day         string   `json:"day"`
		Hours       float64  `json:"hours"`
		Project     int      `json:"project"`
		Description string   `json:"description"`
		Problems    []string `json:"problems"`
	}{
		{"2,5h gestern Website-Relaunch #meeting Kundentermin", "2026-11-23", 2.5, 1, "#meeting Kundentermin", nil},
		{"90min heute @supp Telefon", "2026-11-24", 1.5, 2, "Telefon", nil},
		{"montag 9-11:30 review", "2026-11-23", 2.5, 0, "review", nil},
		{"2h30 23.11. Website relaunch", "2026-11-23", 2.5, 1, "", nil},
		{"yesterday 1.5 hours Internal", "2026-11-23", 1.5, 5, "", nil},
		{"1h 30min sonntag", "2026-11-22", 1.5, 0, "", nil},
		{"2026-11-20 3 Std Support Ticket #42", "2026-11-20", 3, 2, "Ticket #42", nil},
		{"Freitag 8:00–12:15 Uhr Website relaunch", "2026-11-20", 4.25, 1, "", nil},
		{"0,25 h heute Website care", "2026-11-24", 0.25, 3, "", nil},
		{"vorgestern 45 min", "2026-11-22", 0.75, 0, "", nil},
		{"1h Supporting docs", "2026-11-24", 1, 0, "Supporting docs", nil},
		{"2h 31.02. Test", "2026-11-24", 2, 0, "31.02. Test", nil},
		{"Kundentermin @web", "2026-11-24", 0, 0, "Kundentermin", []string{"noHours", "ambiguousProject"}},
		{"1h @old", "2026-11-24", 1, 0, "", []string{"unknownProject"}},
		{"11-9 Planung", "2026-11-24", 0, 0, "11-9 Planung", []string{"noHours"}},
	}

	lines := make([]string, len(cases))
	for i, c := range cases {
		lines[i] = c.Line
	}

	asked, err := json.Marshal(lines)
	if err != nil {
		t.Fatal(err)
	}

	var read []struct {
		Day         string   `json:"day"`
		Hours       float64  `json:"hours"`
		Project     int      `json:"project"`
		Description string   `json:"description"`
		Problems    []string `json:"problems"`
	}

	p.awaitJSON(`(async () => {
		const projects = [
			{ id: 1, name: 'Website relaunch', status: 'active' },
			{ id: 2, name: 'Support', status: 'active' },
			{ id: 3, name: 'Website care', status: 'active' },
			{ id: 4, name: 'Old work', status: 'completed' },
			{ id: 5, name: 'Internal', status: 'active' },
		];
		return `+string(asked)+`.map((line) => {
			const read = readQuickEntry(line, '2026-11-24', projects);
			return { day: read.day, hours: read.hours ?? 0, project: read.project?.id ?? 0,
				description: read.description, problems: read.problems.map((problem) => problem.code) };
		});
	})()`, &read)

	for i, c := range cases {
		got := read[i]

		if got.Day != c.Day || got.Hours != c.Hours || got.Project != c.Project || got.Description != c.Description ||
			strings.Join(got.Problems, ",") != strings.Join(c.Problems, ",") {
			t.Errorf("%q was read as %+v, want day %s, %v hours, project %d, %q, problems %v",
				c.Line, got, c.Day, c.Hours, c.Project, c.Description, c.Problems)
		}
	}
}

// A quick entry books through the booking form: the line under the field says
// what will be booked, Enter books it for the day, the project and with the
// description it names, and empties the field - and a line without hours says
// so and books nothing.
func TestAQuickEntryBooksThroughTheForm(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	var setUp struct {
		Project   int    `json:"project"`
		Yesterday string `json:"yesterday"`
	}

	p.awaitJSON(`(async () => {
		const year = todayISO().slice(0, 4);
		const project = await api('/projects', { method: 'POST', body: JSON.stringify({
			name: 'Website relaunch', startDate: year + '-01-01' }) });
		await loadProjects();
		return { project: project.id, yesterday: daysBefore(todayISO(), 1) };
	})()`, &setUp)

	const line = `#form-quick input[name="line"]`

	p.run("open the time entries", p.click(`.tab[data-view="timesheets"]`))
	p.showTabOf("#form-quick")

	// Without hours: said, and nothing booked.
	p.run("write a line without hours", chromedp.SendKeys(line, "Kundentermin", chromedp.ByQuery))
	p.waitForText("#quick-preview", "no hours yet")
	p.run("press Enter", chromedp.SendKeys(line, kb.Enter, chromedp.ByQuery))

	var none int

	p.awaitJSON(`(async () => (await api('/timesheets?from=2000-01-01&to=2100-01-01')).totalCount)()`, &none)

	if none != 0 {
		t.Fatalf("a line without hours booked %d entries", none)
	}

	var kept string

	p.run("read the field", chromedp.Value(line, &kept, chromedp.ByQuery))

	if kept != "Kundentermin" {
		t.Errorf("a line that could not be booked was taken away: the field holds %q", kept)
	}

	p.run("clear the line", chromedp.Evaluate(`(() => {
		const field = document.querySelector('#form-quick input[name="line"]');
		field.value = '';
		field.dispatchEvent(new Event('input', { bubbles: true }));
	})()`, nil))

	p.run("write a booking", chromedp.SendKeys(line, "2,5h gestern Website-Relaunch #meeting Kundentermin", chromedp.ByQuery))
	p.waitForText("#quick-preview", "Website relaunch")

	if preview := p.text("#quick-preview"); !strings.Contains(preview, "2.50") || !strings.Contains(preview, "#meeting Kundentermin") {
		t.Errorf("the line under the field says %q, want the hours and the description", preview)
	}

	p.run("press Enter", chromedp.SendKeys(line, kb.Enter, chromedp.ByQuery))
	p.waitForText("#toast", "Time booked")

	var booked struct {
		Items []struct {
			Date          string  `json:"date"`
			DurationHours float64 `json:"durationHours"`
			ProjectID     int     `json:"projectId"`
			Description   string  `json:"description"`
		} `json:"items"`
	}

	p.awaitJSON(`(async () => api('/timesheets?from=2000-01-01&to=2100-01-01'))()`, &booked)

	if len(booked.Items) != 1 {
		t.Fatalf("the quick entry booked %d entries, want one", len(booked.Items))
	}

	entry := booked.Items[0]
	if !strings.HasPrefix(entry.Date, setUp.Yesterday) || entry.DurationHours != 2.5 || entry.ProjectID != setUp.Project ||
		entry.Description != "#meeting Kundentermin" {
		t.Errorf("the quick entry booked %+v, want 2.5 hours yesterday (%s) on project %d with its description",
			entry, setUp.Yesterday, setUp.Project)
	}

	var left string

	p.run("read the field", chromedp.Value(line, &left, chromedp.ByQuery))

	if left != "" || p.text("#quick-preview") != "" {
		t.Errorf("after booking, the field holds %q and the line under it %q", left, p.text("#quick-preview"))
	}
}

// The line under the field speaks the language the screen does, from the moment
// the language is changed rather than from the next letter typed.
func TestAQuickEntrysLineFollowsTheLanguage(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	p.run("open the time entries", p.click(`.tab[data-view="timesheets"]`))
	p.showTabOf("#form-quick")
	p.run("write a booking", chromedp.SendKeys(`#form-quick input[name="line"]`, "2,5h gestern Kundentermin", chromedp.ByQuery))
	p.waitForText("#quick-preview", "2.50 h")

	p.chooseLanguage("de")
	p.waitForText("#quick-preview", "2,50 Std.")
}

// The button that books a line stands where the booking form below it puts its
// own: in a row of its own under the field, at the field's left edge. It stood
// at the end of the field, the one button on the tab to be looked for at the
// right.
func TestAQuickEntrysButtonStandsWhereTheBookingFormsDoes(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyWorker()

	p.run("open the time entries", p.click(`.tab[data-view="timesheets"]`))
	p.showTabOf("#form-quick")

	type placed struct {
		Left      float64 `json:"left"`
		Below     float64 `json:"below"`
		InActions bool    `json:"inActions"`
	}

	var layout struct {
		Quick   placed `json:"quick"`
		Booking placed `json:"booking"`
	}

	p.evalJSON(`JSON.stringify((() => {
		// How far the submit button stands right of the form's leftmost field,
		// and how far under its lowest one.
		const measure = (selector) => {
			const form = document.querySelector(selector);
			const fields = [...form.querySelectorAll('input:not([type="hidden"]), select, textarea')]
				.map((field) => field.getBoundingClientRect()).filter((box) => box.width > 0);
			const button = form.querySelector('button[type="submit"]');
			const box = button.getBoundingClientRect();

			return {
				left: box.left - Math.min(...fields.map((field) => field.left)),
				below: box.top - Math.max(...fields.map((field) => field.bottom)),
				inActions: Boolean(button.closest('.form-actions')),
			};
		};

		return { quick: measure('#form-quick'), booking: measure('#form-timesheet') };
	})())`, &layout)

	if !layout.Booking.InActions || math.Abs(layout.Booking.Left) > 1 || layout.Booking.Below <= 0 {
		t.Fatalf("the booking form's own button measures %+v, so this is not measuring what it was written for", layout.Booking)
	}

	if !layout.Quick.InActions || math.Abs(layout.Quick.Left) > 1 || layout.Quick.Below <= 0 {
		t.Errorf("the quick entry's button stands %.0fpx right of its field's edge and %.0fpx under it (in a row of its own: %t), "+
			"want it where the booking form's stands", layout.Quick.Left, layout.Quick.Below, layout.Quick.InActions)
	}
}
