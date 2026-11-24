package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// The routes the tools call, by the key Route.key gives them.
const (
	meRoute         = "GET /api/v1/me"
	projectsRoute   = "GET /api/v1/projects"
	entriesRoute    = "GET /api/v1/timesheets"
	bookRoute       = "POST /api/v1/timesheets"
	correctRoute    = "PUT /api/v1/timesheets/{id}"
	deleteRoute     = "DELETE /api/v1/timesheets/{id}"
	statisticsRoute = "GET /api/v1/me/statistics"
	timerRoute      = "GET /api/v1/me/timer"
	startRoute      = "POST /api/v1/me/timer"
	stopRoute       = "POST /api/v1/me/timer/stop"
	noticesRoute    = "GET /api/v1/me/notices"
	overtimeRoute   = "GET /api/v1/users/{id}/overtime"
	leaveRoute      = "GET /api/v1/me/vacation"
	absencesRoute   = "GET /api/v1/me/absences"
)

// tool is one thing an assistant may do: what it is called and told, the
// arguments it takes as a JSON Schema, what it promises about the world, the
// routes it calls, and how.
type tool struct {
	name, title, description string

	input map[string]any

	readOnly, destructive, idempotent bool

	routes []string
	run    func(e *endpoint, r *http.Request, arguments json.RawMessage) (json.RawMessage, *refusal)
}

