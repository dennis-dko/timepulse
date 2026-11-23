package sqldb

import (
	"context"
	"strings"

	"github.com/dennis-dko/timepulse/internal/domain/model"
	"github.com/dennis-dko/timepulse/internal/domain/repository"
	"github.com/dennis-dko/timepulse/internal/support/apperror"
)

// AdminEventRepository keeps the administration log.
type AdminEventRepository struct {
	base
}

// NewAdminEventRepository reads and writes the log of the given database.
func NewAdminEventRepository(db DB, dialect string) *AdminEventRepository {
	return &AdminEventRepository{base{db: db, dialect: dialect}}
}

var _ repository.AdminEventRepository = (*AdminEventRepository)(nil)

// Record keeps one change.
func (r *AdminEventRepository) Record(ctx context.Context, event *model.AdminEvent) error {
	id, err := r.insert(ctx,
		"INSERT INTO admin_events (at, actor_id, action, target_id, detail) VALUES (?, ?, ?, ?, ?)",
		event.At, event.ActorID, event.Action, nullableID(event.TargetID), event.Detail)
	if err != nil {
		return apperror.Internal(err)
	}

	event.ID = id

	return nil
}

// Find reads the changes the filter keeps, newest first by id: an entry's id
// grows with the moment it was written, and a page asked for by the last id it
// showed cannot skip or repeat an entry the way one asked for by a moment can.
func (r *AdminEventRepository) Find(
	ctx context.Context,
	filter repository.AdminEventFilter,
) ([]model.AdminEvent, uint, error) {
	conditions, args := adminEventConditions(filter)

	var total uint

	row := r.db.QueryRowContext(ctx, r.rebind("SELECT COUNT(*) FROM admin_events"+where(conditions)), args...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, apperror.Internal(err)
	}

	if filter.BeforeID > 0 {
		conditions = append(conditions, "id < ?")
		args = append(args, filter.BeforeID)
	}

	direction := " DESC"
	if filter.Ascending {
		direction = " ASC"
	}

	query := "SELECT id, at, actor_id, action, target_id, detail FROM admin_events" + where(conditions) +
		" ORDER BY id" + direction

	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit, filter.Offset)
	}

	events, err := r.read(ctx, query, args)
	if err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// adminEventConditions turns a filter into what a statement is narrowed by.
//
// The text is matched as written, through likeLiteral, and folded to lower case
// on both sides, as the entry search is: a percent sign typed into the search is
// a percent sign, and on SQLite only ASCII letters are folded.
func adminEventConditions(filter repository.AdminEventFilter) ([]string, []any) {
	var (
		conditions []string
		args       []any
	)

	if len(filter.Prefixes) > 0 {
		kinds := make([]string, 0, len(filter.Prefixes))

		for _, prefix := range filter.Prefixes {
			kinds = append(kinds, "action LIKE ? ESCAPE '!'")
			args = append(args, likeLiteral(prefix)+"%")
		}

		conditions = append(conditions, "("+strings.Join(kinds, " OR ")+")")
	}

	if filter.Text != "" {
		text := "%" + likeLiteral(strings.ToLower(filter.Text)) + "%"
		matches := []string{"LOWER(action) LIKE ? ESCAPE '!'", "LOWER(detail) LIKE ? ESCAPE '!'"}
		args = append(args, text, text)

		if len(filter.People) > 0 {
			matches = append(matches,
				"actor_id IN ("+placeholders(len(filter.People))+")",
				"(action LIKE 'user.%' AND target_id IN ("+placeholders(len(filter.People))+"))")

			for range 2 {
				for _, id := range filter.People {
					args = append(args, id)
				}
			}
		}

		if len(filter.Roles) > 0 {
			matches = append(matches, "(action LIKE 'role.%' AND target_id IN ("+placeholders(len(filter.Roles))+"))")

			for _, id := range filter.Roles {
				args = append(args, id)
			}
		}

		conditions = append(conditions, "("+strings.Join(matches, " OR ")+")")
	}

	return conditions, args
}

// placeholders is n question marks for an IN list, each a parameter.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// read runs a query for whole changes and reads them.
func (r *AdminEventRepository) read(ctx context.Context, query string, args []any) ([]model.AdminEvent, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, apperror.Internal(err)
	}

	defer func() { _ = rows.Close() }()

	events := make([]model.AdminEvent, 0)

	for rows.Next() {
		var (
			event  model.AdminEvent
			at     dateTime
			target *uint
		)

		if err := rows.Scan(&event.ID, &at, &event.ActorID, &event.Action, &target, &event.Detail); err != nil {
			return nil, apperror.Internal(err)
		}

		event.At = at.Time

		if target != nil {
			event.TargetID = *target
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, apperror.Internal(err)
	}

	return events, nil
}

// nullableID is an id for a column that may name nothing: NULL for zero, which is
// what a change to the installation as a whole has as its target.
func nullableID(id uint) any {
	if id == 0 {
		return nil
	}

	return id
}
