//go:build browser

package browser

import (
	"strings"
	"testing"

	"github.com/dennis-dko/timepulse/internal/interface/mcp"
)

// The token card says how an AI assistant connects: the address this installation
// answers the Model Context Protocol at, the header a token travels in, and Claude
// Code's command with both - written from the page's own address, so it is right
// wherever the installation runs, and from the route the server registers.
//
// Shown to whoever has time of their own to read and book, which is all the
// assistant's tools reach; the built-in administrator has none, and is not told
// how to connect something every tool would refuse.
func TestTheTokenCardSaysHowAnAssistantConnects(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyAdmin()
	p.showView("settings")
	p.showTabOf("#token-card")

	if p.visible("#token-assistant") {
		t.Error("the built-in administrator is told how to connect an assistant that reaches no time of theirs")
	}

	p.becomeWorker()
	p.showView("settings")
	p.showTabOf("#token-card")

	if !p.visible("#token-assistant") {
		t.Fatal("the token card says nothing about connecting an assistant")
	}

	var origin string

	p.evalJSON(`JSON.stringify(location.origin)`, &origin)

	address := origin + mcp.Path

	if got := p.text("#token-assistant-address"); got != address {
		t.Errorf("the card names %q as the assistant's address, want %q", got, address)
	}

	if got := p.text("#token-assistant-header"); got != "Authorization: Bearer <token>" {
		t.Errorf("the card gives the header as %q", got)
	}

	command := p.text("#token-assistant-command")

	for _, part := range []string{"claude mcp add --transport http timepulse " + address, `--header "Authorization: Bearer <token>"`} {
		if !strings.Contains(command, part) {
			t.Errorf("Claude Code's command reads %q, want it to carry %q", command, part)
		}
	}
}
