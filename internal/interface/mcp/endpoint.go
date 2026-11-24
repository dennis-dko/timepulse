package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"

	"gofr.dev/pkg/gofr/container"
	"gofr.dev/pkg/gofr/logging"
)

// Path is where the endpoint answers: under the API, so that the middleware
// treats it as the API request it is.
const Path = "/api/v1/mcp"

// protocolVersions are the revisions of the protocol spoken here, the newest
// first. All of them call a tool alike; what the later ones add - a tool's
// title, structured content - an older client passes over.
var protocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

// instructions is what a client is told about the server as a whole, which an
// assistant reads before it reads any one tool.
const instructions = "TimePulse keeps your own working time. A day is a calendar day written " +
	"YYYY-MM-DD, read in your own timezone; hours are decimal, so 1.5 is an hour and a half. " +
	"A booking names its project by the id list_projects gives, and only an active project " +
	"takes one. Correcting an entry keeps the version it replaced; deleting one does not, so " +
	"ask before deleting."

// Server is how the endpoint introduces itself, and the logger the handlers
// it calls are given.
type Server struct {
	Version string
	Logger  logging.Logger
}

// endpoint answers the protocol, calling routes for its tools.
type endpoint struct {
	server    Server
	routes    map[string]Route
	tools     []tool
	container *container.Container
}

// Endpoint answers the protocol at Path, with routes as the handlers its tools
// call. It is not made when a tool's route is missing from routes, because a
// tool that could only ever answer that its route is unknown is a mistake made
// at start-up, and the place to say so.
func Endpoint(routes []Route, server Server) (func(http.Handler) http.Handler, error) {
	e := &endpoint{
		server:    server,
		routes:    make(map[string]Route, len(routes)),
		tools:     tools(),
		container: newContainer(server.Logger),
	}

	for _, route := range routes {
		e.routes[route.key()] = route
	}

	for _, needed := range append(routesOf(e.tools), meRoute) {
		if _, ok := e.routes[needed]; !ok {
			return nil, fmt.Errorf("the MCP endpoint has no route %s to call", needed)
		}
	}

	return e.middleware, nil
}

