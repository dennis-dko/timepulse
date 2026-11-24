package spreadsheet_test

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/dennis-dko/timepulse/internal/support/spreadsheet"
)

// An evaluation's workbook writes what the screen showed, with every figure it
// could read written as a number: a column of hours somebody cannot add up is a
// list, and the PDF already is one.
func TestAnEvaluationsFiguresAreWrittenAsNumbers(t *testing.T) {
	hours, total := 8.5, 12.0

	written, err := spreadsheet.WriteEvaluation(spreadsheet.Evaluation{
		Title:    "Report: roof / cellar [2026]",
		Subtitle: "1.11.2026 – 30.11.2026",
		Sections: []spreadsheet.EvaluationSection{
			{Heading: "Hours per day"},
			{
				Heading: "Result",
				Columns: []string{"Day", "Hours"},
				Rows: [][]spreadsheet.Cell{
					{spreadsheet.Text("2.11.2026"), spreadsheet.Number(hours)},
					{spreadsheet.Text("3.11.2026"), spreadsheet.Text("–")},
				},
			},
		},
		Summary:  []spreadsheet.EvaluationLine{{Label: "Total", Value: spreadsheet.Number(total)}},
		Author:   "Wera",
		Written:  time.Date(2026, 11, 25, 9, 30, 0, 0, time.UTC),
		Language: "de",
	})
	if err != nil {
		t.Fatalf("writing: %v", err)
	}

	book, err := excelize.OpenReader(bytes.NewReader(written))
	if err != nil {
		t.Fatalf("the workbook does not open: %v", err)
	}

	defer func() { _ = book.Close() }()

	// The title, made a name Excel takes: no slash, no brackets, no colon.
	sheets := book.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Report roof cellar 2026" {
		t.Fatalf("the sheets are %q, want one named after the title", sheets)
	}

	sheet := sheets[0]

	rows, err := book.GetRows(sheet)
	if err != nil {
		t.Fatalf("reading the rows: %v", err)
	}

	find := func(text string) (string, int) {
		t.Helper()

		for r, row := range rows {
			for c, cell := range row {
				if cell == text {
					name, _ := excelize.CoordinatesToCellName(c+1, r+1)

					return name, r
				}
			}
		}

		t.Fatalf("the sheet does not say %q: %q", text, rows)

		return "", 0
	}

	if title, _ := find("Report: roof / cellar [2026]"); title != "A1" {
		t.Errorf("the title stands at %s, want A1", title)
	}

	find("1.11.2026 – 30.11.2026")
	find("Hours per day")

	_, day := find("2.11.2026")

	figure, _ := excelize.CoordinatesToCellName(2, day+1)

	if kind, _ := book.GetCellType(sheet, figure); kind == excelize.CellTypeSharedString || kind == excelize.CellTypeInlineString {
		t.Errorf("8.5 hours are written as text at %s", figure)
	}

	if value, _ := book.GetCellValue(sheet, figure, excelize.Options{RawCellValue: true}); value != "8.5" {
		t.Errorf("the hours at %s read %q, want 8.5", figure, value)
	}

	// A dash is what the screen wrote where there is no figure, and stays text.
	find("–")

	_, totalRow := find("Total")

	if value, _ := book.GetCellValue(sheet, "B"+strconv.Itoa(totalRow+1), excelize.Options{RawCellValue: true}); value != "12" {
		t.Errorf("the total reads %q, want the number 12", value)
	}

	props, err := book.GetDocProps()
	if err != nil {
		t.Fatalf("reading the properties: %v", err)
	}

	if props.Creator != "Wera" || props.Title != "Report: roof / cellar [2026]" || props.Language != "de" {
		t.Errorf("the workbook's properties are %+v", props)
	}
}

// Whatever the title, the sheet gets a name Excel takes.
func TestAnEvaluationsSheetIsNamedWhateverItsTitle(t *testing.T) {
	for title, want := range map[string]string{
		"":         "Sheet1",
		"/:?*[]":   "Sheet1",
		"'Quoted'": "Quoted",
		"Changes to the installation since the first day it ran": "Changes to the installation sin",
	} {
		written, err := spreadsheet.WriteEvaluation(spreadsheet.Evaluation{Title: title})
		if err != nil {
			t.Errorf("%q: writing: %v", title, err)

			continue
		}

		book, err := excelize.OpenReader(bytes.NewReader(written))
		if err != nil {
			t.Errorf("%q: the workbook does not open: %v", title, err)

			continue
		}

		if sheets := book.GetSheetList(); len(sheets) != 1 || sheets[0] != want {
			t.Errorf("%q made the sheets %q, want %q", title, sheets, want)
		}

		_ = book.Close()
	}
}
