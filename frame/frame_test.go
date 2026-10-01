package frame

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EnderWolf50/enved/winenv"
)

// fakeTab is a tab with changes to save, or none once loaded again.
type fakeTab struct {
	name    string
	changes []winenv.Change
	warn    []string
}

func (t *fakeTab) Name() string                    { return t.name }
func (t *fakeTab) Label() string                   { return t.name + " things" }
func (t *fakeTab) Count() string                   { return "1" }
func (t *fakeTab) Err() error                      { return nil }
func (t *fakeTab) Load()                           { t.changes, t.warn = nil, nil }
func (t *fakeTab) Update(tea.Msg) (tea.Cmd, Event) { return nil, None }
func (t *fakeTab) Resize(int, int)                 {}
func (t *fakeTab) Focus(bool)                      {}
func (t *fakeTab) Heading() string                 { return t.name }
func (t *fakeTab) Body() string                    { return "" }
func (t *fakeTab) Overlay() string                 { return "" }
func (t *fakeTab) Status() string                  { return "" }
func (t *fakeTab) NeedsAdmin() bool                { return false }
func (t *fakeTab) Pending() int                    { return len(t.changes) }
func (t *fakeTab) Changes() []winenv.Change        { return t.changes }
func (t *fakeTab) Review() []string                { return []string{"  ~ " + t.name + " change"} }
func (t *fakeTab) Warnings() []string              { return t.warn }

// lookTab only looks: it is not a Saver, so the frame treats it as read-only.
type lookTab struct{ fakeTab }

func (t *lookTab) Changes() {} // shadows fakeTab's: lookTab is no Saver

func change() []winenv.Change {
	return []winenv.Change{{Scope: winenv.User, Name: "A", New: &winenv.Value{Data: "1"}}}
}

func newTest(apply func([]winenv.Change) error, tabs ...Tab) Model {
	next, _ := New(Options{SidebarWidth: 30, Apply: apply}, tabs, 0, false).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model)
}

func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		msg := tea.KeyPressMsg{Text: k}
		if k == "enter" {
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		} else if k == "esc" {
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd
}

func screen(m Model) string { return ansi.Strip(m.View().Content) }

func TestSaveWritesEachDirtyTabAndKeepsFailures(t *testing.T) {
	user, machine := &fakeTab{name: "User", changes: change()}, &fakeTab{name: "Machine", changes: change()}
	calls := 0
	m := newTest(func([]winenv.Change) error {
		calls++
		if calls == 2 {
			return errors.New("declined")
		}
		return nil
	}, user, machine, &lookTab{fakeTab{name: "Process"}})

	m, _ = press(t, m, "s")
	if !m.reviewing || !strings.Contains(screen(m), "User change") || !strings.Contains(screen(m), "Machine change") {
		t.Fatalf("s did not review both tabs:\n%s", screen(m))
	}
	m, cmd := press(t, m, "enter")
	if !m.saving || cmd == nil {
		t.Fatal("enter on the review did not start saving")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if calls != 2 || m.saveErr[0] != nil || m.saveErr[1] == nil {
		t.Fatalf("%d writes, errors %v", calls, m.saveErr)
	}
	out := strings.Join(m.outcome, "\n")
	if !strings.Contains(out, "User things saved") || !strings.Contains(out, "Machine things not saved: declined") {
		t.Fatalf("outcome %q", out)
	}
	if user.changes != nil || machine.changes == nil {
		t.Fatal("the saved tab was not loaded again, or the failed one lost its changes")
	}

	m, _ = press(t, m, "esc", "q")
	if !m.confirmQuit {
		t.Fatal("quitting with a change left did not ask")
	}
}

func TestWarningsAskTwice(t *testing.T) {
	m := newTest(func([]winenv.Change) error { return nil }, &fakeTab{name: "User", changes: change(), warn: []string{"removes TEMP"}})
	m, _ = press(t, m, "s", "enter")
	if m.saving || !m.confirmSave {
		t.Fatal("the first enter did not ask again")
	}
	m, _ = press(t, m, "esc")
	if !m.reviewing || m.confirmSave {
		t.Fatal("esc on the second question left the review")
	}
	if m, cmd := press(t, m, "enter", "y"); !m.saving || cmd == nil {
		t.Fatal("y did not save")
	}
}

func TestReadOnlyTab(t *testing.T) {
	m := newTest(nil, &lookTab{fakeTab{name: "Process", changes: change()}})
	if !strings.Contains(screen(m), "read-only") {
		t.Fatalf("no read-only badge:\n%s", screen(m))
	}
	if m.anyDirty() {
		t.Fatal("a tab that cannot save counts as dirty")
	}
	if m, _ = press(t, m, "q"); m.confirmQuit {
		t.Fatal("quitting asked about a read-only tab")
	}
}
