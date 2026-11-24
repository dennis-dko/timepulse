package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/container"
	gofrHTTP "gofr.dev/pkg/gofr/http"
	"gofr.dev/pkg/gofr/logging"
)

// Route is a route a tool may call: the method and path the router serves it
// at, and the handler it serves it with.
type Route struct {
	Method  string
	Path    string
	Handler gofr.Handler
}

func (r Route) key() string { return r.Method + " " + r.Path }

// apiCall is one call of a route, as a tool makes it: the route by its key,
// what fills the {name}s of its path, its query and its body.
type apiCall struct {
	route  string
	params map[string]string
	query  url.Values
	body   any
}

// refusal is how the API refused a call: its status, its sentence, and the code
// a client looks the sentence up by.
type refusal struct {
	status  int
	message string
	code    string
}

// call hands one call to its route's handler as the caller of outer, and
// answers with the data the API would have sent, as JSON, or with its refusal.
//
// In this process rather than over the network: a request to the plain port
// would be counted by the rate limit a second and third time for the one
// request the caller made, and the limit is thirty a minute for anything a
// token carries. The handler is given the request GoFr would have given it -
// the caller's context, which holds who is calling, the query, the body and the
// path's parameters - and the container it reads a logger from.
func (e *endpoint) call(outer *http.Request, c apiCall) (json.RawMessage, *refusal) {
	route := e.routes[c.route]

	target := route.Path
	for name, value := range c.params {
		target = strings.ReplaceAll(target, "{"+name+"}", url.PathEscape(value))
	}

	if len(c.query) > 0 {
		target += "?" + c.query.Encode()
	}

	var body io.Reader = http.NoBody

	if c.body != nil {
		encoded, err := json.Marshal(c.body)
		if err != nil {
			return nil, &refusal{status: http.StatusInternalServerError, message: err.Error()}
		}

		body = bytes.NewReader(encoded)
	}

	inner, err := http.NewRequestWithContext(outer.Context(), route.Method, target, body)
	if err != nil {
		return nil, &refusal{status: http.StatusInternalServerError, message: err.Error()}
	}

	inner.Header.Set("Content-Type", "application/json")
	inner.Host = outer.Host
	inner.RemoteAddr = outer.RemoteAddr
	inner = mux.SetURLVars(inner, c.params)

	data, err := route.Handler(&gofr.Context{
		Context:       inner.Context(),
		Request:       gofrHTTP.NewRequest(inner),
		Container:     e.container,
		ContextLogger: logging.ContextLoggerFor(inner.Context(), e.container.Logger),
	})
	if err != nil {
		return nil, refusalOf(err)
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, &refusal{status: http.StatusInternalServerError, message: err.Error()}
	}

	return encoded, nil
}

// refusalOf reads a handler's error the way GoFr's responder does: the status
// it names, its sentence, and the code its response carries.
func refusalOf(err error) *refusal {
	answer := &refusal{status: http.StatusInternalServerError, message: err.Error()}

	var named interface{ StatusCode() int }
	if errors.As(err, &named) {
		answer.status = named.StatusCode()
	}

	var described interface{ Response() map[string]any }
	if errors.As(err, &described) {
		if code, isText := described.Response()["code"].(string); isText {
			answer.code = code
		}
	}

	return answer
}

// newContainer is what a handler reads its logger from. Nothing else in it is
// asked by a route a tool calls: every handler here reaches the database
// through the services it was made with.
func newContainer(logger logging.Logger) *container.Container {
	return &container.Container{Logger: logger}
}
