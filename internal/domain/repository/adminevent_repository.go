package repository

import (
	"context"

	"github.com/dennis-dko/timepulse/internal/domain/model"
)

// AdminEventRepository keeps the administration log.
type AdminEventRepository interface {
	// Record keeps a change, filling in its id.
	Record(ctx context.Context, event *model.AdminEvent) error

	// Find reads the changes the filter keeps, newest first, and how many it
	// keeps altogether - wherever the page starts, so BeforeID, Offset and Limit
	// do not change the count.
	Find(ctx context.Context, filter AdminEventFilter) ([]model.AdminEvent, uint, error)
}

// AdminEventFilter narrows the administration log. Its zero value narrows
// nothing.
type AdminEventFilter struct {
	// Prefixes keeps the changes whose action begins with one of these - "user."
	// and "users." for the changes made to accounts. None keeps every change.
	Prefixes []string

	// Text keeps the changes whose action or detail contains it, whatever its
	// case, together with the ones made by an account in People, made to one,
	// or made to a role in Roles: the accounts and roles whose names contain
	// it, which the log keeps as ids and so cannot search itself. Empty keeps
	// every change, and People and Roles are then not asked.
	Text   string
	People []uint
	Roles  []uint

	// BeforeID keeps the changes older than this one; zero reads from the newest.
	BeforeID uint

	// Offset skips that many of the changes kept, and Limit reads at most that
	// many; a zero Limit reads them all.
	Offset uint
	Limit  uint
}