// tools are the tools offered, about the caller's own time and nothing else.
func tools() []tool {
	return []tool{
		{
			name:  "list_projects",
			title: "List projects",
			description: "Lists your projects: id, name, status, start and end, and for a project with an " +
				"hour budget the budget and the hours booked on it. Only an active project takes bookings.",
			input: object(nil, map[string]any{
				"status": map[string]any{
					"type": "string", "enum": []string{"active", "completed", "archived"},
					"description": "Only the projects with this status.",
				},
			}),
			readOnly: true,
			routes:   []string{projectsRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					Status string `json:"status"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				return e.call(r, apiCall{route: projectsRoute, query: queryOf("status", args.Status)})
			},
		},
		{
			name:  "list_time_entries",
			title: "List time entries",
			description: "Lists your time entries in a period: id, day, hours, project id and description. " +
				"Narrowed by project or by words in the description, a page at a time; totalCount says " +
				"how many there are in all.",
			input: object(nil, map[string]any{
				"from":      dateProperty("The first day, YYYY-MM-DD."),
				"to":        dateProperty("The last day, YYYY-MM-DD."),
				"projectId": idProperty("Only the entries on this project."),
				"search":    textProperty("Only the entries whose description contains this."),
				"limit":     countProperty("How many to list, at most 500; 100 if not given.", 1, 500),
				"offset":    countProperty("How many to skip, for the next page.", 0, 0),
			}),
			readOnly: true,
			routes:   []string{entriesRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					From      string `json:"from"`
					To        string `json:"to"`
					ProjectID uint   `json:"projectId"`
					Search    string `json:"search"`
					Limit     uint   `json:"limit"`
					Offset    uint   `json:"offset"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				return e.call(r, apiCall{route: entriesRoute, query: queryOf(
					"from", args.From, "to", args.To, "projectId", number(args.ProjectID),
					"q", args.Search, "limit", number(args.Limit), "offset", number(args.Offset))})
			},
		},
		{
			name:  "summarize_hours",
			title: "Summarize hours",
			description: "Totals your hours in a period: altogether, per day, per project and per activity - " +
				"a word written with a # in a description, such as #meeting. This month if no period is given.",
			input: object(nil, map[string]any{
				"from":      dateProperty("The first day, YYYY-MM-DD."),
				"to":        dateProperty("The last day, YYYY-MM-DD."),
				"projectId": idProperty("Only the hours on this project."),
			}),
			readOnly: true,
			routes:   []string{statisticsRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					From      string `json:"from"`
					To        string `json:"to"`
					ProjectID uint   `json:"projectId"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				return e.call(r, apiCall{route: statisticsRoute, query: queryOf(
					"from", args.From, "to", args.To, "projectId", number(args.ProjectID))})
			},
		},
		{
			name:  "book_time",
			title: "Book time",
			description: "Books hours you worked on a day as a time entry. hours is decimal - 1.5 is an hour " +
				"and a half - at least 0.01 and at most the daily ceiling your account allows. projectId " +
				"is optional and names one of your active projects.",
			input: object([]string{"date", "hours"}, map[string]any{
				"date":        dateProperty("The day worked, YYYY-MM-DD."),
				"hours":       map[string]any{"type": "number", "description": "The hours worked, decimal."},
				"projectId":   idProperty("The project worked on."),
				"description": textProperty("What was done."),
			}),
			routes: []string{bookRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					Date        *string  `json:"date"`
					Hours       *float64 `json:"hours"`
					ProjectID   *uint    `json:"projectId"`
					Description *string  `json:"description"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				if args.Date == nil || args.Hours == nil {
					return nil, missing("date and hours")
				}

				return e.call(r, apiCall{route: bookRoute, body: present(
					"date", args.Date, "durationHours", args.Hours, "projectId", args.ProjectID,
					"description", args.Description)})
			},
		},
		{
			name:  "correct_time_entry",
			title: "Correct a time entry",
			description: "Corrects one of your time entries by its id. Only the fields given change, and the " +
				"version replaced is kept in the entry's history.",
			input: object([]string{"id"}, map[string]any{
				"id":          idProperty("The entry, by its id from list_time_entries."),
				"date":        dateProperty("The day it moves to, YYYY-MM-DD."),
				"hours":       map[string]any{"type": "number", "description": "The hours it now holds, decimal."},
				"projectId":   idProperty("The project it moves to."),
				"description": textProperty("What it now says was done."),
			}),
			destructive: true,
			idempotent:  true,
			routes:      []string{correctRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					ID          uint     `json:"id"`
					Date        *string  `json:"date"`
					Hours       *float64 `json:"hours"`
					ProjectID   *uint    `json:"projectId"`
					Description *string  `json:"description"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				if args.ID == 0 {
					return nil, missing("id")
				}

				return e.call(r, apiCall{
					route:  correctRoute,
					params: map[string]string{"id": number(args.ID)},
					body: present("date", args.Date, "durationHours", args.Hours, "projectId", args.ProjectID,
						"description", args.Description),
				})
			},
		},
		{
			name:        "delete_time_entry",
			title:       "Delete a time entry",
			description: "Deletes one of your time entries by its id, with its history. It cannot be undone.",
			input: object([]string{"id"}, map[string]any{
				"id": idProperty("The entry, by its id from list_time_entries."),
			}),
			destructive: true,
			idempotent:  true,
			routes:      []string{deleteRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					ID uint `json:"id"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				if args.ID == 0 {
					return nil, missing("id")
				}

				return e.call(r, apiCall{route: deleteRoute, params: map[string]string{"id": number(args.ID)}})
			},
		},
		{
			name:        "get_stopwatch",
			title:       "Read the stopwatch",
			description: "Says whether your stopwatch is running, since when, and on which project.",
			input:       object(nil, map[string]any{}),
			readOnly:    true,
			routes:      []string{timerRoute},
			run:         plain(timerRoute),
		},
		{
			name:  "start_stopwatch",
			title: "Start the stopwatch",
			description: "Starts your stopwatch, on a project and with a description if given. There is one " +
				"per person, so a start while it runs is refused.",
			input: object(nil, map[string]any{
				"projectId":   idProperty("The project being worked on."),
				"description": textProperty("What is being done."),
			}),
			routes: []string{startRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					ProjectID   *uint   `json:"projectId"`
					Description *string `json:"description"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				return e.call(r, apiCall{route: startRoute, body: present(
					"projectId", args.ProjectID, "description", args.Description)})
			},
		},
		{
			name:  "stop_stopwatch",
			title: "Stop the stopwatch",
			description: "Stops your stopwatch and books the time it measured as a time entry: up to now, or " +
				"up to endedAt for a stopwatch left running after the work ended.",
			input: object(nil, map[string]any{
				"endedAt": map[string]any{
					"type": "string", "format": "date-time",
					"description": "When the work ended, as an RFC 3339 time such as 2026-11-24T17:30:00+01:00.",
				},
			}),
			routes: []string{stopRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					EndedAt *string `json:"endedAt"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				return e.call(r, apiCall{route: stopRoute, body: present("endedAt", args.EndedAt)})
			},
		},
		{
			name:  "get_balance",
			title: "Read the running balance",
			description: "Reads your running balance: the hours you are ahead of your targets (positive) or " +
				"behind them (negative), counted from the first day that counts, with the bounds you set " +
				"and whether it lies outside them - and the last working day with nothing booked on it.",
			input:    object(nil, map[string]any{}),
			readOnly: true,
			routes:   []string{noticesRoute},
			run:      plain(noticesRoute),
		},
		{
			name:  "get_overtime",
			title: "Read the overtime of a period",
			description: "Reads your overtime in a period day by day: what each day asked, what was booked on " +
				"it and the difference, with the period's total. This month up to today if no period is given.",
			input: object(nil, map[string]any{
				"from": dateProperty("The first day, YYYY-MM-DD."),
				"to":   dateProperty("The last day, YYYY-MM-DD."),
			}),
			readOnly: true,
			routes:   []string{meRoute, overtimeRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					From string `json:"from"`
					To   string `json:"to"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				// The balance is read by account, and this is the caller's own.
				me, refused := e.call(r, apiCall{route: meRoute})
				if refused != nil {
					return nil, refused
				}

				var who struct {
					User struct {
						ID uint `json:"id"`
					} `json:"user"`
				}

				if err := json.Unmarshal(me, &who); err != nil {
					return nil, &refusal{status: http.StatusInternalServerError, message: err.Error()}
				}

				return e.call(r, apiCall{
					route:  overtimeRoute,
					params: map[string]string{"id": number(who.User.ID)},
					query:  queryOf("from", args.From, "to", args.To),
				})
			},
		},
		{
			name:  "get_leave",
			title: "Read the leave of a year",
			description: "Reads your leave for a year: the days given, carried over, taken, planned and left. " +
				"This year if none is given.",
			input: object(nil, map[string]any{
				"year": map[string]any{"type": "integer", "description": "The year, such as 2026."},
			}),
			readOnly: true,
			routes:   []string{leaveRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					Year uint `json:"year"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				return e.call(r, apiCall{route: leaveRoute, query: queryOf("year", number(args.Year))})
			},
		},
		{
			name:  "list_absences",
			title: "List the days away",
			description: "Lists the days you are away in a period, each with its kind: vacation, halfVacation, " +
				"sick, flexDay or localHoliday. This year if no period is given.",
			input: object(nil, map[string]any{
				"from": dateProperty("The first day, YYYY-MM-DD."),
				"to":   dateProperty("The last day, YYYY-MM-DD."),
			}),
			readOnly: true,
			routes:   []string{absencesRoute},
			run: func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
				var args struct {
					From string `json:"from"`
					To   string `json:"to"`
				}

				if refused := decode(raw, &args); refused != nil {
					return nil, refused
				}

				return e.call(r, apiCall{route: absencesRoute, query: queryOf("from", args.From, "to", args.To)})
			},
		},
	}
}

