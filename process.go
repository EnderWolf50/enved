package main

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// processTab is the environment enved started with, read-only: it answers "why doesn't my
// shell see it". A value that differs from what the saved variables give now is stale.
type processTab struct {
	environ func() []string
	scopes  []*varTab // the User and Machine tabs, for the saved values
	vars    []winenv.Var
	grid
	status string
}

func newProcessTab(scopes []*varTab) *processTab {
	t := &processTab{environ: os.Environ, scopes: scopes, grid: newGrid()}
	t.Load()
	return t
}

func (t *processTab) Name() string             { return "Process" }
func (t *processTab) Label() string            { return "This process" }
func (t *processTab) Count() string            { return "" }
func (t *processTab) Err() error               { return nil }
func (t *processTab) ReadOnly() bool           { return true }
func (t *processTab) NeedsAdmin() bool         { return false }
func (t *processTab) Dirty() bool              { return false }
func (t *processTab) Pending() int             { return 0 }
func (t *processTab) Changes() []winenv.Change { return nil }
func (t *processTab) Review() []string         { return nil }
func (t *processTab) Warnings() []string       { return nil }
func (t *processTab) Overlay() string          { return "" }
func (t *processTab) Status() string           { return t.status }
func (t *processTab) Focus(on bool)            { t.focused = on; t.redraw() }
func (t *processTab) Resize(w, h int)          { t.grid.resize(w, h, t.columns); t.redraw() }
func (t *processTab) columns() []table.Column {
	return theme.Columns(t.w,
		table.Column{Title: "", Width: 2}, table.Column{Title: "NAME", Width: min(28, max(t.w/4, 10))},
		table.Column{Title: "VALUE"}, table.Column{Title: "STATE", Width: 10})
}

// Load reads the process's environment again. (It never changes; the saved values might.)
func (t *processTab) Load() {
	t.vars = nil
	for _, kv := range t.environ() {
		name, value, _ := strings.Cut(kv, "=")
		if name == "" { // cmd.exe's hidden =C:=C:\... entries
			continue
		}
		t.vars = append(t.vars, winenv.Var{Name: name, Value: winenv.Value{Data: value}})
	}
	winenv.Sort(t.vars)
	t.refresh()
}

// saved is what a new process would get for a variable from the saved ones, expanded; ok is
// false when neither scope has it.
func (t *processTab) saved(name string) (entries []string, value string, ok bool) {
	var got [2]*winenv.Var
	for i, s := range t.scopes {
		for _, v := range s.saved {
			if strings.EqualFold(v.Name, name) {
				got[i] = &v
			}
		}
	}
	user, machine := got[0], got[1]
	if strings.EqualFold(name, "Path") { // Machine's entries, then User's
		for _, v := range []*winenv.Var{machine, user} {
			if v != nil {
				ok = true
				entries = append(entries, winenv.Split(winenv.Expand(v.Data))...)
			}
		}
		return entries, winenv.Join(entries), ok
	}
	for _, v := range []*winenv.Var{user, machine} {
		if v != nil {
			return nil, winenv.Expand(v.Data), true
		}
	}
	return nil, "", false
}

// state is "stale" when the saved variables give another value, "here only" when they do
// not have it at all. PATH is stale only when a saved entry is missing: a shell adds its own.
func (t *processTab) state(v winenv.Var) string {
	entries, value, ok := t.saved(v.Name)
	switch {
	case !ok:
		return "here only"
	case entries != nil:
		have := map[string]bool{}
		for _, e := range winenv.Split(v.Data) {
			have[strings.ToLower(strings.TrimRight(e, `\/`))] = true
		}
		for _, e := range entries {
			if !have[strings.ToLower(strings.TrimRight(e, `\/`))] {
				return "stale"
			}
		}
		return ""
	case value != v.Data:
		return "stale"
	}
	return ""
}

