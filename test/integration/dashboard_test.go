//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// The arrangement of somebody's tiles is kept on their account: GET /me answers
// it in the order it was saved, an empty one goes back to the order the screen
// ships with, what is not an arrangement is refused naming tiles, nobody else's
// changes with it, and it travels in the account's export.
func TestTheArrangementOfTheTilesIsKeptOnTheAccount(t *testing.T) {
	t.Parallel()

	_, admin, worker := startWithWorker(t)

	arrangement := func(c *client) []string {
		var me struct {
			User struct {
				Dashboard []string `json:"dashboard"`
			} `json:"user"`
		}

		c.must(c.api(http.MethodGet, "/me", nil), http.StatusOK).Data(t, &me)

		return me.User.Dashboard
	}

	if got := arrangement(worker); got == nil || len(got) != 0 {
		t.Errorf("a new account's arrangement reads %v, want an empty list", got)
	}

	chosen := []string{"leave", "week", "balance"}

	worker.must(worker.api(http.MethodPut, "/me/dashboard", map[string]any{"tiles": chosen}), http.StatusOK)

	if got := arrangement(worker); !slices.Equal(got, chosen) {
		t.Errorf("after arranging %v, the account reads %v", chosen, got)
	}

	tooMany := make([]string, 25)
	for i := range tooMany {
		tooMany[i] = strings.Repeat("a", i+1)
	}

	for _, refused := range [][]string{{"week", "week"}, {"Week"}, {"two words"}, {""}, tooMany} {
		answer := worker.api(http.MethodPut, "/me/dashboard", map[string]any{"tiles": refused})

		if answer.Status != http.StatusBadRequest || !strings.Contains(string(answer.Body), "tiles") {
			t.Errorf("%v was answered %d: %s, want a refusal naming tiles", refused, answer.Status, answer.Body)
		}
	}

	if got := arrangement(worker); !slices.Equal(got, chosen) {
		t.Errorf("after the refusals the account reads %v, want %v still", got, chosen)
	}

	if got := arrangement(admin); len(got) != 0 {
		t.Errorf("another account's arrangement changed with it: %v", got)
	}

	exported := worker.api(http.MethodGet, "/me/export", nil)

	archive, err := zip.NewReader(bytes.NewReader(exported.Body), int64(len(exported.Body)))
	if err != nil {
		t.Fatalf("the export is not a ZIP: %v", err)
	}

	var account []map[string]any

	for _, file := range archive.File {
		if file.Name != "account.json" {
			continue
		}

		opened, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}

		content, _ := io.ReadAll(opened)
		_ = opened.Close()

		if err := json.Unmarshal(content, &account); err != nil {
			t.Fatalf("account.json is not a list of rows: %v", err)
		}
	}

	if len(account) != 1 || account[0]["dashboard"] != strings.Join(chosen, ",") {
		t.Errorf("the export's account reads %v, want its arrangement %q", account, strings.Join(chosen, ","))
	}

	worker.must(worker.api(http.MethodPut, "/me/dashboard", map[string]any{"tiles": []string{}}), http.StatusOK)

	if got := arrangement(worker); len(got) != 0 {
		t.Errorf("an empty arrangement left %v, want the order the screen ships with", got)
	}
}
