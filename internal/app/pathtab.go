package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/winenv"
)

// pathTab is one scope's PATH in the list editor: pathed's screen, and enved path's.
type pathTab struct {
	scope      winenv.Scope
	st         winenv.Store
	saved      *winenv.Var // nil: the scope has no PATH yet
	err        error       // it could not be read
	needsAdmin bool        // saving it asks for admin (UAC)
	list       *listedit.Model
}

func newPathTab(s winenv.Scope, st winenv.Store, exists func(string) bool) *pathTab {
	t := &pathTab{scope: s, st: st}
	t.list = listedit.New(t.Label(), listedit.Folders, nil, nil, exists)
	t.Load()
	return t
}

func (t *pathTab) Name() string     { return string(t.scope) }
func (t *pathTab) Label() string    { return string(t.scope) + " PATH" }
func (t *pathTab) Count() string    { return fmt.Sprint(len(t.list.Result())) }
func (t *pathTab) Err() error       { return t.err }
func (t *pathTab) ReadOnly() bool   { return false }
func (t *pathTab) NeedsAdmin() bool { return t.needsAdmin }

// Load reads the PATH again, dropping every change.
func (t *pathTab) Load() {
	v, ok, err := find(t.st, t.scope, "Path")
	t.saved, t.err = nil, err
	if ok {
		t.saved = &v
	}
	t.needsAdmin = !t.st.CanWrite(t.scope)
	var entries []string
	if t.saved != nil {
		entries = winenv.Split(t.saved.Data)
	}
	t.list.ReadOnly = t.err != nil
	t.list.Reset(entries, entries)
}

func (t *pathTab) Dirty() bool        { return t.list.Dirty() }
func (t *pathTab) Pending() int       { return t.list.Pending() }
func (t *pathTab) Review() []string   { return t.list.Review() }
func (t *pathTab) Warnings() []string { return nil }

func (t *pathTab) Changes() []winenv.Change {
	if !t.Dirty() {
		return nil
	}
	return []winenv.Change{pathChange(t.scope, t.saved, t.list.Result())}
}

func (t *pathTab) Update(msg tea.Msg) (tea.Cmd, frame.Event) { return t.list.Update(msg) }

func (t *pathTab) Resize(w, h int) { t.list.Resize(w, h) }
func (t *pathTab) Focus(on bool)   { t.list.Focus(on) }
func (t *pathTab) Heading() string { return t.list.Heading() }
func (t *pathTab) Body() string    { return t.list.Body() }
func (t *pathTab) Overlay() string { return t.list.Overlay() }
func (t *pathTab) Status() string  { return t.list.Status() }

// newPathModel is the PATH editor: one tab per scope's PATH, the sidebar on scope on.
func newPathModel(st winenv.Store, exists func(string) bool, on winenv.Scope) frame.Model {
	var tabs []frame.Tab
	start := 0
	for i, s := range winenv.Scopes {
		tabs = append(tabs, newPathTab(s, st, exists))
		if s == on {
			start = i
		}
	}
	opts := frame.Options{
		SidebarWidth: cfg.SidebarWidth,
		Apply:        st.Apply,
		AfterSave:    "New windows see the change; with the pwsh wrapper (pathed init pwsh), this one does too once it exits.",
	}
	return frame.New(opts, tabs, start, false)
}
