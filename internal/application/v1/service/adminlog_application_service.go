package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dennis-dko/timepulse/internal/domain/model"
	"github.com/dennis-dko/timepulse/internal/domain/repository"
	"github.com/dennis-dko/timepulse/internal/support/apperror"
)

// AdminLogService keeps and reads the administration log: who changed how the
// installation is run, and when.
//
// It keeps ids and reads names. A name kept in an entry would outlive the person
// it names once their account is erased, which is the one thing an erasure must
// not leave behind; an id names nobody once its account is gone, because no id is
// handed out twice. A role's name is not a person's and is read the same way, so
// a role renamed since appears under its new name.
type AdminLogService struct {
	events repository.AdminEventRepository
	users  repository.UserRepository
	roles  repository.RoleRepository
	now    func() time.Time
}

// NewAdminLogService needs the accounts and the roles, to name them.
func NewAdminLogService(events repository.AdminEventRepository, users repository.UserRepository,
	roles repository.RoleRepository) *AdminLogService {
	return &AdminLogService{events: events, users: users, roles: roles, now: time.Now}
}

// AdminLogEntry is a change as it is shown: its ids, and the names they carry now,
// empty for an account or a role that is gone and for a change with no target.
type AdminLogEntry struct {
	model.AdminEvent

	ActorName  string
	TargetName string
}

// Record keeps one change made by the account actorID.
func (s *AdminLogService) Record(ctx context.Context, actorID uint, action string, targetID uint, detail string) error {
	return s.events.Record(ctx, &model.AdminEvent{
		At: s.now().UTC(), ActorID: actorID, Action: action, TargetID: targetID, Detail: detail,
	})
}

// AdminLogQuery is the part of the log a page asks for. Its zero value is the
// newest changes of every kind.
type AdminLogQuery struct {
	// Text keeps the changes made by or to an account or a role whose name
	// contains it, or whose action or detail does, whatever its case.
	Text string

	// Area keeps the changes to one part of the installation: one of the keys
	// of adminLogAreas, or empty for every change.
	Area string

	// BeforeID keeps the changes older than this one; zero reads from the newest.
	BeforeID uint

	// Offset skips that many of the changes kept, and Limit is the page's size.
	Offset uint
	Limit  uint

	// Ascending reads the oldest change first, as an audit reads a period.
	Ascending bool
}

// AdminLogPage is a page of the log: its changes, how many the query keeps
// altogether, and whether older ones remain.
type AdminLogPage struct {
	Entries []AdminLogEntry
	Total   uint
	More    bool
}

// adminLogAreas sorts the changes by the part of the installation they were
// made to, by how their action begins - which is what the log's filter offers.
// Each action the log records belongs to exactly one, and test/adminlog_test.go
// holds every one of them to that.
var adminLogAreas = map[string][]string{
	"accounts":     {"user.", "users."},
	"roles":        {"role.", "roles."},
	"settings":     {"settings."},
	"installation": {"installation."},
	"directory":    {"directory."},
}

// Page reads the part of the log the query asks for, with the names its ids
// carry now. An area the log does not have is refused rather than answered with
// nothing, because an empty page would read as no change having been made.
func (s *AdminLogService) Page(ctx context.Context, query AdminLogQuery) (AdminLogPage, error) {
	filter := repository.AdminEventFilter{
		Text: query.Text, BeforeID: query.BeforeID, Offset: query.Offset, Limit: query.Limit + 1,
		Ascending: query.Ascending,
	}

	if query.Area != "" {
		prefixes, known := adminLogAreas[query.Area]
		if !known {
			return AdminLogPage{}, apperror.InvalidFields("area")
		}

		filter.Prefixes = prefixes
	}

	if query.Text != "" {
		var err error

		if filter.People, filter.Roles, err = s.named(ctx, query.Text); err != nil {
			return AdminLogPage{}, err
		}
	}

	events, total, err := s.events.Find(ctx, filter)
	if err != nil {
		return AdminLogPage{}, err
	}

	more := uint(len(events)) > query.Limit
	if more {
		events = events[:query.Limit]
	}

	names := namer{users: s.users, roles: s.roles, known: map[string]string{}}
	entries := make([]AdminLogEntry, 0, len(events))

	for _, event := range events {
		entry := AdminLogEntry{AdminEvent: event}

		if entry.ActorName, err = names.user(ctx, event.ActorID); err != nil {
			return AdminLogPage{}, err
		}

		switch {
		case event.TargetID == 0:
		case strings.HasPrefix(event.Action, "user."):
			entry.TargetName, err = names.user(ctx, event.TargetID)
		case strings.HasPrefix(event.Action, "role."):
			entry.TargetName, err = names.role(ctx, event.TargetID)
		}

		if err != nil {
			return AdminLogPage{}, err
		}

		entries = append(entries, entry)
	}

	return AdminLogPage{Entries: entries, Total: total, More: more}, nil
}

// named finds the accounts and the roles whose names contain text, whatever its
// case, for the log to find the changes made by and to them: it keeps ids, so a
// name can only be searched where the name is kept. Folded here rather than in
// the database, so an umlaut finds its capital on every dialect.
func (s *AdminLogService) named(ctx context.Context, text string) (people, roles []uint, err error) {
	text = strings.ToLower(text)

	users, err := s.users.GetAll(ctx)
	if err != nil {
		return nil, nil, err
	}

	for _, user := range users {
		if strings.Contains(strings.ToLower(user.Name), text) {
			people = append(people, user.ID)
		}
	}

	all, err := s.roles.GetAll(ctx)
	if err != nil {
		return nil, nil, err
	}

	for _, role := range all {
		if strings.Contains(strings.ToLower(role.Name), text) {
			roles = append(roles, role.ID)
		}
	}

	return people, roles, nil
}

// namer reads the names a page of the log needs, each once.
type namer struct {
	users repository.UserRepository
	roles repository.RoleRepository
	known map[string]string
}

func (n namer) user(ctx context.Context, id uint) (string, error) {
	return n.read("user", id, func() (string, error) {
		user, err := n.users.GetByID(ctx, id)
		if err != nil {
			return "", err
		}

		return user.Name, nil
	})
}

func (n namer) role(ctx context.Context, id uint) (string, error) {
	return n.read("role", id, func() (string, error) {
		role, err := n.roles.GetByID(ctx, id)
		if err != nil {
			return "", err
		}

		return role.Name, nil
	})
}

// read answers from what this page has already read, and an account or a role
// that is gone with no name rather than an error: being gone is what the log says
// about it.
func (n namer) read(kind string, id uint, lookUp func() (string, error)) (string, error) {
	key := kind + ":" + strconv.FormatUint(uint64(id), 10)
	if name, found := n.known[key]; found {
		return name, nil
	}

	name, err := lookUp()
	if err != nil {
		if detail, ok := apperror.Detail(err); !ok || detail.Kind != apperror.KindNotFound {
			return "", err
		}

		name = ""
	}

	n.known[key] = name

	return name, nil
}