// plain is a tool that takes nothing and calls one route.
func plain(route string) func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
	return func(e *endpoint, r *http.Request, raw json.RawMessage) (json.RawMessage, *refusal) {
		var args struct{}

		if refused := decode(raw, &args); refused != nil {
			return nil, refused
		}

		return e.call(r, apiCall{route: route})
	}
}

// routesOf are the routes some tool calls.
func routesOf(offered []tool) []string {
	var routes []string

	for _, t := range offered {
		routes = append(routes, t.routes...)
	}

	return routes
}

// describe is how tools/list names the tools.
func describe(offered []tool) []map[string]any {
	described := make([]map[string]any, 0, len(offered))

	for _, t := range offered {
		described = append(described, map[string]any{
			"name":        t.name,
			"title":       t.title,
			"description": t.description,
			"inputSchema": t.input,
			"annotations": map[string]any{
				"title":           t.title,
				"readOnlyHint":    t.readOnly,
				"destructiveHint": t.destructive,
				"idempotentHint":  t.idempotent || t.readOnly,
				// Nothing reaches past this installation.
				"openWorldHint": false,
			},
		})
	}

	return described
}

// decode reads a tool's arguments, refusing a name the tool does not take and
// a value of the wrong kind in words an assistant can act on.
func decode(raw json.RawMessage, into any) *refusal {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || string(trimmed) == "null" {
		raw = json.RawMessage("{}")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(into); err != nil {
		return &refusal{status: http.StatusBadRequest, message: "the arguments do not fit the tool: " + err.Error()}
	}

	return nil
}

func missing(what string) *refusal {
	return &refusal{status: http.StatusBadRequest, message: "the tool needs " + what}
}

// queryOf is a query of the pairs whose value is not empty.
func queryOf(pairs ...string) url.Values {
	query := url.Values{}

	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			query.Set(pairs[i], pairs[i+1])
		}
	}

	return query
}

// present is a body of the pairs whose value was given, so a field left out is
// left alone rather than set to nothing.
func present(pairs ...any) map[string]any {
	body := map[string]any{}

	for i := 0; i+1 < len(pairs); i += 2 {
		name, _ := pairs[i].(string)

		switch value := pairs[i+1].(type) {
		case *string:
			if value != nil {
				body[name] = *value
			}
		case *float64:
			if value != nil {
				body[name] = *value
			}
		case *uint:
			if value != nil {
				body[name] = *value
			}
		}
	}

	return body
}

// number writes a count or an id for a query or a path, and nothing for zero,
// which no id is and which every count here means as "not given".
func number(n uint) string {
	if n == 0 {
		return ""
	}

	return strconv.FormatUint(uint64(n), 10)
}

func object(required []string, properties map[string]any) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

func dateProperty(description string) map[string]any {
	return map[string]any{"type": "string", "format": "date", "description": description}
}

func idProperty(description string) map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "description": description}
}

func textProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func countProperty(description string, minimum, maximum int) map[string]any {
	property := map[string]any{"type": "integer", "minimum": minimum, "description": description}
	if maximum > 0 {
		property["maximum"] = maximum
	}

	return property
}
