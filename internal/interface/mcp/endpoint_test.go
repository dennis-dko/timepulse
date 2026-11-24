package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/logging"
)

// refused is an error as a handler returns a refusal: a status and a code.
type refused struct {
	status int
	code   string
}

func (e refused) Error() string                 { return "refused by the route" }
func (e refused) StatusCode() int               { return e.status }
func (e refused) Response() map[string]any      { return map[string]any{"code": e.code} }
func (f *fake) answer(route string, a answerer) { f.answers[route] = a }

type answerer func() (any, error)

// fake serves every route a tool calls, recording what each was handed.
type fake struct {
	calls   []handed
	answers map[string]answerer
}

// handed is what a route's handler was given.
type handed struct {
	route string
	id    string
	query map[string]string
	body  map[string]any
}

func (f *fake) routes() []Route {
	var routes []Route

	for _, key := range append(routesOf(tools()), meRoute) {
		method, path, _ := strings.Cut(key, " ")
		routes = append(routes, Route{Method: method, Path: path, Handler: f.handler(key)})
	}

	return routes
}

func (f *fake) handler(route string) gofr.Handler {
	return func(c *gofr.Context) (any, error) {
		got := handed{route: route, id: c.PathParam("id"), query: map[string]string{}}

		for _, name := range []string{"from", "to", "q", "projectId", "limit", "offset", "status", "year"} {
			if value := c.Param(name); value != "" {
				got.query[name] = value
			}
		}

		var body map[string]any
		if err := c.Bind(&body); err == nil {
			got.body = body
		}

		f.calls = append(f.calls, got)

		if answer, ok := f.answers[route]; ok {
			return answer()
		}

		return map[string]any{"route": route}, nil
	}
}

// to finds the call of a route, failing when it was never made.
func (f *fake) to(t *testing.T, route string) handed {
	t.Helper()

	for _, call := range f.calls {
		if call.route == route {
			return call
		}
	}

	t.Fatalf("%s was never called; the calls were %+v", route, f.calls)

	return handed{}
}

func (f *fake) called(route string) bool {
	for _, call := range f.calls {
		if call.route == route {
			return true
		}
	}

	return false
}

// served is the endpoint over the fake routes, in front of a handler that
// answers 418, so a request passed on can be told from one answered.
func served(t *testing.T) (http.Handler, *fake) {
	t.Helper()

	f := &fake{answers: map[string]answerer{}}

	middleware, err := Endpoint(f.routes(), Server{Version: "v9.9.9", Logger: logging.NewMockLogger(logging.INFO)})
	if err != nil {
		t.Fatalf("the endpoint was not made: %v", err)
	}

	return middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})), f
}

func post(h http.Handler, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	answer := httptest.NewRecorder()
	h.ServeHTTP(answer, req)

	return answer
}

// rpc is a JSON-RPC reply as it is read back.
type rpc struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func replied(t *testing.T, answer *httptest.ResponseRecorder) rpc {
	t.Helper()

	if answer.Code != http.StatusOK {
		t.Fatalf("answered %d, want 200: %s", answer.Code, answer.Body)
	}

	var got rpc
	if err := json.Unmarshal(answer.Body.Bytes(), &got); err != nil {
		t.Fatalf("the reply is not JSON-RPC: %v: %s", err, answer.Body)
	}

	return got
}

// result is a tool's answer as it is read back.
type result struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StructuredContent map[string]any `json:"structuredContent"`
	IsError           bool           `json:"isError"`
}

func callTool(t *testing.T, h http.Handler, name, arguments string) result {
	t.Helper()

	got := replied(t, post(h, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"`+name+
		`","arguments":`+arguments+`}}`))
	if got.Error != nil {
		t.Fatalf("calling %s was a protocol error: %+v", name, got.Error)
	}

	var answer result
	if err := json.Unmarshal(got.Result, &answer); err != nil {
		t.Fatalf("the tool's answer is not a result: %v: %s", err, got.Result)
	}

	return answer
}

