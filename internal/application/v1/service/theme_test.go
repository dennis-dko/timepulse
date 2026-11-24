package service_test

import (
	"context"
	"testing"

	"github.com/dennis-dko/timepulse/internal/domain/model"
)

// An appearance chosen is kept, and an empty one clears it - through the store
// the service tests stand on as well as through the database, or a service case
// about the appearance would fail on the store rather than on the rule.
func TestAnAppearanceChosenIsKeptAndAnEmptyOneClearsIt(t *testing.T) {
	f, sessions := newSessionFixture(t, nil)

	for _, theme := range []string{model.ThemeDark, ""} {
		if err := sessions.SetTheme(context.Background(), f.userID, theme); err != nil {
			t.Fatalf("setting the appearance %q: %v", theme, err)
		}

		user, err := f.userRepo.GetByID(context.Background(), f.userID)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}

		if user.Theme != theme {
			t.Errorf("after choosing %q the account keeps %q", theme, user.Theme)
		}
	}
}
