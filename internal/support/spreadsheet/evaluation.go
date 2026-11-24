package spreadsheet

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// Evaluation is what an evaluation screen showed, to be written as a workbook:
// the words as the screen said them, and every figure it could read as a number
// written as one, so a column of hours can be summed.
//
// Nothing here is translated: the screen sent its words already in the reader's
// language, as it does for the PDF of the same screen.
type Evaluation struct {
	Title    string
	Subtitle string
	Sections []EvaluationSection
	Summary  []EvaluationLine

	// Author, Written and Language go into the workbook's properties rather than
	// onto the sheet, which holds what the screen held and nothing else.
	Author   string
	Written  time.Time
	Language string
}

// EvaluationSection is one heading with its table. A section without columns is
// its heading and caption alone, which is what a chart becomes here.
type EvaluationSection struct {
	Heading string
	Caption string
	Columns []string
	Rows    [][]Cell
}

// EvaluationLine is one line of what an evaluation comes to.
type EvaluationLine struct {
	Label string
	Value Cell
}

// maxSheetName is the longest name a sheet may have; Excel refuses a longer one.
const maxSheetName = 31

// WriteEvaluation writes an evaluation as a workbook of one sheet, laid out the
// way its document is: the title, the period, each section's heading over its
// table, and what it all comes to.
//
// One sheet rather than one per section, because a section's table means little
// without the period it is about, and a workbook somebody opens should show what
// it is before it shows a figure.
func WriteEvaluation(e Evaluation) ([]byte, error) {
	book := excelize.NewFile()

	defer func() {
		_ = book.Close()
	}()

	sheet := sheetName(e.Title)

	if err := book.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("naming the sheet: %w", err)
	}

	styles, err := evaluationStyles(book)
	if err != nil {
		return nil, err
	}

	w := &evaluationWriter{book: book, sheet: sheet, styles: styles}

	w.line(styles.title, Text(e.Title))

	if e.Subtitle != "" {
		w.line(0, Text(e.Subtitle))
	}

	for _, section := range e.Sections {
		w.row++

		if section.Heading != "" {
			w.line(styles.heading, Text(section.Heading))
		}

		if section.Caption != "" {
			w.line(styles.caption, Text(section.Caption))
		}

		if len(section.Columns) == 0 {
			continue
		}

		headings := make([]Cell, 0, len(section.Columns))
		for _, column := range section.Columns {
			headings = append(headings, Text(column))
		}

		w.line(styles.columnHeading, headings...)

		decimals := columnsWithFractions(section.Rows)

		for _, row := range section.Rows {
			w.figures(row, decimals)
		}
	}

	if len(e.Summary) > 0 {
		w.row++

		for _, line := range e.Summary {
			w.figures([]Cell{Text(line.Label), line.Value}, map[int]bool{1: hasFraction(line.Value)})
		}
	}

	if w.err != nil {
		return nil, w.err
	}

	if err := w.fitColumns(); err != nil {
		return nil, err
	}

	if err := book.SetDocProps(&excelize.DocProperties{
		Title:    e.Title,
		Subject:  e.Subtitle,
		Creator:  e.Author,
		Created:  e.Written.UTC().Format(time.RFC3339),
		Language: e.Language,
	}); err != nil {
		return nil, fmt.Errorf("writing the workbook's properties: %w", err)
	}

	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("writing the workbook: %w", err)
	}

	return buffer.Bytes(), nil
}

// evaluationStyle names the styles an evaluation is written in.
type evaluationStyle struct {
	title, heading, caption, columnHeading, number, fraction int
}

func evaluationStyles(book *excelize.File) (evaluationStyle, error) {
	var s evaluationStyle

	for _, want := range []struct {
		into  *int
		style *excelize.Style
	}{
		{&s.title, &excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}}},
		{&s.heading, &excelize.Style{Font: &excelize.Font{Bold: true, Size: 12}}},
		{&s.caption, &excelize.Style{Font: &excelize.Font{Italic: true, Color: "666666"}}},
		{&s.columnHeading, &excelize.Style{
			Font:   &excelize.Font{Bold: true},
			Border: []excelize.Border{{Type: "bottom", Color: "999999", Style: 1}},
		}},
		{&s.number, &excelize.Style{NumFmt: 3}},
		{&s.fraction, &excelize.Style{NumFmt: 4}},
	} {
		id, err := book.NewStyle(want.style)
		if err != nil {
			return s, fmt.Errorf("creating a style: %w", err)
		}

		*want.into = id
	}

	return s, nil
}

