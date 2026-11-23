//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type adminChanges struct {
	Entries []struct {
		Action string `json:"action"`
		Actor  struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"actor"`
		Target *struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"target"`
		Detail string `json:"detail"`
	} `json:"entries"`
	More       bool `json:"more"`
	TotalCount uint `json:"totalCount"`
}

// What an administrator changes is recorded with who changed it, through the
// whole stack on every database: an account made, given another role and erased,
// newest first, the erased one kept under its id and no name. The log is the
// administrators' to read, and nobody else's.
func TestTheAdministrationLogRecordsWhoChangedWhat(t *testing.T) {
	t.Parallel()

	_, admin, worker := startWithWorker(t)

	var made struct {
		ID uint `json:"id"`
	}

	admin.must(admin.api(http.MethodPost, "/users", map[string]any{
		"name": "Bea", "email": "bea@example.com", "role": "user", "password": "bea-password-1",
	}), http.StatusCreated, http.StatusOK).Data(t, &made)

	admin.must(admin.api(http.MethodPut, path("/users/", made.ID, "/role"), map[string]any{"role": "user-admin"}),
		http.StatusOK)
	admin.must(admin.api(http.MethodDelete, path("/users/", made.ID), nil), http.StatusOK, http.StatusNoContent)

	var log adminChanges

	admin.must(admin.api(http.MethodGet, "/admin/changes", nil), http.StatusOK).Data(t, &log)

	if len(log.Entries) < 3 {
		t.Fatalf("the log holds %+v, want the three changes", log.Entries)
	}

	deleted, role, created := log.Entries[0], log.Entries[1], log.Entries[2]

	for _, entry := range []struct {
		action string
		got    string
	}{{"user.deleted", deleted.Action}, {"user.role", role.Action}, {"user.created", created.Action}} {
		if entry.got != entry.action {
			t.Errorf("an entry is %q where %q was expected: %+v", entry.got, entry.action, log.Entries[:3])
		}
	}

	if created.Actor.Name == "" || created.Target == nil || created.Target.ID != made.ID || created.Target.Name != "" {
		t.Errorf("the creation reads %+v, want the administrator named and the erased account kept by id alone", created)
	}

	if role.Detail != "user-admin" {
		t.Errorf("the role change keeps %q, want user-admin", role.Detail)
	}

	if refused := worker.api(http.MethodGet, "/admin/changes", nil); refused.Status != http.StatusForbidden {
		t.Errorf("an ordinary account asking for the log was answered %d, want 403", refused.Status)
	}
}

// Checking a file before importing it changes nothing, and the log says so: the
// import is recorded once, and the check before it not at all. The check is the
// same request as the import with dryRun set, which is what let the route's
// entry in the log's table take it for an import of its own.
func TestCheckingAnImportIsNotRecordedAsOne(t *testing.T) {
	t.Parallel()

	a := start(t)
	admin := a.signInAsAdmin("a-much-better-password")

	exported := admin.must(admin.api(http.MethodGet, "/roles/export", nil), http.StatusOK)

	imports := func() int {
		var log adminChanges

		admin.must(admin.api(http.MethodGet, "/admin/changes", nil), http.StatusOK).Data(t, &log)

		counted := 0

		for _, entry := range log.Entries {
			if entry.Action == "roles.imported" {
				counted++
			}
		}

		return counted
	}

	if _, r := importSheet(t, admin, "/roles/import", exported.Body, "true"); !accepted(r.Status) {
		t.Fatalf("checking the file: status %d, body %.300q", r.Status, r.Body)
	}

	if got := imports(); got != 0 {
		t.Errorf("after checking a file the log holds %d imports, want none", got)
	}

	if _, r := importSheet(t, admin, "/roles/import", exported.Body, "false"); !accepted(r.Status) {
		t.Fatalf("importing the file: status %d, body %.300q", r.Status, r.Body)
	}

	if got := imports(); got != 1 {
		t.Errorf("after checking a file and importing it the log holds %d imports, want one", got)
	}
}

// The log is searched, narrowed to a part of the installation and paged by the
// server, over every change it holds, on every database: the statement built
// from the filter - its LIKE and escape, its lists of ids, its count - is the
// part no unit test runs.
func TestTheAdministrationLogIsSearchedNarrowedAndPagedOnTheServer(t *testing.T) {
	t.Parallel()

	a := start(t)
	admin := a.signInAsAdmin("a-much-better-password")

	// Twelve roles, each recorded as made, the newest last.
	for i := 1; i <= 12; i++ {
		admin.must(admin.api(http.MethodPost, "/roles", map[string]any{
			"name":        fmt.Sprintf("reviewer-%02d", i),
			"description": "Reads what others booked",
			"permissions": []string{"timesheets:read:own"},
		}), http.StatusCreated)
	}

	// And an account a search in capitals has to find by its umlaut.
	admin.must(admin.api(http.MethodPost, "/users", map[string]any{
		"name": "Greta Grün", "email": "greta@example.com", "role": "user", "password": "greta-password-1",
	}), http.StatusCreated, http.StatusOK)

	read := func(query string) adminChanges {
		t.Helper()

		var log adminChanges

		admin.must(admin.api(http.MethodGet, "/admin/changes?"+query, nil), http.StatusOK).Data(t, &log)

		return log
	}

	// The second page of five of the roles: counted, newest first, more to come.
	second := read("area=roles&limit=5&offset=5")

	if len(second.Entries) != 5 || second.TotalCount != 12 || !second.More {
		t.Fatalf("the second page of the role changes holds %d entries, a total of %d and more=%v, "+
			"want 5, 12 and more", len(second.Entries), second.TotalCount, second.More)
	}

	for _, entry := range second.Entries {
		if entry.Action != "role.created" {
			t.Errorf("narrowed to the roles, the log answered %q", entry.Action)
		}
	}

	if first := second.Entries[0]; first.Target == nil || first.Target.Name != "reviewer-07" {
		t.Errorf("the second page begins with %+v, want the seventh newest role, reviewer-07", first.Target)
	}

	// A name in capitals finds the change made to that account, and only that.
	greta := read("q=" + url.QueryEscape("GRÜN"))

	if len(greta.Entries) != 1 || greta.Entries[0].Action != "user.created" || greta.TotalCount != 1 {
		t.Errorf("searching for GRÜN found %+v (total %d), want the account made", greta.Entries, greta.TotalCount)
	}

	// A role's name finds the change made to it.
	role := read("q=reviewer-03")

	if len(role.Entries) != 1 || role.Entries[0].Target == nil || role.Entries[0].Target.Name != "reviewer-03" {
		t.Errorf("searching for reviewer-03 found %+v, want the role made", role.Entries)
	}

	// A percent sign is one, not a pattern matching every change.
	if percent := read("q=" + url.QueryEscape("%")); percent.TotalCount != 0 {
		t.Errorf("searching for a percent sign found %d changes, want none", percent.TotalCount)
	}

	// An area the log does not have, and a search longer than anything it holds,
	// are refused rather than answered with an empty log.
	if refused := admin.api(http.MethodGet, "/admin/changes?area=nowhere", nil); refused.Status != http.StatusBadRequest {
		t.Errorf("an unknown area was answered %d, want 400", refused.Status)
	}

	if refused := admin.api(http.MethodGet, "/admin/changes?q="+strings.Repeat("x", 256), nil); refused.Status != http.StatusBadRequest {
		t.Errorf("a search of 256 characters was answered %d, want 400", refused.Status)
	}
}
