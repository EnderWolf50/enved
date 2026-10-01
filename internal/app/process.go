package app

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// processTab is the environment enved started with, read-only: it answers "why doesn't my
// shell see it". A value that differs from what the saved variables give now is stale.
type processTab struct {
	environ func() []string
	scopes  []*varTab // the User and Machine tabs, for the saved values
	vars    []winenv.Var
	listedit.Grid
	status string
}

func newProcessTab(scopes []*varTab) *processTab {
	t := &processTab{environ: os.Environ, scopes: scopes, Grid: listedit.NewGrid("filter by name or value")}
	t.Load()
	return t
}

func (t *processTab) Name() string    { return "Process" }
func (t *processTab) Label() string   { return "This process" }
func (t *processTab) Count() string   { return fmt.Sprint(len(t.vars)) }
func (t *processTab) Err() error      { return nil }
func (t *processTab) Overlay() string { return "" }
func (t *processTab) Status() string  { return t.status }
func (t *processTab) Focus(on bool)   { t.Focused = on; t.redraw() }
func (t *processTab) Resize(w, h int) { t.SetSize(w, h, t.columns); t.redraw() }
func (t *processTab) columns() []table.Column {
	return theme.Columns(t.W,
		table.Column{Title: "", Width: 2}, table.Column{Title: "NAME", Width: min(28, max(t.W/4, 10))},
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
			have[pathKey(e)] = true
		}
		for _, e := range entries {
			if !have[pathKey(e)] {
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
	t.Narrow(len(t.vars), func(i int) string { return t.vars[i].Name + "\x00" + t.vars[i].Data })
	t.redraw()
}

func (t *processTab) redraw() {
	cols := t.Table.Columns()
	t.Paint(func(i int, cursor bool) table.Row {
		v := t.vars[i]
		state := t.state(v)
		r := theme.NewRow(theme.Unchanged, theme.Pick(state == "stale", theme.Info, theme.NoNote), cursor)
		style := theme.Pick(state == "stale", theme.Warn, theme.Dim)
		return r.Cells(cols, r.Mark(theme.Unchanged, cursor), r.Paint(lipgloss.NewStyle(), v.Name),
			r.Paint(lipgloss.NewStyle(), v.Data), r.Paint(style, state))
	})
}

func (t *processTab) Update(msg tea.Msg) (tea.Cmd, frame.Event) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		t.Filter, cmd = t.Filter.Update(msg)
		return cmd, frame.None
	}
	t.status = ""
	if t.Filter.Focused() {
		cmd, changed := t.UpdateFilter(k)
		if changed {
			t.refresh()
		}
		return cmd, frame.None
	}
	switch {
	case key.Matches(k, frame.KeyBack):
		if t.Filter.Value() != "" {
			t.Filter.SetValue("")
			t.refresh()
			return nil, frame.None
		}
		return nil, frame.Back
	case key.Matches(k, frame.KeyFilter):
		return t.Filter.Focus(), frame.None
	case key.Matches(k, frame.KeyReload):
		return nil, frame.Reload
	case key.Matches(k, frame.KeySave):
		return nil, frame.Save
	case key.Matches(k, keyCopy, keyCopyName):
		if i := t.Current(); i >= 0 {
			v := t.vars[i]
			if key.Matches(k, keyCopyName) {
				t.status = "copied the name " + v.Name
				return tea.SetClipboard(v.Name), frame.None
			}
			t.status = "copied the value of " + v.Name
			return tea.SetClipboard(v.Data), frame.None
		}
		return nil, frame.None
	case key.Matches(k, keyAdd, keyEdit, keyText, frame.KeyRemove, keyRename, keyType, keyList, frame.KeyUndo, frame.KeyRedo):
		t.status = "this process's environment is read-only"
		return nil, frame.None
	}
	var cmd tea.Cmd
	t.Table, cmd = t.Table.Update(k)
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
	var detail [2]string
	if i := t.Current(); i >= 0 {
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
	return t.Layout(theme.Legend(false, "stale", ""), detail, "",
		[]key.Binding{frame.KeyUp, frame.KeyDown, frame.KeyFilter, frame.KeyBack},
		[]key.Binding{keyCopyName, keyCopy},
		[]key.Binding{frame.KeyReload})
}