// evaluationWriter fills the sheet a row at a time and remembers how wide each
// column has to open for what was written into it.
type evaluationWriter struct {
	book   *excelize.File
	sheet  string
	styles evaluationStyle
	row    int
	widths []int
	err    error
}

// line writes a row of cells in one style, zero for none.
func (w *evaluationWriter) line(style int, cells ...Cell) {
	w.row++

	for i, cell := range cells {
		w.cell(i, cell, style)
	}
}

// figures writes a row whose numbers are written as numbers, two decimals in a
// column that holds any fraction and none in one that does not.
func (w *evaluationWriter) figures(cells []Cell, decimals map[int]bool) {
	w.row++

	for i, cell := range cells {
		style := 0

		if cell.Number != nil {
			style = w.styles.number
			if decimals[i] {
				style = w.styles.fraction
			}
		}

		w.cell(i, cell, style)
	}
}

func (w *evaluationWriter) cell(column int, value Cell, style int) {
	if w.err != nil {
		return
	}

	name, err := excelize.CoordinatesToCellName(column+1, w.row)
	if err != nil {
		w.err = fmt.Errorf("naming a cell in row %d: %w", w.row, err)

		return
	}

	shown := value.Text

	if value.Number != nil {
		err = w.book.SetCellFloat(w.sheet, name, *value.Number, -1, 64)
		shown = fmt.Sprintf("%.2f", *value.Number)
	} else {
		err = w.book.SetCellStr(w.sheet, name, value.Text)
	}

	if err == nil && style != 0 {
		err = w.book.SetCellStyle(w.sheet, name, name, style)
	}

	if err != nil {
		w.err = fmt.Errorf("writing row %d: %w", w.row, err)

		return
	}

	// The title and the headings run across the columns beside them rather than
	// widening the first, which is what a long title in a narrow column does.
	if style == w.styles.title || style == w.styles.heading || style == w.styles.caption {
		return
	}

	for len(w.widths) <= column {
		w.widths = append(w.widths, 0)
	}

	w.widths[column] = max(w.widths[column], utf8.RuneCountInString(shown))
}

// fitColumns opens each column wide enough for what is in it, within reason: a
// description of a few hundred characters wraps in a column of sixty rather than
// pushing the figures off the screen.
func (w *evaluationWriter) fitColumns() error {
	for i, width := range w.widths {
		column, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return fmt.Errorf("naming column %d: %w", i+1, err)
		}

		if err := w.book.SetColWidth(w.sheet, column, column, float64(min(max(width+2, 10), 60))); err != nil {
			return fmt.Errorf("setting the width of column %s: %w", column, err)
		}
	}

	return nil
}

// columnsWithFractions says which columns hold a number that is not whole.
func columnsWithFractions(rows [][]Cell) map[int]bool {
	out := map[int]bool{}

	for _, row := range rows {
		for i, cell := range row {
			if hasFraction(cell) {
				out[i] = true
			}
		}
	}

	return out
}

func hasFraction(cell Cell) bool {
	return cell.Number != nil && *cell.Number != math.Trunc(*cell.Number)
}

// sheetName makes a title into a name Excel takes for a sheet: none of the seven
// characters it refuses, no apostrophe at either end, and no longer than it
// allows. A title made of nothing else is a sheet called Sheet1, as a new
// workbook's is.
func sheetName(title string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`:\/?*[]`, r) {
			return ' '
		}

		return r
	}, title)

	cleaned = strings.Trim(strings.Join(strings.Fields(cleaned), " "), "'")

	if utf8.RuneCountInString(cleaned) > maxSheetName {
		cleaned = strings.TrimSpace(string([]rune(cleaned)[:maxSheetName]))
	}

	if cleaned == "" {
		return "Sheet1"
	}

	return cleaned
}
