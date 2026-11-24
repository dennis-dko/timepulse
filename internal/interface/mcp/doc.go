// Package mcp serves the Model Context Protocol at /api/v1/mcp, so that an AI
// assistant can read and book its user's own time with that user's personal
// token.
//
// A tool is a call of an API route and nothing else. Each one is handed, in this
// process, to the handler the router serves that route with, as the caller - so
// a tool does exactly what its user could do through the API with the same
// token, is refused for the same reasons in the same words, and a rule changed
// in a handler changes for the tools without anybody having to remember them.
// What the middleware does to an API request it does to this endpoint, because
// it is one: the rate limit counts it, maintenance turns it away, and a request
// no token carries meets the CSRF check, which is also what checks its Origin.
// A token is a secret no page holds, which is what that check stands in for.
//
// Only the caller's own time. A route that changes how the installation is run
// is recorded by the administration log, which is middleware keyed on the route
// a request arrived at, and a call made from here would pass round it;
// test/mcproute_test.go keeps every tool off those routes, and holds each route
// a tool calls to the handler the router serves it with.
package mcp
