//go:build browser

package browser

import (
	"testing"

	"github.com/chromedp/chromedp"
)

// A project's status can be changed back in its form.
//
// The table offered "complete" and then "archive" and nothing the other way, so a
// project completed or archived too soon stayed that way: the server took the
// change all along, and no screen asked for it. The form that corrects a project
// now carries its status, and offers only what the server takes from there -
// archived only beside completed, or where it already is.
func TestAProjectsStatusCanBeChangedBackInItsForm(t *testing.T) {
	t.Parallel()

	const field = `#form-project select[name="status"]`

	p := open(t)
	p.readyWorker()

	p.run("open Projects", p.click(`.tab[data-view="projects"]`),
		chromedp.WaitVisible("#form-project", chromedp.ByID))

	if p.visible(field) {
		t.Error("a project being made is offered a status, which it starts in as active")
	}

	var archived bool

	p.awaitJSON(`(async () => {
		const project = await api('/projects', { method: 'POST', body: JSON.stringify({ name: 'Old roof', startDate: todayISO() }) });
		await api('/projects/' + project.id, { method: 'PUT', body: JSON.stringify({ status: 'completed' }) });
		await api('/projects/' + project.id + '/archive', { method: 'POST' });
		await refreshAll();
		return true;
	})()`, &archived)

	p.waitForText("#table-projects tbody", "Old roof")
	p.run("open it", p.click(`#table-projects tbody button[data-action="edit"]`),
		chromedp.WaitVisible(field, chromedp.ByQuery))

	if got := p.value(field); got != "archived" {
		t.Errorf("the archived project opens on %q", got)
	}

	offered := func() string {
		var values string

		p.evalJSON(`JSON.stringify([...document.querySelectorAll('`+field+` option')].map((o) => o.value).join(' '))`, &values)

		return values
	}

	if got := offered(); got != "active completed archived" {
		t.Errorf("an archived project is offered %q", got)
	}

	p.run("make it active again", chromedp.SetValue(field, "active", chromedp.ByQuery),
		p.click("#project-submit"))
	p.waitEvaluates("the form back to making one", `String(document.querySelector('#project-cancel').hidden)`, "true")

	var status string

	p.awaitJSON(`api('/projects').then((list) => list.items.find((project) => project.name === 'Old roof').status)`, &status)

	if status != "active" {
		t.Fatalf("the project is %q after it was made active in its form", status)
	}

	if p.visible(field) {
		t.Error("the form back to making a project still offers a status")
	}

	p.run("open it again", p.click(`#table-projects tbody button[data-action="edit"]`),
		chromedp.WaitVisible(field, chromedp.ByQuery))

	// From active the server archives nothing: completed comes first.
	if got := offered(); got != "active completed" {
		t.Errorf("an active project is offered %q", got)
	}
}
