//go:build browser

package browser

import (
	"testing"
)

// TestTheSettingsAskForWhatTheServerRequires reads which fields the database's
// and the directory's forms require, in the states where the server requires
// them and in the states where it does not.
//
// The server refuses a PostgreSQL or MySQL connection without a host, a name
// and a user, and a directory switched on without a host, a base DN and a
// filter holding %s. The forms asked for the name alone, so the rest was found
// out only on pressing Save or Test. A field the form hides must not be
// required while hidden, or the form cannot be sent at all.
func TestTheSettingsAskForWhatTheServerRequires(t *testing.T) {
	t.Parallel()

	p := open(t)
	p.readyAdmin()
	p.showView("admin")
	p.showTabOf("#form-datasource")

	var database struct {
		Server map[string]bool `json:"server"`
		File   map[string]bool `json:"file"`
	}

	// Chosen and read in one go: the screen puts the running connection's type
	// back over anything a script chose, at its next reload.
	p.evalJSON(`JSON.stringify((() => {
		const form = document.querySelector('#form-datasource');
		const read = () => Object.fromEntries(['host', 'name', 'user'].map((name) => [name, form.elements[name].required]));
		form.elements.dialect.value = 'postgres';
		syncDatasourceFields();
		const server = read();
		form.elements.dialect.value = 'sqlite';
		syncDatasourceFields();
		return { server, file: read() };
	})())`, &database)

	for _, name := range []string{"host", "name", "user"} {
		if !database.Server[name] {
			t.Errorf("a PostgreSQL connection does not require its %s, which the server refuses it without", name)
		}
	}

	if database.File["host"] || database.File["user"] {
		t.Errorf("a SQLite file requires a host or a user it has no field for on screen: %v", database.File)
	}

	p.showTabOf("#form-ldap")

	var directory struct {
		On      map[string]bool `json:"on"`
		Off     map[string]bool `json:"off"`
		Pattern string          `json:"pattern"`
	}

	p.evalJSON(`JSON.stringify((() => {
		const form = document.querySelector('#form-ldap');
		const read = () => Object.fromEntries(['host', 'baseDn', 'userFilter'].map((name) => [name, form.elements[name].required]));
		form.elements.enabled.checked = true;
		form.elements.enabled.dispatchEvent(new Event('change', { bubbles: true }));
		const on = read();
		form.elements.enabled.checked = false;
		form.elements.enabled.dispatchEvent(new Event('change', { bubbles: true }));
		return { on, off: read(), pattern: form.elements.userFilter.pattern };
	})())`, &directory)

	for _, name := range []string{"host", "baseDn", "userFilter"} {
		if !directory.On[name] {
			t.Errorf("a directory switched on does not require its %s, which the server refuses it without", name)
		}

		if directory.Off[name] {
			t.Errorf("a directory switched off requires its %s, which the server does not ask for", name)
		}
	}

	if directory.Pattern != ".*%s.*" {
		t.Errorf("the user filter is held to the pattern %q, want one asking for the %%s the server requires", directory.Pattern)
	}
}
