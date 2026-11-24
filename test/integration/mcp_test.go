//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// assistant speaks the Model Context Protocol to an instance with a token, as
// an AI assistant's client does: a POST of one message, answered with JSON.
type assistant struct {
	t     *testing.T
	url   string
	token string
}

// say sends one message and answers with the status and the reply's result,
// or its error as text.
func (s assistant) say(method, params string) (int, json.RawMessage, string) {
	s.t.Helper()

	req := rawRequest(s.t, http.MethodPost, s.url+"/api/v1/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`)
	req.Header.Set("Content-Type", "application/json")

	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp := sendPlain(s.t, req)

	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}

	if resp.Status == http.StatusOK {
		if err := json.Unmarshal(resp.Body, &reply); err != nil {
			s.t.Fatalf("%s was answered with %s: %v", method, resp.Body, err)
		}
	}

	return resp.Status, reply.Result, string(reply.Error)
}

// use calls a tool and answers with what it said and whether it was refused.
func (s assistant) use(name, arguments string) (string, bool) {
	s.t.Helper()

	status, result, failed := s.say("tools/call", `{"name":"`+name+`","arguments":`+arguments+`}`)
	if status != http.StatusOK || failed != "" {
		s.t.Fatalf("calling %s was answered %d: %s", name, status, failed)
	}

	var answer struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}

	if err := json.Unmarshal(result, &answer); err != nil || len(answer.Content) == 0 {
		s.t.Fatalf("%s answered %s: %v", name, result, err)
	}

	return answer.Content[0].Text, answer.IsError
}

// An assistant holding somebody's token books their time through the endpoint
// exactly as the API would: the booking is theirs and on the screen's list, a
// booking the API refuses is refused in the API's words with its code, a call
// without a token is answered 401, and maintenance turns the endpoint away like
// any other part of the API.
func TestAnAssistantBooksTimeThroughTheMCPEndpointAsTheAPIWould(t *testing.T) {
	t.Parallel()

	a, admin, worker := startWithWorker(t)

	var token struct {
		Secret string `json:"secret"`
	}

	worker.must(worker.api(http.MethodPost, "/me/tokens", map[string]any{"name": "assistant", "expiresInDays": 0}),
		http.StatusCreated, http.StatusOK).Data(t, &token)

	var project struct {
		ID uint `json:"id"`
	}

	worker.must(worker.api(http.MethodPost, "/projects", map[string]any{
		"name": "Website relaunch", "startDate": "2026-01-01",
	}), http.StatusCreated, http.StatusOK).Data(t, &project)

	ai := assistant{t: t, url: a.BaseURL(), token: token.Secret}

	status, result, _ := ai.say("initialize",
		`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}`)
	if status != http.StatusOK || !strings.Contains(string(result), `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("initialize was answered %d: %s", status, result)
	}

	if listed, refused := ai.use("list_projects", `{}`); refused || !strings.Contains(listed, "Website relaunch") {
		t.Errorf("list_projects answered %q, refused: %v", listed, refused)
	}

	booked, refused := ai.use("book_time",
		`{"date":"2026-11-23","hours":1.5,"projectId":`+jsonNumber(project.ID)+`,"description":"Reviewed the design"}`)
	if refused {
		t.Fatalf("book_time was refused: %s", booked)
	}

	var entries struct {
		Items []struct {
			DurationHours float64 `json:"durationHours"`
			Description   string  `json:"description"`
		} `json:"items"`
	}

	worker.must(worker.api(http.MethodGet, "/timesheets?from=2026-11-23&to=2026-11-23", nil),
		http.StatusOK).Data(t, &entries)

	if len(entries.Items) != 1 || entries.Items[0].DurationHours != 1.5 ||
		entries.Items[0].Description != "Reviewed the design" {
		t.Errorf("after the assistant booked, the worker's day holds %+v", entries.Items)
	}

	// Below the floor the API keeps for a booking: refused there, so refused here.
	if said, refused := ai.use("book_time", `{"date":"2026-11-23","hours":0.001}`); !refused ||
		!strings.Contains(said, "Refused (4") {
		t.Errorf("a booking the API refuses answered %q, refused: %v", said, refused)
	}

	if status, _, _ := (assistant{t: t, url: a.BaseURL()}).say("tools/list", `{}`); status != http.StatusUnauthorized {
		t.Errorf("a call without a token was answered %d, want 401", status)
	}

	setMaintenance(t, admin, true, "maintenance")

	if status, _, _ := ai.say("tools/list", `{}`); status != http.StatusServiceUnavailable {
		t.Errorf("during maintenance the endpoint answered %d, want 503 like the rest of the API", status)
	}
}

func jsonNumber(n uint) string {
	encoded, _ := json.Marshal(n)

	return string(encoded)
}
