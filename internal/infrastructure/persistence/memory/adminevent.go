package memory

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/dennis-dko/timepulse/internal/domain/model"
	"github.com/dennis-dko/timepulse/internal/domain/repository"
)

// AdminEventRepository is an in-memory repository.AdminEventRepository.
type AdminEventRepository struct {
	mu     sync.RWMutex
	events []model.AdminEvent
}

// NewAdminEventRepository returns an empty log.
func NewAdminEventRepository() *AdminEventRepository {
	return &AdminEventRepository{}
}

var _ repository.AdminEventRepository = (*AdminEventRepository)(nil)

func (r *AdminEventRepository) Record(_ context.Context, event *model.AdminEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	event.ID = uint(len(r.events) + 1)
	r.events = append(r.events, *event)

	return nil
}

func (r *AdminEventRepository) Find(
	_ context.Context,
	filter repository.AdminEventFilter,
) ([]model.AdminEvent, uint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var (
		page    []model.AdminEvent
		total   uint
		skipped uint
	)

	for n := range r.events {
		// Newest first unless the oldest is asked for, as the statement orders by id.
		i := len(r.events) - 1 - n
		if filter.Ascending {
			i = n
		}

		event := r.events[i]
		if !keeps(filter, event) {
			continue
		}

		total++

		switch {
		case filter.BeforeID > 0 && event.ID >= filter.BeforeID:
		case skipped < filter.Offset:
			skipped++
		case filter.Limit == 0 || uint(len(page)) < filter.Limit:
			page = append(page, event)
		}
	}

	return page, total, nil
}

// keeps says whether a filter keeps a change, as the database's statement does.
func keeps(filter repository.AdminEventFilter, event model.AdminEvent) bool {
	if len(filter.Prefixes) > 0 && !slices.ContainsFunc(filter.Prefixes, func(prefix string) bool {
		return strings.HasPrefix(event.Action, prefix)
	}) {
		return false
	}

	if filter.Text == "" {
		return true
	}

	text := strings.ToLower(filter.Text)

	return strings.Contains(strings.ToLower(event.Action), text) ||
		strings.Contains(strings.ToLower(event.Detail), text) ||
		slices.Contains(filter.People, event.ActorID) ||
		(strings.HasPrefix(event.Action, "user.") && slices.Contains(filter.People, event.TargetID)) ||
		(strings.HasPrefix(event.Action, "role.") && slices.Contains(filter.Roles, event.TargetID)) ||
		slices.Contains(filter.Actions, event.Action)
}