// A client's own revision is spoken back where it is spoken here, and the
// newest one where it is not - with the tools on offer and the version named.
func TestTheEndpointAgreesOnARevisionOfTheProtocol(t *testing.T) {
	h, _ := served(t)

	for asked, want := range map[string]string{"2025-06-18": "2025-06-18", "2024-11-05": "2025-11-25"} {
		got := replied(t, post(h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+
			asked+`","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))

		var agreed struct {
			ProtocolVersion string         `json:"protocolVersion"`
			Capabilities    map[string]any `json:"capabilities"`
			ServerInfo      struct {
				Version string `json:"version"`
			} `json:"serverInfo"`
		}

		if err := json.Unmarshal(got.Result, &agreed); err != nil {
			t.Fatalf("initialize answered %s: %v", got.Result, err)
		}

		if agreed.ProtocolVersion != want || agreed.Capabilities["tools"] == nil || agreed.ServerInfo.Version != "v9.9.9" {
			t.Errorf("asked for %s, agreed on %+v, want %s with tools and v9.9.9", asked, agreed, want)
		}
	}
}

// A notification is taken with 202 and nothing said, as the transport wants.
func TestANotificationIsTakenWithNothingSaid(t *testing.T) {
	h, _ := served(t)

	answer := post(h, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if answer.Code != http.StatusAccepted || answer.Body.Len() != 0 {
		t.Errorf("a notification was answered %d with %q, want 202 and nothing", answer.Code, answer.Body)
	}
}

// Nothing is sent unasked and nothing is kept between messages, so a GET and a
// DELETE are answered 405, naming the method that is.
func TestOnlyAPostIsAnswered(t *testing.T) {
	h, _ := served(t)

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		answer := httptest.NewRecorder()
		h.ServeHTTP(answer, httptest.NewRequest(method, Path, nil))

		if answer.Code != http.StatusMethodNotAllowed || answer.Header().Get("Allow") != http.MethodPost {
			t.Errorf("%s was answered %d, Allow %q", method, answer.Code, answer.Header().Get("Allow"))
		}
	}
}

// A revision of the protocol not spoken here is refused with 400, and every
// other path is passed on untouched.
func TestARevisionNotSpokenIsRefusedAndOtherPathsPassOn(t *testing.T) {
	h, _ := served(t)

	ping := `{"jsonrpc":"2.0","id":1,"method":"ping"}`

	if answer := post(h, ping, "MCP-Protocol-Version", "1999-01-01"); answer.Code != http.StatusBadRequest {
		t.Errorf("an unknown revision was answered %d, want 400", answer.Code)
	}

	if answer := post(h, ping, "MCP-Protocol-Version", "2025-03-26"); answer.Code != http.StatusOK {
		t.Errorf("a revision spoken here was answered %d, want 200", answer.Code)
	}

	answer := httptest.NewRecorder()
	h.ServeHTTP(answer, httptest.NewRequest(http.MethodGet, "/api/v1/timesheets", nil))

	if answer.Code != http.StatusTeapot {
		t.Errorf("another path was answered %d rather than passed on", answer.Code)
	}
}

// What is not a JSON message is refused: by its type with 415, by its body with
// JSON-RPC's own parse error, and a batch, which the protocol took out, as an
// invalid request.
func TestWhatIsNotAMessageIsRefused(t *testing.T) {
	h, _ := served(t)

	if answer := post(h, `{}`, "Content-Type", "text/plain"); answer.Code != http.StatusUnsupportedMediaType {
		t.Errorf("a text body was answered %d, want 415", answer.Code)
	}

	for body, code := range map[string]int{
		`{"jsonrpc":"2.0","id":1`:                     parseError,
		`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`:  invalidRequest,
		`{"jsonrpc":"1.0","id":1,"method":"ping"}`:    invalidRequest,
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`: invalidRequest,
		`{"jsonrpc":"2.0","id":1,"method":"nothing"}`: methodNotFound,
	} {
		if got := replied(t, post(h, body)); got.Error == nil || got.Error.Code != code {
			t.Errorf("%s was answered %+v, want the error %d", body, got.Error, code)
		}
	}
}

// A caller the API does not know is answered 401 on every message, with the
// scheme to authenticate by - not just at the first tool call.
func TestACallerWithoutAnAccountIsAnswered401(t *testing.T) {
	h, f := served(t)
	f.answer(meRoute, func() (any, error) { return nil, refused{status: http.StatusUnauthorized, code: "unauthenticated"} })

	answer := post(h, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if answer.Code != http.StatusUnauthorized || !strings.HasPrefix(answer.Header().Get("WWW-Authenticate"), "Bearer") {
		t.Errorf("a caller without an account was answered %d, WWW-Authenticate %q",
			answer.Code, answer.Header().Get("WWW-Authenticate"))
	}
}

// Every tool is listed with a schema of its arguments, which is an object, and
// a name no other tool has; one that only reads says so.
func TestEveryToolIsListedWithTheArgumentsItTakes(t *testing.T) {
	h, _ := served(t)

	var listed struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
			Annotations map[string]any `json:"annotations"`
		} `json:"tools"`
	}

	got := replied(t, post(h, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err := json.Unmarshal(got.Result, &listed); err != nil {
		t.Fatalf("tools/list answered %s: %v", got.Result, err)
	}

	if len(listed.Tools) != len(tools()) {
		t.Errorf("%d tools are listed, want %d", len(listed.Tools), len(tools()))
	}

	names := map[string]bool{}

	for _, listedTool := range listed.Tools {
		if names[listedTool.Name] {
			t.Errorf("two tools are called %s", listedTool.Name)
		}

		names[listedTool.Name] = true

		if listedTool.InputSchema["type"] != "object" {
			t.Errorf("%s takes %v, want an object", listedTool.Name, listedTool.InputSchema["type"])
		}

		changes := map[string]bool{
			"book_time": true, "correct_time_entry": true, "delete_time_entry": true,
			"start_stopwatch": true, "stop_stopwatch": true,
		}
		if listedTool.Annotations["readOnlyHint"] != !changes[listedTool.Name] {
			t.Errorf("%s says readOnlyHint %v", listedTool.Name, listedTool.Annotations["readOnlyHint"])
		}
	}
}

// Booking time is the booking route, handed the arguments under the names the
// API reads, and its answer is the tool's - as structured content and as text.
func TestBookingTimeIsTheBookingRoute(t *testing.T) {
	h, f := served(t)
	f.answer(bookRoute, func() (any, error) { return map[string]any{"id": 31, "durationHours": 1.5}, nil })

	answer := callTool(t, h, "book_time",
		`{"date":"2026-11-24","hours":1.5,"projectId":3,"description":"Review"}`)

	booked := f.to(t, bookRoute)
	if booked.body["date"] != "2026-11-24" || booked.body["durationHours"] != 1.5 ||
		booked.body["projectId"] != float64(3) || booked.body["description"] != "Review" {
		t.Errorf("the booking route was handed %v", booked.body)
	}

	if answer.IsError || answer.StructuredContent["id"] != float64(31) ||
		len(answer.Content) != 1 || !strings.Contains(answer.Content[0].Text, `"id":31`) {
		t.Errorf("the tool answered %+v", answer)
	}
}

// A refusal is the tool's answer, in the API's words with its code and status,
// so the assistant reads it and can put right what it asked for.
func TestARefusalIsTheToolsAnswer(t *testing.T) {
	h, f := served(t)
	f.answer(bookRoute, func() (any, error) {
		return nil, refused{status: http.StatusConflict, code: "projectClosedForBooking"}
	})

	answer := callTool(t, h, "book_time", `{"date":"2026-11-24","hours":1}`)

	if !answer.IsError || len(answer.Content) != 1 ||
		!strings.Contains(answer.Content[0].Text, "409, projectClosedForBooking") {
		t.Errorf("a refused booking answered %+v", answer)
	}
}

// Arguments a tool does not take, or does not have, are refused before any
// route is called.
func TestArgumentsThatDoNotFitAreRefusedBeforeAnythingIsCalled(t *testing.T) {
	h, f := served(t)

	for arguments, want := range map[string]string{
		`{"date":"2026-11-24","hours":1,"minutes":30}`: "unknown field",
		`{"date":"2026-11-24"}`:                        "needs date and hours",
		`{"date":"2026-11-24","hours":"one"}`:          "do not fit",
	} {
		answer := callTool(t, h, "book_time", arguments)

		if !answer.IsError || !strings.Contains(answer.Content[0].Text, want) {
			t.Errorf("%s answered %+v, want a refusal saying %q", arguments, answer, want)
		}
	}

	if f.called(bookRoute) {
		t.Error("the booking route was called for arguments that did not fit")
	}
}

// Correcting an entry names it in the path, and hands on only what changes.
func TestCorrectingAnEntryNamesItInThePath(t *testing.T) {
	h, f := served(t)

	callTool(t, h, "correct_time_entry", `{"id":42,"hours":2}`)

	corrected := f.to(t, correctRoute)
	if corrected.id != "42" || len(corrected.body) != 1 || corrected.body["durationHours"] != float64(2) {
		t.Errorf("the correction was handed id %q and %v", corrected.id, corrected.body)
	}
}

// A listing is handed its narrowing under the names the API reads.
func TestAListingIsHandedItsNarrowing(t *testing.T) {
	h, f := served(t)

	callTool(t, h, "list_time_entries", `{"from":"2026-11-01","search":"review","limit":5}`)

	listed := f.to(t, entriesRoute)
	if listed.query["from"] != "2026-11-01" || listed.query["q"] != "review" || listed.query["limit"] != "5" ||
		listed.query["offset"] != "" {
		t.Errorf("the listing was handed %v", listed.query)
	}
}

// The overtime is read for the caller's own account, which the endpoint learns
// from the route the screen learns it from.
func TestTheOvertimeIsTheCallersOwn(t *testing.T) {
	h, f := served(t)
	f.answer(meRoute, func() (any, error) { return map[string]any{"user": map[string]any{"id": 7}}, nil })

	callTool(t, h, "get_overtime", `{"from":"2026-11-01","to":"2026-11-24"}`)

	read := f.to(t, overtimeRoute)
	if read.id != "7" || read.query["from"] != "2026-11-01" || read.query["to"] != "2026-11-24" {
		t.Errorf("the overtime was read for %q over %v", read.id, read.query)
	}
}

// A tool nobody offers is a mistake in the request rather than a refusal.
func TestAToolNobodyOffersIsAnInvalidRequest(t *testing.T) {
	h, _ := served(t)

	got := replied(t, post(h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"drop_tables"}}`))
	if got.Error == nil || got.Error.Code != invalidParams {
		t.Errorf("an unknown tool was answered %+v", got.Error)
	}
}

// The endpoint is not made without a route one of its tools calls, so a tool
// that could only ever fail is a start-up mistake rather than a refusal later.
func TestTheEndpointIsNotMadeWithoutTheRoutesItsToolsCall(t *testing.T) {
	f := &fake{answers: map[string]answerer{}}

	var short []Route

	for _, route := range f.routes() {
		if route.key() != stopRoute {
			short = append(short, route)
		}
	}

	if _, err := Endpoint(short, Server{Logger: logging.NewMockLogger(logging.INFO)}); err == nil ||
		!strings.Contains(err.Error(), stopRoute) {
		t.Errorf("made without %s, the endpoint said %v", stopRoute, err)
	}
}

// A message with no credentials at all is told to authenticate before the CSRF
// check can call it a forgery; one carrying a token or a cookie, or bound for
// another path, is passed on to be judged by the chain.
func TestTheGateAsksForATokenOnlyWhereNothingWasPresented(t *testing.T) {
	gate := Gate("X-API-Token")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	for name, want := range map[string]int{
		"nothing":     http.StatusUnauthorized,
		"bearer":      http.StatusTeapot,
		"token":       http.StatusTeapot,
		"cookie":      http.StatusTeapot,
		"other path":  http.StatusTeapot,
		"not a write": http.StatusTeapot,
	} {
		method, path := http.MethodPost, Path

		switch name {
		case "other path":
			path = "/api/v1/timesheets"
		case "not a write":
			method = http.MethodGet
		}

		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))

		switch name {
		case "bearer":
			req.Header.Set("Authorization", "Bearer tp_x")
		case "token":
			req.Header.Set("X-API-Token", "tp_x")
		case "cookie":
			req.AddCookie(&http.Cookie{Name: "timepulse_session", Value: "x"})
		}

		answer := httptest.NewRecorder()
		gate.ServeHTTP(answer, req)

		if answer.Code != want {
			t.Errorf("with %s the gate answered %d, want %d", name, answer.Code, want)
		}

		if want == http.StatusUnauthorized && !strings.HasPrefix(answer.Header().Get("WWW-Authenticate"), "Bearer") {
			t.Errorf("the gate's 401 names no scheme: %q", answer.Header().Get("WWW-Authenticate"))
		}
	}
}
