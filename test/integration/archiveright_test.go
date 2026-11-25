//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/dennis-dko/timepulse/internal/support/spreadsheet"
)

// Whether a project is in the archive is projects:archive's to decide, whichever
// route asks.
//
// The archive route has always required it. A correction carrying the status, a
// project created in the archive and an imported row saying "archived" did not,
// so a role an administrator made without the right archived all the same - and
// took projects back out of the archive, which the project form now offers.
func TestOnlyTheArchiveRightMovesAProjectIntoTheArchiveOrOut(t *testing.T) {
	t.Parallel()

	a := start(t)
	admin := a.signInAsAdmin("a-much-better-password")

	admin.must(admin.api(http.MethodPost, "/roles", map[string]any{
		"name": "planner", "description": "keeps projects and does not archive them",
		"permissions": []string{"projects:read", "projects:write", "timesheets:read:own", "timesheets:write:own"},
	}), http.StatusCreated, http.StatusOK)

	var pia struct {
		ID uint `json:"id"`
	}

	admin.must(admin.api(http.MethodPost, "/users", map[string]any{
		"name": "Pia", "email": "pia@example.com", "role": "planner", "password": "pia-password-1",
	}), http.StatusCreated, http.StatusOK).Data(t, &pia)

	planner := a.newClient()
	planner.signIn("pia@example.com", "pia-password-1")

	var project struct {
		ID     uint   `json:"id"`
		Status string `json:"status"`
	}

	planner.must(planner.api(http.MethodPost, "/projects", map[string]any{
		"name": "Old roof", "startDate": "2026-08-01",
	}), http.StatusCreated, http.StatusOK).Data(t, &project)

	planner.must(planner.api(http.MethodPut, path("/projects/", project.ID),
		map[string]any{"status": "completed"}), http.StatusOK)

	refusedAsMissing := func(what string, r response) {
		t.Helper()

		if r.Status != http.StatusForbidden || errorCode(t, r) != "missingPermission" {
			t.Errorf("%s without projects:archive answered %d %q, want 403 missingPermission: %.200s",
				what, r.Status, errorCode(t, r), r.Body)
		}
	}

	refusedAsMissing("archiving by correction", planner.api(http.MethodPut, path("/projects/", project.ID),
		map[string]any{"status": "archived"}))

	refusedAsMissing("creating a project in the archive", planner.api(http.MethodPost, "/projects",
		map[string]any{"name": "Straight away", "startDate": "2026-08-01", "status": "archived"}))

	book, err := spreadsheet.WriteProjects("", []spreadsheet.ProjectRow{
		{Name: "Old roof", StartDate: mustDay(t, "2026-08-01"), Status: "archived"},
	})
	if err != nil {
		t.Fatalf("building the workbook: %v", err)
	}

	preview, r := importSheet(t, planner, "/projects/import", book, "true")
	if !accepted(r.Status) {
		t.Fatalf("previewing: status %d, body %.200q", r.Status, r.Body)
	}

	if preview.Rejected != 1 || len(preview.Rows) != 1 || preview.Rows[0].ProblemCode != "archiveNeedsTheRight" {
		t.Errorf("a row archiving without projects:archive was previewed as %d refused: %+v",
			preview.Rejected, preview.Rows)
	}

	// Archived by somebody who may, and then asked back out by somebody who may not.
	admin.must(admin.api(http.MethodPut, path("/users/", pia.ID, "/role"), map[string]any{"role": "user"}),
		http.StatusOK)
	planner.must(planner.api(http.MethodPut, path("/projects/", project.ID),
		map[string]any{"status": "archived"}), http.StatusOK)
	admin.must(admin.api(http.MethodPut, path("/users/", pia.ID, "/role"), map[string]any{"role": "planner"}),
		http.StatusOK)

	refusedAsMissing("taking a project out of the archive", planner.api(http.MethodPut,
		path("/projects/", project.ID), map[string]any{"status": "active"}))

	// Corrected otherwise, the archived project keeps its status and is no business
	// of the right.
	planner.must(planner.api(http.MethodPut, path("/projects/", project.ID),
		map[string]any{"name": "Old roof, done", "status": "archived"}), http.StatusOK)
}
