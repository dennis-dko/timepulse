package memory

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dennis-dko/timepulse/internal/domain/model"
	"github.com/dennis-dko/timepulse/internal/domain/repository"
)

// TimesheetRepository is an in-memory repository.TimesheetRepository.
type TimesheetRepository struct {
	store *store[model.Timesheet]

	// revisions is every entry's history, under its own lock: the store's guards
	// the entries, and a correction reads one before the history is written.
	mu           sync.Mutex
	revisions    []model.TimesheetRevision
	nextRevision uint
}

// NewTimesheetRepository creates an empty in-memory timesheet repository.
func NewTimesheetRepository() *TimesheetRepository {
	return &TimesheetRepository{store: newStore[model.Timesheet]("timesheet")}
}

var _ repository.TimesheetRepository = (*TimesheetRepository)(nil)

// Save stamps the recording moment the same way the SQL repository does.
//
// Not a detail the in-memory store may skip: a service test that checks an entry
// knows when it was booked would pass here and fail against a database, or the
// reverse, and either way the suite would be proving something about a store
// nobody deploys.
func (r *TimesheetRepository) Save(_ context.Context, timesheet *model.Timesheet) (*model.Timesheet, error) {
	// Truncated the way the SQL repository truncates, and for its reason: a store
	// whose stamps are finer than any database's would let a test pass here that
	// cannot pass against PostgreSQL.
	now := time.Now().UTC().Truncate(time.Second)

	return r.store.add(timesheet, func(t *model.Timesheet, id uint) {
		t.ID = id
		t.CreatedAt, t.UpdatedAt = now, now
	}), nil
}

func (r *TimesheetRepository) GetByID(_ context.Context, id uint) (*model.Timesheet, error) {
	return r.store.get(id)
}

func (r *TimesheetRepository) GetAll(_ context.Context) ([]*model.Timesheet, error) {
	return r.store.all(), nil
}

func (r *TimesheetRepository) GetByFilter(
	_ context.Context,
	filter repository.TimesheetFilter,
) ([]*model.Timesheet, error) {
	// The same narrowing the SQL repository applies, for the same reason: a range
	// is a range of days. Two repositories that disagree about which entries a
	// month contains would be worse than either answer alone.
	filter = filter.OverWholeDays()

	out := make([]*model.Timesheet, 0)

	for _, ts := range r.store.all() {
		if !matchesFilter(ts, filter) {
			continue
		}

		out = append(out, ts)
	}

	// The same order the SQL repository gives, because paging is defined in terms
	// of it: by the column asked for, then the day and then the id in the same
	// direction - newest day first when nothing was asked. The in-memory store
	// hands rows back in insertion order, which agrees with neither.
	sort.SliceStable(out, func(i, j int) bool {
		before := compareEntries(out[i], out[j], filter.Order.By)
		if filter.Order.Ascending {
			return before < 0
		}

		return before > 0
	})

	return page(out, filter.Limit, filter.Offset), nil
}

// CountByFilter counts the matching entries, ignoring Limit and Offset.
func (r *TimesheetRepository) CountByFilter(
	_ context.Context,
	filter repository.TimesheetFilter,
) (uint, error) {
	filter = filter.OverWholeDays()

	var total uint

	for _, ts := range r.store.all() {
		if matchesFilter(ts, filter) {
			total++
		}
	}

	return total, nil
}

// HoursByProject totals somebody's hours on each project they booked on.
func (r *TimesheetRepository) HoursByProject(_ context.Context, userID uint) (map[uint]float64, error) {
	totals := map[uint]float64{}

	for _, ts := range r.store.all() {
		if ts.ProjectID != nil && (userID == 0 || ts.UserID == userID) {
			totals[*ts.ProjectID] += ts.DurationHours
		}
	}

	return totals, nil
}

func (r *TimesheetRepository) FirstDay(_ context.Context, userID uint) (time.Time, bool, error) {
	var first time.Time

	for _, ts := range r.store.all() {
		if ts.UserID == userID && (first.IsZero() || ts.Date.Before(first)) {
			first = ts.Date
		}
	}

	return model.CalendarDay(first), !first.IsZero(), nil
}

// page applies a limit and an offset the way a SQL LIMIT/OFFSET would, including
// answering with nothing for an offset past the end rather than with the last
// page, which is what a hand-rolled slice usually gets wrong.
func page[T any](all []T, limit, offset uint) []T {
	if limit == 0 {
		return all
	}

	if offset >= uint(len(all)) {
		return all[:0]
	}

	end := min(offset+limit, uint(len(all)))

	return all[offset:end]
}

