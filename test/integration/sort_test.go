//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// The entry list is ordered as it is asked to be - by day, by hours or by
// description, either way - and the order holds across pages.
//
// A screen sorts the entry list on the server because it holds one page of it:
// sorted in the browser, "oldest first" would have been the oldest of the
// twenty-five newest, which looks exactly like an answer and is not one. The
// default stays the newest day first, which every reader that totals or exports
// relies on.
func TestTheEntryListIsOrderedAsAskedAcrossPages(t *testing.T) {
	t.Parallel()

	a := start(t)
	admin := a.signInAsAdmin("a-much-better-password")
	anna := a.signInAsUser(admin, "Anna", "anna@example.com")

	// Hours falling as the days rise, and descriptions in neither order.
	for _, entry := range []struct {
		day   string
		hours float64
		text  string
	}{
		{"2026-08-01", 5, "charlie"},
		{"2026-08-02", 4, "alpha"},
		{"2026-08-03", 3, "echo"},
		{"2026-08-04", 2, "bravo"},
		{"2026-08-05", 1, "delta"},
	} {
		entryOf(t, anna, map[string]any{"date": entry.day, "durationHours": entry.hours, "description": entry.text})
	}

	listed := func(query string, of func(timesheetResponse) string) []string {
		t.Helper()

		var out []string
		for _, entry := range page(t, anna, query).Items {
			out = append(out, of(entry))
		}

		return out
	}

	day := func(entry timesheetResponse) string { return entry.Date[:10] }
	hours := func(entry timesheetResponse) string { return fmt.Sprint(entry.DurationHours) }
	text := func(entry timesheetResponse) string { return *entry.Description }

	for _, ask := range []struct {
		query string
		of    func(timesheetResponse) string
		want  []string
	}{
		{"?sort=date&order=asc&limit=3", day, []string{"2026-08-01", "2026-08-02", "2026-08-03"}},
		{"?sort=date&order=asc&limit=3&offset=3", day, []string{"2026-08-04", "2026-08-05"}},
		{"?sort=hours&order=desc", hours, []string{"5", "4", "3", "2", "1"}},
		{"?sort=hours&order=asc&limit=2", hours, []string{"1", "2"}},
		{"?sort=description&order=asc", text, []string{"alpha", "bravo", "charlie", "delta", "echo"}},
		{"?sort=description&order=desc&limit=1", text, []string{"echo"}},
		{"", day, []string{"2026-08-05", "2026-08-04", "2026-08-03", "2026-08-02", "2026-08-01"}},
	} {
		if got := listed(ask.query, ask.of); !slices.Equal(got, ask.want) {
			t.Errorf("GET /timesheets%s listed %q, want %q", ask.query, got, ask.want)
		}
	}

	// A column the list cannot be ordered by, and a direction that is neither,
	// are refused rather than quietly answered in the default order.
	for _, query := range []string{"?sort=project", "?order=sideways"} {
		if refused := anna.api(http.MethodGet, "/timesheets"+query, nil); refused.Status != http.StatusBadRequest {
			t.Errorf("GET /timesheets%s was answered %d, want 400", query, refused.Status)
		}
	}
}

// The administration log is read oldest first when asked, which is how a
// change log is read for an audit: from the start of the period forwards.
func TestTheChangeLogIsReadOldestFirstWhenAsked(t *testing.T) {
	t.Parallel()

	a := start(t)
	admin := a.signInAsAdmin("a-much-better-password")

	for i := 1; i <= 3; i++ {
		admin.must(admin.api(http.MethodPost, "/roles", map[string]any{
			"name":        fmt.Sprintf("reviewer-%02d", i),
			"description": "Reads what others booked",
			"permissions": []string{"timesheets:read:own"},
		}), http.StatusCreated)
	}

	names := func(query string) []string {
		t.Helper()

		var log adminChanges

		admin.must(admin.api(http.MethodGet, "/admin/changes?"+query, nil), http.StatusOK).Data(t, &log)

		var out []string
		for _, entry := range log.Entries {
			if entry.Target != nil {
				out = append(out, entry.Target.Name)
			}
		}

		return out
	}

	if got := names("area=roles&order=asc"); !slices.Equal(got, []string{"reviewer-01", "reviewer-02", "reviewer-03"}) {
		t.Errorf("the role changes oldest first read %q", got)
	}

	if got := names("area=roles&order=desc&limit=1"); strings.Join(got, ",") != "reviewer-03" {
		t.Errorf("the newest role change reads %q, want reviewer-03", got)
	}

	if refused := admin.api(http.MethodGet, "/admin/changes?order=upwards", nil); refused.Status != http.StatusBadRequest {
		t.Errorf("an order that is neither direction was answered %d, want 400", refused.Status)
	}
}
