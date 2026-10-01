package app

import (
	"fmt"

	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/winenv"
)

// pathTab is one scope's PATH in the list editor: pathed's screen, and enved path's.
type pathTab struct {
	scope           winenv.Scope
	st              winenv.Store
	saved           *winenv.Var // nil: the scope has no PATH yet
	err             error       // it could not be read
	needsAdmin      bool        // saving it asks for admin (UAC)
	*listedit.Model             // the rest of frame.Tab
}

func newPathTab(s winenv.Scope, st winenv.Store, exists func(string) bool) *pathTab {
	t := &pathTab{scope: s, st: st}
	t.Model = listedit.New(t.Label(), listedit.Folders, nil, nil, exists)
	t.Load()
	return t
}

func (t *pathTab) Name() string     { return string(t.scope) }
func (t *pathTab) Label() string    { return string(t.scope) + " PATH" }
func (t *pathTab) Count() string    { return fmt.Sprint(len(t.Result())) }
func (t *pathTab) Err() error       { return t.err }
func (t *pathTab) ReadOnly() bool   { return false }
func (t *pathTab) NeedsAdmin() bool { return t.needsAdmin }

// Load reads the PATH again, dropping every change.
func (t *pathTab) Load() {
	var entries []string
	t.saved, entries, t.err = readPath(t.st, t.scope)
	t.needsAdmin = !t.st.CanWrite(t.scope)
	t.Model.ReadOnly = t.err != nil
	t.Reset(entries, entries)
}

func (t *pathTab) Warnings() []string { return nil }

func (t *pathTab) Changes() []winenv.Change {
	if !t.Dirty() {
		return nil
	}
	return []winenv.Change{pathChange(t.scope, t.saved, t.Result())}
}

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