// compareEntries orders two entries by a column and then by day and id, as the
// SQL repository does: negative when a comes first in ascending order.
func compareEntries(a, b *model.Timesheet, by repository.TimesheetColumn) int {
	switch by {
	case repository.ByHours:
		if a.DurationHours != b.DurationHours {
			if a.DurationHours < b.DurationHours {
				return -1
			}

			return 1
		}
	case repository.ByDescription:
		if order := strings.Compare(strings.ToLower(described(a)), strings.ToLower(described(b))); order != 0 {
			return order
		}
	}

	if !a.Date.Equal(b.Date) {
		return a.Date.Compare(b.Date)
	}

	switch {
	case a.ID < b.ID:
		return -1
	case a.ID > b.ID:
		return 1
	default:
		return 0
	}
}

func described(ts *model.Timesheet) string {
	if ts.Description == nil {
		return ""
	}

	return *ts.Description
}

func matchesFilter(ts *model.Timesheet, filter repository.TimesheetFilter) bool {
	switch {
	case filter.UserID != 0 && ts.UserID != filter.UserID:
		return false
	case filter.ProjectID != 0 && (!ts.HasProject() || *ts.ProjectID != filter.ProjectID):
		return false
	case filter.WithoutProject && ts.HasProject():
		return false
	case filter.StartDate != nil && ts.Date.Before(*filter.StartDate):
		return false
	case filter.EndDate != nil && ts.Date.After(*filter.EndDate):
		return false
	case filter.Search != "" && (ts.Description == nil ||
		!strings.Contains(strings.ToLower(*ts.Description), strings.ToLower(filter.Search))):
		return false
	default:
		return true
	}
}

// Update moves the correction moment and keeps the recording one.
//
// The caller hands over a whole entry, and the one it was given may have travelled
// through a DTO that never carried created_at - so the stored value is read back
// rather than trusted, which is what the SQL repository gets for free by leaving
// the column out of its UPDATE.
func (r *TimesheetRepository) Update(_ context.Context, timesheet *model.Timesheet) (*model.Timesheet, error) {
	corrected := *timesheet
	corrected.UpdatedAt = time.Now().UTC().Truncate(time.Second)

	before, err := r.store.get(timesheet.ID)
	if err == nil {
		corrected.CreatedAt, corrected.RecordedBy = before.CreatedAt, before.RecordedBy

		// As the SQL repository does: a save that changes nothing leaves the name
		// of whoever last did as it was.
		if before.Says(timesheet) {
			corrected.UpdatedBy = before.UpdatedBy
		}
	}

	updated, err := r.store.update(timesheet.ID, &corrected)
	if err != nil {
		return nil, err
	}

	if !before.Says(timesheet) {
		r.mu.Lock()
		r.nextRevision++

		kept := model.RevisionOf(before, corrected.UpdatedAt)
		kept.ID = r.nextRevision
		kept.ReplacedBy = timesheet.UpdatedBy
		r.revisions = append(r.revisions, kept)
		r.mu.Unlock()
	}

	return updated, nil
}

func (r *TimesheetRepository) Delete(_ context.Context, id uint) error {
	if err := r.store.delete(id); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.revisions = slices.DeleteFunc(r.revisions, func(revision model.TimesheetRevision) bool {
		return revision.TimesheetID == id
	})

	return nil
}

func (r *TimesheetRepository) Revisions(_ context.Context, timesheetID uint) ([]model.TimesheetRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var history []model.TimesheetRevision

	for _, revision := range r.revisions {
		if revision.TimesheetID == timesheetID {
			history = append(history, revision)
		}
	}

	// The most recently replaced first, as the SQL repository's ORDER BY returns
	// them - by when, and by which was kept last for two in the same second.
	slices.SortStableFunc(history, func(a, b model.TimesheetRevision) int {
		if order := b.ReplacedAt.Compare(a.ReplacedAt); order != 0 {
			return order
		}

		return int(b.ID) - int(a.ID)
	})

	return history, nil
}

// SaveMany writes several entries.
//
// Not atomic, and it cannot be: this store has no transaction to roll back to.
// The guarantee belongs to the SQL repository, and the integration tests are where
// it is checked - a unit test against this would be checking nothing.
func (r *TimesheetRepository) SaveMany(ctx context.Context, entries []*model.Timesheet) error {
	for _, entry := range entries {
		if _, err := r.Save(ctx, entry); err != nil {
			return err
		}
	}

	return nil
}
