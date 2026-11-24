package test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gofr.dev/pkg/gofr/logging"

	v1 "github.com/dennis-dko/timepulse/internal/interface/api/v1"
	"github.com/dennis-dko/timepulse/internal/interface/mcp"
)

// Every route an MCP tool calls is served by the router with the same handler,
// and none of them is a route the administration log records.
//
// The tools call handlers in-process, from a list of their own beside the
// router's. Two lists are how a tool comes to call a handler the router no
// longer serves at that path, and nothing about either list would say so. And a
// call made in-process passes round the administration log, which is middleware
// keyed on the route a request arrived at - so a tool on one of its routes would
// change how the installation is run without a trace.
func TestEveryToolCallsWhatTheRouterServesAndNothingTheLogRecords(t *testing.T) {
	router := read(t, filepath.Join("..", "internal", "interface", "api", "v1", "router.go"))
	table := read(t, filepath.Join("..", "internal", "interface", "api", "v1", "rest", "adminlog.go"))

	methods := map[string]string{"Get": "GET", "Post": "POST", "Put": "PUT", "Delete": "DELETE", "Patch": "PATCH"}

	served := map[string]string{}

	for _, match := range regexp.MustCompile(`app\.(GET|POST|PUT|DELETE|PATCH)\(base\+"([^"]*)", ([^)]+)\)`).
		FindAllStringSubmatch(router, -1) {
		served[match[1]+" /api/v1"+match[2]] = strings.TrimSpace(match[3])
	}

	_, toolRoutes, found := strings.Cut(router, "func ToolRoutes(")
	if !found {
		t.Fatal("router.go declares no ToolRoutes")
	}

	entries := regexp.MustCompile(`\{Method: http\.Method(\w+), Path: base \+ "([^"]*)", Handler: ([^}]+)\}`).
		FindAllStringSubmatch(toolRoutes, -1)

	// Read every entry or fail: a pattern that stopped matching would otherwise
	// report a clean list over routes it never looked at.
	if written := strings.Count(toolRoutes, "{Method:"); len(entries) != written || written == 0 {
		t.Fatalf("read %d of the %d routes ToolRoutes lists", len(entries), written)
	}

	logged := map[string]bool{}

	for _, match := range regexp.MustCompile(`\{http\.Method(\w+), "([^"]+)", `).FindAllStringSubmatch(table, -1) {
		logged[methods[match[1]]+" "+match[2]] = true
	}

	if len(logged) == 0 {
		t.Fatal("read no route from the administration log's table")
	}

	for _, entry := range entries {
		route, handler := methods[entry[1]]+" /api/v1"+entry[2], strings.TrimSpace(entry[3])

		switch servedBy, ok := served[route]; {
		case !ok:
			t.Errorf("a tool calls %s, which the router does not serve", route)
		case servedBy != handler:
			t.Errorf("a tool calls %s with %s, and the router serves it with %s", route, handler, servedBy)
		}

		if logged[route] {
			t.Errorf("a tool calls %s, which the administration log records and a tool's call passes round", route)
		}
	}
}

// The endpoint is made from the routes the router hands it - every route one of
// its tools calls is among them - so a start cannot stop on a tool without its
// route.
func TestTheMCPEndpointHasEveryRouteItsToolsCall(t *testing.T) {
	if _, err := mcp.Endpoint(v1.ToolRoutes(v1.Handlers{}), mcp.Server{Logger: logging.NewMockLogger(logging.INFO)}); err != nil {
		t.Errorf("the endpoint could not be made from the router's routes: %v", err)
	}
}