func (t *processTab) refresh() {
	t.narrow(len(t.vars), func(i int) string { return t.vars[i].Name + "\x00" + t.vars[i].Data })
	t.redraw()
}

func (t *processTab) redraw() {
	cols := t.table.Columns()
	t.paint(func(i int, cursor bool) table.Row {
		v := t.vars[i]
		r := theme.NewRow(theme.Unchanged, cursor)
		state := t.state(v)
		style := pick(state == "stale", theme.Warn, theme.Dim)
		return r.Cells(cols, r.Mark(theme.Unchanged, cursor), r.Paint(lipgloss.NewStyle(), v.Name),
			r.Paint(lipgloss.NewStyle(), v.Data), r.Paint(style, state))
	})
}

func (t *processTab) Update(msg tea.Msg) (tea.Cmd, frame.Event) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		t.filter, cmd = t.filter.Update(msg)
		return cmd, frame.None
	}
	t.status = ""
	if t.filter.Focused() {
		cmd, changed := t.updateFilter(k)
		if changed {
			t.refresh()
		}
		return cmd, frame.None
	}
	switch {
	case key.Matches(k, keyBack):
		if t.filter.Value() != "" {
			t.filter.SetValue("")
			t.refresh()
			return nil, frame.None
		}
		return nil, frame.Back
	case key.Matches(k, keyFilter):
		return t.filter.Focus(), frame.None
	case key.Matches(k, keyReload):
		return nil, frame.Reload
	case key.Matches(k, keySave):
		return nil, frame.Save
	case key.Matches(k, keyCopy, keyCopyName):
		if i := t.current(); i >= 0 {
			v := t.vars[i]
			if key.Matches(k, keyCopyName) {
				t.status = "copied the name " + v.Name
				return tea.SetClipboard(v.Name), frame.None
			}
			t.status = "copied the value of " + v.Name
			return tea.SetClipboard(v.Data), frame.None
		}
		return nil, frame.None
	case key.Matches(k, keyAdd, keyEdit, keyText, keyRemove, keyRename, keyType, keyList, keyUndo, keyRedo):
		t.status = "this process's environment is read-only"
		return nil, frame.None
	}
	var cmd tea.Cmd
	t.table, cmd = t.table.Update(k)
	t.redraw()
	return cmd, frame.None
}

func (t *processTab) Heading() string {
	stale := 0
	for _, v := range t.vars {
		if t.state(v) == "stale" {
			stale++
		}
	}
	h := theme.Accent.Render(t.Label()) + theme.Dim.Render(" · "+theme.Plural(len(t.vars), "variable", "variables"))
	if stale > 0 {
		h += theme.Warn.Render(" · " + theme.Plural(stale, "stale", "stale"))
	}
	return h
}

func (t *processTab) Body() string {
	width := max(t.w, 0)
	detail := make([]string, 2)
	if i := t.current(); i >= 0 {
		v := t.vars[i]
		detail[0] = theme.Accent.Render(v.Name) + theme.Dim.Render(" = ") + v.Data
		switch t.state(v) {
		case "stale":
			_, value, _ := t.saved(v.Name)
			detail[1] = theme.Warn.Render("stale: saved as ") + value + theme.Dim.Render(" · a new window gets it, or use `enved init pwsh`")
		case "here only":
			detail[1] = theme.Dim.Render("not a saved variable: Windows or the shell set it for this process")
		default:
			detail[1] = theme.Dim.Render("the same as the saved value")
		}
	}
	for i := range detail {
		detail[i] = ansi.Truncate(detail[i], width, "…")
	}
	keys := []key.Binding{keyUp, keyDown, keyFilter, keyCopy, keyCopyName, keyReload, keyBack}
	return strings.Join([]string{
		t.filterLine(),
		t.table.View(), "",
		theme.Faint.Render(strings.Repeat("─", width)),
		strings.Join(detail, "\n"),
		ansi.Truncate(t.help.ShortHelpView(keys), width, "…"),
	}, "\n")
}