// Gate answers 401 to a message carrying no credentials at all - no token under
// either header and no cookie - before the CSRF check would answer it 403 as a
// forgery from another site. A program that forgot its token is told to
// authenticate, which is what it needs to hear; anything carrying a token or a
// cookie passes on, to be judged by the chain like every other API request.
// tokenHeader is the second header a token may travel in besides Authorization.
func Gate(tokenHeader string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == Path && r.Method == http.MethodPost && r.Header.Get("Authorization") == "" &&
				r.Header.Get(tokenHeader) == "" && len(r.Cookies()) == 0 {
				unauthenticated(w)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func unauthenticated(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="TimePulse"`)
	http.Error(w, "a personal token of this installation is needed", http.StatusUnauthorized)
}

func (e *endpoint) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != Path {
			next.ServeHTTP(w, r)

			return
		}

		e.serve(w, r)
	})
}

// serve answers one message, over the transport the protocol calls streamable
// HTTP: a message is POSTed, a request is answered with its response as JSON,
// and a notification with 202 and nothing.
func (e *endpoint) serve(w http.ResponseWriter, r *http.Request) {
	// Nothing is sent from here unasked, so there is no stream to open with a
	// GET, and nothing is kept between messages, so there is no session to end
	// with a DELETE. The protocol answers both with 405.
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "the protocol is spoken here with POST", http.StatusMethodNotAllowed)

		return
	}

	if asked := r.Header.Get("MCP-Protocol-Version"); asked != "" && !slices.Contains(protocolVersions, asked) {
		http.Error(w, "protocol version "+asked+" is not spoken here", http.StatusBadRequest)

		return
	}

	if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || media != "application/json" {
		http.Error(w, "a message is sent as application/json", http.StatusUnsupportedMediaType)

		return
	}

	// Who is calling, asked of the route the screen asks it of: a token that
	// does not open an account is answered 401 before anything is read, on
	// every message, as the protocol wants - not just at the first tool call.
	if _, refused := e.call(r, apiCall{route: meRoute}); refused != nil && refused.status == http.StatusUnauthorized {
		unauthenticated(w)

		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "the message could not be read", http.StatusBadRequest)

		return
	}

	// Batches were taken out of the protocol in its revision of 2025-06-18.
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '[' {
		e.reply(w, nil, nil, &rpcError{Code: invalidRequest, Message: "a batch of messages is not taken"})

		return
	}

	var msg message
	if err := json.Unmarshal(raw, &msg); err != nil {
		e.reply(w, nil, nil, &rpcError{Code: parseError, Message: "the message is not JSON: " + err.Error()})

		return
	}

	switch {
	case msg.JSONRPC != "2.0":
		e.reply(w, msg.ID, nil, &rpcError{Code: invalidRequest, Message: `jsonrpc must be "2.0"`})
	case msg.Method == "" || len(msg.ID) == 0:
		// A notification, or the answer to a request this endpoint never sends:
		// taken, and nothing to say back.
		w.WriteHeader(http.StatusAccepted)
	case string(msg.ID) == "null":
		e.reply(w, nil, nil, &rpcError{Code: invalidRequest, Message: "a request's id may not be null"})
	default:
		result, failed := e.answer(r, msg)
		e.reply(w, msg.ID, result, failed)
	}
}

// answer works out the result of one request.
func (e *endpoint) answer(r *http.Request, msg message) (any, *rpcError) {
	switch msg.Method {
	case "initialize":
		return e.initialize(msg.Params)
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		return map[string]any{"tools": describe(e.tools)}, nil
	case "tools/call":
		return e.callTool(r, msg.Params)
	default:
		return nil, &rpcError{Code: methodNotFound, Message: "no method " + msg.Method}
	}
}

// initialize agrees on a revision of the protocol - the client's own where it
// is spoken here, otherwise the newest that is - and says what is offered.
func (e *endpoint) initialize(params json.RawMessage) (any, *rpcError) {
	var asked struct {
		ProtocolVersion string `json:"protocolVersion"`
	}

	if err := json.Unmarshal(params, &asked); err != nil {
		return nil, &rpcError{Code: invalidParams, Message: "initialize takes an object: " + err.Error()}
	}

	version := protocolVersions[0]
	if slices.Contains(protocolVersions, asked.ProtocolVersion) {
		version = asked.ProtocolVersion
	}

	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "timepulse", "title": "TimePulse", "version": e.server.Version},
		"instructions":    instructions,
	}, nil
}

// callTool runs one tool. A refusal is the tool's answer rather than a failure
// of the protocol, so the assistant reads it - in the API's words, with the
// code beside them - and can put right what it asked for.
func (e *endpoint) callTool(r *http.Request, params json.RawMessage) (any, *rpcError) {
	var asked struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal(params, &asked); err != nil {
		return nil, &rpcError{Code: invalidParams, Message: "tools/call takes an object: " + err.Error()}
	}

	i := slices.IndexFunc(e.tools, func(candidate tool) bool { return candidate.name == asked.Name })
	if i < 0 {
		return nil, &rpcError{Code: invalidParams, Message: "no tool " + asked.Name}
	}

	data, refused := e.tools[i].run(e, r, asked.Arguments)
	if refused != nil {
		return toolResult{
			Content: []textContent{{Type: "text", Text: refused.String()}},
			IsError: true,
		}, nil
	}

	return succeeded(data), nil
}

// succeeded is a tool's answer: the API's data both as structured content,
// which has to be an object, and as text, for a client that reads only text.
func succeeded(data json.RawMessage) toolResult {
	text := string(data)
	if text == "" || text == "null" {
		return toolResult{Content: []textContent{{Type: "text", Text: "Done."}}}
	}

	var structured any = data
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] != '{' {
		structured = map[string]json.RawMessage{"result": data}
	}

	return toolResult{Content: []textContent{{Type: "text", Text: text}}, StructuredContent: structured}
}

func (e *endpoint) reply(w http.ResponseWriter, id json.RawMessage, result any, failed *rpcError) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}

	w.Header().Set("Content-Type", "application/json")

	// Best effort: the client has gone if this fails, and there is nobody left
	// to tell.
	_ = json.NewEncoder(w).Encode(reply{JSONRPC: "2.0", ID: id, Result: result, Error: failed})
}

// String is a refusal as the assistant reads it.
func (r *refusal) String() string {
	if r.code == "" {
		return fmt.Sprintf("Refused (%d): %s", r.status, r.message)
	}

	return fmt.Sprintf("Refused (%d, %s): %s", r.status, r.code, r.message)
}

// message is one JSON-RPC message as it arrives: a request when it carries an
// id and a method, a notification when it carries a method and no id.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC's own codes for a message it could not take.
const (
	parseError     = -32700
	invalidRequest = -32600
	methodNotFound = -32601
	invalidParams  = -32602
)

type toolResult struct {
	Content           []textContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
