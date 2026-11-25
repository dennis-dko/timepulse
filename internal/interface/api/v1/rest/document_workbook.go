package rest

import (
	"strings"
	"time"

	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/http/response"

	"github.com/dennis-dko/timepulse/internal/support/apperror"
	"github.com/dennis-dko/timepulse/internal/support/spreadsheet"
)

// ExportWorkbook handles POST /api/v1/exports/workbook.
//
// The request Export takes, read the same way and bounded by the same limits,
// written as a workbook rather than as a page - so every screen with a PDF has
// the same figures as a spreadsheet without a route and a writer of its own. A
// figure arrives as a number beside the text the screen wrote it as, and is
// written as one, so a column of hours can be added up. A chart has no cells and
// is left out: a screen made of charts sends the figures behind them as tables.
func (h *DocumentHandler) ExportWorkbook(c *gofr.Context) (any, error) {
	var req DocumentRequest

	principal, err := h.authz.Principal(c)
	if err != nil {
		return nil, err
	}

	if err := bind(c, &req); err != nil {
		return nil, toHTTPError(err)
	}

	doc, err := req.document()
	if err != nil {
		return nil, toHTTPError(err)
	}

	// Who exported it, and when, go into the workbook's properties rather than
	// onto the sheet, which holds what the screen held and nothing else.
	book := spreadsheet.Evaluation{
		Title:    doc.Title,
		Subtitle: doc.Subtitle,
		Author:   strings.TrimSpace(h.instanceName(c)),
		Written:  time.Now(),
		Language: language(c),
	}

	if principal != nil && principal.User != nil {
		book.Author = principal.User.Name
	}

	for i, section := range doc.Sections {
		out := spreadsheet.EvaluationSection{Heading: section.Heading, Caption: section.Caption}

		if section.Table != nil {
			out.Columns = section.Table.Columns
			out.Rows = cellsOf(section.Table.Rows, req.Sections[i].Table.Figures)
		}

		book.Sections = append(book.Sections, out)
	}

	for i, line := range doc.Summary {
		value := spreadsheet.Text(line.Value)
		if i < len(req.SummaryFigures) && req.SummaryFigures[i] != nil {
			value = spreadsheet.Number(*req.SummaryFigures[i])
		}

		book.Summary = append(book.Summary, spreadsheet.EvaluationLine{Label: line.Label, Value: value})
	}

	content, err := spreadsheet.WriteEvaluation(book)
	if err != nil {
		return nil, toHTTPError(apperror.Internal(err))
	}

	return response.File{Content: content, ContentType: xlsxContentType}, nil
}

// cellsOf pairs each cell's text with its figure, where the screen could read
// one. A figure beyond the rows or the columns it belongs beside is ignored
// rather than refused: it describes nothing on the sheet.
func cellsOf(rows [][]string, figures [][]*float64) [][]spreadsheet.Cell {
	out := make([][]spreadsheet.Cell, 0, len(rows))

	for r, row := range rows {
		cells := make([]spreadsheet.Cell, 0, len(row))

		for c, text := range row {
			if r < len(figures) && c < len(figures[r]) && figures[r][c] != nil {
				cells = append(cells, spreadsheet.Number(*figures[r][c]))

				continue
			}

			cells = append(cells, spreadsheet.Text(text))
		}

		out = append(out, cells)
	}

	return out
}
