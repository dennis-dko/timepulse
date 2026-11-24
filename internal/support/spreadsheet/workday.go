package spreadsheet

import "time"

// workDays is the sheet a period of somebody's working time is written to. Only
// written, never read: it is a record for somebody's own books or payroll, and
// what is recorded is entered day by day on the screen.
//
// The Working Hours Act's notices are left to the document. They are sentences
// the screen words from a rule and a figure, and a second wording of them here
// would be one more copy to keep in step with the first.
var workDays = Table{
	Key:      "Working time",
	Headings: []string{"Date", "Start", "End", "Breaks (minutes)", "Worked", "Booked"},
	Widths:   []float64{12, 8, 8, 16, 12, 12},
}

// WorkDayRow is one day of working time. Recorded says whether its times were
// written down; a day booked on without them carries only its booked hours, and
// its other cells stay empty rather than reading as a day of no work.
type WorkDayRow struct {
	Day          time.Time
	Recorded     bool
	Start, End   string
	BreakMinutes int
	WorkedHours  float64
	BookedHours  float64
}

// WriteWorkDays builds the workbook of a period of working time: figures a
// spreadsheet can add up, the day as the time entries' sheet writes it.
func WriteWorkDays(language string, rows []WorkDayRow) ([]byte, error) {
	cells := make([][]Cell, 0, len(rows))

	for _, row := range rows {
		line := []Cell{Text(row.Day.Format(dateFormat)), Text(""), Text(""), Text(""), Text(""), Number(row.BookedHours)}

		if row.Recorded {
			line[1] = Text(row.Start)
			line[2] = Text(row.End)
			line[3] = Number(float64(row.BreakMinutes))
			line[4] = Number(row.WorkedHours)
		}

		cells = append(cells, line)
	}

	return write(workDays, language, cells)
}
