//go:build integration

package integration

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// An evaluation leaves as a workbook from the request its PDF is made from, and
// a figure the screen read as a number arrives as one: a column of hours that
// cannot be added up is a list, and the PDF already is one.
func TestAnEvaluationLeavesAsAWorkbookOfFigures(t *testing.T) {
	t.Parallel()

	_, _, worker := startWithWorker(t)

	r := worker.api(http.MethodPost, "/exports/workbook", map[string]any{
		"title":    "Overtime",
		"subtitle": "1.11.2026 – 30.11.2026",
		"sections": []map[string]any{{
			"heading": "Balance",
			"chart":   "",
			"table": map[string]any{
				"columns": []string{"Day", "Booked"},
				"numeric": []bool{false, true},
				"rows":    [][]string{{"2.11.2026", "8.50 h"}, {"3.11.2026", "–"}},
				"figures": [][]any{{nil, 8.5}, {nil, nil}},
			},
		}},
		"summary":        []map[string]any{{"label": "Balance", "value": "+0.50 h"}},
		"summaryFigures": []any{0.5},
	})

	if !accepted(r.Status) {
		t.Fatalf("the workbook was refused with %d: %.200q", r.Status, r.Body)
	}

	if kind := r.Header.Get("Content-Type"); !strings.Contains(kind, "spreadsheetml") {
		t.Errorf("the answer says it is %q", kind)
	}

	book, err := excelize.OpenReader(bytes.NewReader(r.Body))
	if err != nil {
		t.Fatalf("the answer does not open as a workbook: %v", err)
	}

	defer func() { _ = book.Close() }()

	sheet := book.GetSheetList()[0]

	rows, err := book.GetRows(sheet)
	if err != nil {
		t.Fatalf("reading the rows: %v", err)
	}

	var hours, balance string

	for r, row := range rows {
		if len(row) > 1 && row[0] == "2.11.2026" {
			hours, _ = book.GetCellValue(sheet, "B"+strconv.Itoa(r+1), excelize.Options{RawCellValue: true})
		}

		if len(row) > 1 && row[0] == "Balance" && r > 3 {
			balance, _ = book.GetCellValue(sheet, "B"+strconv.Itoa(r+1), excelize.Options{RawCellValue: true})
		}
	}

	if hours != "8.5" {
		t.Errorf("the hours read %q, want the number 8.5 rather than %q as written on screen", hours, "8.50 h")
	}

	if balance != "0.5" {
		t.Errorf("the balance reads %q, want the number 0.5", balance)
	}
}

// The workbook is bounded as the document is, because it is read from the same
// request: a table no screen sends is refused rather than written.
func TestAnEvaluationsWorkbookIsBoundedAsItsDocumentIs(t *testing.T) {
	t.Parallel()

	_, _, worker := startWithWorker(t)

	rows := make([][]string, 2001)
	for i := range rows {
		rows[i] = []string{"x"}
	}

	r := worker.api(http.MethodPost, "/exports/workbook", map[string]any{
		"title":    "Too long",
		"sections": []map[string]any{{"table": map[string]any{"columns": []string{"A"}, "rows": rows}}},
	})

	if r.Status != http.StatusBadRequest || errorCode(t, r) != "documentTooLong" {
		t.Errorf("a table of 2001 rows answered %d %q, want 400 documentTooLong", r.Status, errorCode(t, r))
	}
}
