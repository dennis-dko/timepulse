package service_test

import (
	"context"
	"slices"
	"testing"

	"github.com/dennis-dko/timepulse/internal/application/v1/command"
	"github.com/dennis-dko/timepulse/internal/application/v1/service"
	"github.com/dennis-dko/timepulse/internal/domain/model"
	"github.com/dennis-dko/timepulse/internal/infrastructure/persistence/memory"
	"github.com/dennis-dko/timepulse/internal/support/apperror"
)

// The log names whoever made a change and whom it was made to as they are called
// now: an account renamed since appears under its new name, and an erased one
// under none at all, though the entry stays - and a role is named by its role. It
// is read newest first, a page at a time, and says when there is more.
func TestTheAdministrationLogNamesPeopleAsTheyAreNow(t *testing.T) {
	f := newFixture(t)
	log := service.NewAdminLogService(memory.NewAdminEventRepository(), f.userRepo, f.roleRepo)

	admin, err := f.users.CreateUser(context.Background(), command.CreateUserCommand{
		Name: "Ada", Email: "ada@example.com", Role: model.RoleUserAdmin,
	})
	if err != nil {
		t.Fatalf("seed the administrator: %v", err)
	}

	gone, err := f.users.CreateUser(context.Background(), command.CreateUserCommand{
		Name: "Gunter", Email: "gunter@example.com", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("seed the account to erase: %v", err)
	}

	role, err := f.roleRepo.GetByName(context.Background(), model.RoleUser)
	if err != nil {
		t.Fatalf("read the ordinary role: %v", err)
	}

	ctx := context.Background()

	for _, change := range []struct {
		action string
		target uint
		detail string
	}{
		{"user.created", gone.Result.ID, ""},
		{"role.updated", role.ID, ""},
		{"user.role", f.userID, model.RoleUserAdmin},
		{"settings.branding", 0, ""},
	} {
		if err := log.Record(ctx, admin.Result.ID, change.action, change.target, change.detail); err != nil {
			t.Fatalf("record %s: %v", change.action, err)
		}
	}

	if err := f.userRepo.Delete(ctx, gone.Result.ID); err != nil {
		t.Fatalf("erase the account: %v", err)
	}

	first, err := log.Page(ctx, service.AdminLogQuery{Limit: 3})
	if err != nil {
		t.Fatalf("read the log: %v", err)
	}

	newest := first.Entries

	if len(newest) != 3 || !first.More || first.Total != 4 {
		t.Fatalf("the first page holds %d entries, more=%v and a total of %d, want 3, more and 4",
			len(newest), first.More, first.Total)
	}

	if first := newest[0]; first.Action != "settings.branding" || first.ActorName != "Ada" || first.TargetName != "" {
		t.Errorf("the newest entry reads %+v, want Ada changing the branding", first)
	}

	if assigned := newest[1]; assigned.TargetName != "Dennis" || assigned.Detail != model.RoleUserAdmin {
		t.Errorf("the role assignment reads %+v, want Dennis given %s", assigned, model.RoleUserAdmin)
	}

	if changed := newest[2]; changed.TargetName != model.RoleUser {
		t.Errorf("the role change reads %+v, want the role named", changed)
	}

	next, err := log.Page(ctx, service.AdminLogQuery{BeforeID: newest[2].ID, Limit: 3})
	if err != nil {
		t.Fatalf("read the next page: %v", err)
	}

	older := next.Entries

	if len(older) != 1 || next.More {
		t.Fatalf("the next page holds %d entries and more=%v, want the last one and no more", len(older), next.More)
	}

	if erased := older[0]; erased.Action != "user.created" || erased.TargetID != gone.Result.ID || erased.TargetName != "" {
		t.Errorf("the creation of the erased account reads %+v, want its id kept and no name", erased)
	}
}

// The log is searched by the names of the people and roles a change was made by
// and to, and by its action and detail, and narrowed to the part of the
// installation it was made to - over every change it holds rather than over a
// page, and counted, so a screen can say how many pages there are.
func TestTheAdministrationLogIsSearchedAndNarrowedToAnArea(t *testing.T) {
	f := newFixture(t)
	log := service.NewAdminLogService(memory.NewAdminEventRepository(), f.userRepo, f.roleRepo)
	ctx := context.Background()

	admin, err := f.users.CreateUser(ctx, command.CreateUserCommand{
		Name: "Ada", Email: "ada@example.com", Role: model.RoleUserAdmin,
	})
	if err != nil {
		t.Fatalf("seed the administrator: %v", err)
	}

	greta, err := f.users.CreateUser(ctx, command.CreateUserCommand{
		Name: "Greta Grün", Email: "greta@example.com", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("seed the account changed: %v", err)
	}

	role, err := f.roleRepo.GetByName(ctx, model.RoleUser)
	if err != nil {
		t.Fatalf("read the ordinary role: %v", err)
	}

	// Oldest first, as they were made.
	for _, change := range []struct {
		action string
		target uint
		detail string
	}{
		{"user.created", greta.Result.ID, ""},
		{"role.updated", role.ID, ""},
		{"settings.branding", 0, ""},
		{"settings.maintenance", 0, "true"},
		{"user.role", f.userID, model.RoleUserAdmin},
		{"directory.synchronised", 0, ""},
	} {
		if err := log.Record(ctx, admin.Result.ID, change.action, change.target, change.detail); err != nil {
			t.Fatalf("record %s: %v", change.action, err)
		}
	}

	actions := func(query service.AdminLogQuery) ([]string, uint) {
		t.Helper()

		query.Limit = max(query.Limit, 50)

		page, err := log.Page(ctx, query)
		if err != nil {
			t.Fatalf("read the log for %+v: %v", query, err)
		}

		var names []string
		for _, entry := range page.Entries {
			names = append(names, entry.Action)
		}

		return names, page.Total
	}

	for _, ask := range []struct {
		what  string
		query service.AdminLogQuery
		want  []string
	}{
		{"the settings", service.AdminLogQuery{Area: "settings"},
			[]string{"settings.maintenance", "settings.branding"}},
		{"the accounts", service.AdminLogQuery{Area: "accounts"}, []string{"user.role", "user.created"}},
		{"the roles", service.AdminLogQuery{Area: "roles"}, []string{"role.updated"}},
		{"the directory", service.AdminLogQuery{Area: "directory"}, []string{"directory.synchronised"}},
		// A name in capitals, umlaut and all, finds the change made to her.
		{"the account changed", service.AdminLogQuery{Text: "GRÜN"}, []string{"user.created"}},
		// And the name of the role changed, the change made to it.
		{"the role changed", service.AdminLogQuery{Text: role.Name, Area: "roles"}, []string{"role.updated"}},
		// The one who made them finds every change.
		{"the administrator", service.AdminLogQuery{Text: "ada"}, []string{
			"directory.synchronised", "user.role", "settings.maintenance", "settings.branding",
			"role.updated", "user.created",
		}},
		{"an action", service.AdminLogQuery{Text: "Maintenance"}, []string{"settings.maintenance"}},
		{"a detail", service.AdminLogQuery{Text: model.RoleUserAdmin, Area: "accounts"}, []string{"user.role"}},
		{"nothing there", service.AdminLogQuery{Text: "nobody by that name"}, nil},
	} {
		got, total := actions(ask.query)

		if !slices.Equal(got, ask.want) || total != uint(len(ask.want)) {
			t.Errorf("searching %s found %q with a total of %d, want %q", ask.what, got, total, ask.want)
		}
	}

	// A page from the middle counts every change, and knows there are older ones.
	middle, err := log.Page(ctx, service.AdminLogQuery{Offset: 2, Limit: 2})
	if err != nil {
		t.Fatalf("read a page from the middle: %v", err)
	}

	if len(middle.Entries) != 2 || middle.Entries[0].Action != "settings.maintenance" ||
		middle.Total != 6 || !middle.More {
		t.Errorf("the second page of two reads %+v with a total of %d and more=%v, "+
			"want the maintenance and the branding change, 6 and more", middle.Entries, middle.Total, middle.More)
	}

	// An area the log does not have is refused, not answered with nothing.
	_, err = log.Page(ctx, service.AdminLogQuery{Area: "nowhere", Limit: 10})
	requireKind(t, err, apperror.KindInvalid)
}
