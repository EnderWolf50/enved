package app

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

var (
	keyUp       = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	keyDown     = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	keyAdd      = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add"))
	keyEdit     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit"))
	keyText     = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit as text"))
	keyRename   = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename"))
	keyRemove   = key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "remove"))
	keyType     = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "%expand"))
	keyList     = key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "list on/off"))
	keyCopy     = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "copy value"))
	keyCopyName = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "copy name"))
	keyUndo     = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo"))
	keyRedo     = key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "redo"))
	keyFilter   = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	keyReload   = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload"))
	keySave     = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save"))
	keyBack     = key.NewBinding(key.WithKeys("left", "h", "esc", "q"), key.WithHelp("←/h/esc/q", "back"))
)

// varTab is one scope's variables: the table, and the list editor of one of them when open.
type varTab struct {
	scope      winenv.Scope
	st         winenv.Store
	prefs      *listPrefs
	peers      []*varTab // every scope's tab, this one included: who uses a variable
	saved      []winenv.Var
	vars       []*variable
	err        error // it could not be read
	needsAdmin bool  // saving it asks for admin (UAC)

	grid
	open    *variable // the variable whose list editor is showing
	dialog  *varDialog
	status  string
	history [][]varState // the variables before each change, for u
	future  [][]varState // the variables before each undo, for z
}

// varState is one variable at one moment, for undo: its fields, and its list's entries.
type varState struct {
	v    *variable
	was  variable
	list listedit.Snapshot
}

func (t *varTab) snapshot() []varState {
	out := make([]varState, len(t.vars))
	for i, v := range t.vars {
		out[i] = varState{v: v, was: *v}
		if v.list != nil {
			out[i].list = v.list.Snapshot()
		}
	}
	return out
}

// remember keeps the variables as they are, before a change; the change ends what z could
// redo.
func (t *varTab) remember() { t.history, t.future = append(t.history, t.snapshot()), nil }

// step moves one state from one stack to the other: from history for undo, from future for
// redo. It says whether there was one. One history covers the table and every list editor.
func (t *varTab) step(from, to *[][]varState) bool {
	if len(*from) == 0 {
		return false
	}
	last := (*from)[len(*from)-1]
	*from, *to = (*from)[:len(*from)-1], append(*to, t.snapshot())
	t.vars = make([]*variable, len(last))
	for i, s := range last {
		*s.v = s.was
		if s.v.list != nil {
			s.v.list.Restore(s.list)
		}
		t.vars[i] = s.v
	}
	switch {
	case t.open == nil:
	case t.open.list == nil || !slices.Contains(t.vars, t.open):
		t.open = nil // the state came from before the list editor was opened
	default:
		t.open.list.Resize(t.w, t.h)
		t.open.list.Focus(true)
	}
	t.refresh()
	return true
}

// history1 is u or z.
func (t *varTab) history1(k tea.KeyPressMsg) {
	switch {
	case key.Matches(k, keyUndo):
		t.undo()
	default:
		t.redo()
	}
}

func (t *varTab) undo() { t.status = pick(t.step(&t.history, &t.future), "undone", "nothing to undo") }
func (t *varTab) redo() { t.status = pick(t.step(&t.future, &t.history), "redone", "nothing to redo") }

func newVarTab(s winenv.Scope, st winenv.Store, prefs *listPrefs) *varTab {
	t := &varTab{scope: s, st: st, prefs: prefs, grid: newGrid()}
	t.Load()
	return t
}

func (t *varTab) Name() string     { return string(t.scope) }
func (t *varTab) Label() string    { return string(t.scope) + " variables" }
func (t *varTab) Err() error       { return t.err }
func (t *varTab) NeedsAdmin() bool { return t.needsAdmin }

func (t *varTab) Count() string {
	n := 0
	for _, v := range t.vars {
		if !v.removed {
			n++
		}
	}
	return fmt.Sprint(n)
}

// Load reads the scope again, dropping every change.
func (t *varTab) Load() {
	t.saved, t.err = t.st.ReadAll(t.scope)
	t.needsAdmin = !t.st.CanWrite(t.scope)
	t.history, t.future = nil, nil
	t.reset()
}

// reset drops every change, back to what was read last.
func (t *varTab) reset() {
	t.vars, t.open, t.dialog = nil, nil, nil
	for _, s := range t.saved {
		orig := s
		t.vars = append(t.vars, &variable{name: s.Name, value: s.Value, orig: &orig})
	}
	t.refresh()
}

// ---- the table --------------------------------------------------------------------------

func (t *varTab) columns() []table.Column {
	return theme.Columns(t.w,
		table.Column{Title: "", Width: 2}, table.Column{Title: "NAME", Width: min(28, max(t.w/4, 10))},
		table.Column{Title: "VALUE"}, table.Column{Title: "%", Width: 1}, table.Column{Title: "KIND", Width: 8})
}

func (t *varTab) Resize(w, h int) {
	t.grid.resize(w, h, t.columns)
	for _, v := range t.vars {
		if v.list != nil {
			v.list.Resize(w, h)
		}
	}
	t.redraw()
}

func (t *varTab) Focus(on bool) {
	t.focused = on
	if t.open != nil {
		t.open.list.Focus(on)
	}
	t.redraw()
}

func (t *varTab) refresh() {
	t.narrow(len(t.vars), func(i int) string { return t.vars[i].name + "\x00" + t.vars[i].current().Data })
	t.redraw()
}

// shownValue is a value on one line: a list shows its first entry and how many there are.
func (t *varTab) shownValue(v *variable) string {
	cur := v.current().Data
	if _, isList := t.prefs.kind(v.name); isList {
		if entries := winenv.Split(cur); len(entries) > 1 {
			return fmt.Sprintf("%s; … (%d)", entries[0], len(entries))
		}
	}
	return cur
}

func (t *varTab) redraw() {
	cols := t.table.Columns()
	t.paint(func(i int, cursor bool) table.Row {
		v := t.vars[i]
		r := theme.NewRow(v.change(), pick(isSystem(v.name), theme.Info, theme.NoNote), cursor)
		plain := lipgloss.NewStyle()
		name, value := r.Paint(plain, v.name), r.Paint(plain, t.shownValue(v))
		if v.removed {
			name, value = r.Paint(theme.Dim.Strikethrough(true), v.name), r.Paint(theme.Dim.Strikethrough(true), t.shownValue(v))
		}
		var kind []string
		if _, isList := t.prefs.kind(v.name); isList {
			kind = append(kind, "list")
		}
		if isSystem(v.name) {
			kind = append(kind, "sys")
		}
		return r.Cells(cols, r.Mark(v.change(), cursor), name, value,
			r.Paint(theme.Accent, pick(v.current().Expands(), "%", " ")), r.Paint(theme.Warn, strings.Join(kind, " ")))
	})
}

func (t *varTab) current() (*variable, int) {
	i := t.grid.current()
	if i < 0 {
		return nil, -1
	}
	return t.vars[i], i
}

// point puts the cursor on a variable, clearing a filter that hides it.
func (t *varTab) point(v *variable) {
	i := slices.Index(t.vars, v)
	if i < 0 {
		return
	}
	if !slices.Contains(t.shown, i) {
		t.filter.SetValue("")
		t.refresh()
	}
	t.grid.point(i)
	t.redraw()
}

// find is the variable named name (any case) that is not removed, or nil.
func (t *varTab) find(name string, skip *variable) *variable {
	for _, v := range t.vars {
		if v != skip && !v.removed && strings.EqualFold(v.name, name) {
			return v
		}
	}
	return nil
}

func (t *varTab) Status() string {
	if t.open != nil && t.open.list.Status() != "" {
		return t.open.list.Status()
	}
	return t.status
}

func (t *varTab) editable() bool {
	if t.err != nil {
		t.status = "these variables could not be read"
		return false
	}
	return true
}

func (t *varTab) Update(msg tea.Msg) (tea.Cmd, frame.Event) {
	if t.open != nil {
		// u, U and z in the list editor work on the scope's history, which covers the table
		// and every list.
		if k, ok := msg.(tea.KeyPressMsg); ok && key.Matches(k, keyUndo, keyRedo) && !t.open.list.Busy() {
			t.status = ""
			t.history1(k)
			return nil, frame.None
		}
		list, edits, before := t.open.list, t.open.list.Edits(), t.snapshot()
		cmd, ev := list.Update(msg)
		if list.Edits() != edits {
			t.history = append(t.history, before)
		}
		if ev == frame.Back {
			t.closeList()
			return cmd, frame.None
		}
		return cmd, ev
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		if t.dialog != nil {
			cmd = t.dialog.update(msg)
		} else {
			t.filter, cmd = t.filter.Update(msg)
		}
		return cmd, frame.None
	}
	t.status = ""
	switch {
	case t.dialog != nil:
		return t.updateDialog(k), frame.None
	case t.filter.Focused():
		cmd, changed := t.updateFilter(k)
		if changed {
			t.refresh()
		}
		return cmd, frame.None
	}
	return t.updateTable(k)
}

func (t *varTab) updateTable(msg tea.KeyPressMsg) (tea.Cmd, frame.Event) {
	v, _ := t.current()
	switch {
	case key.Matches(msg, keyBack):
		if t.filter.Value() != "" {
			t.filter.SetValue("")
			t.refresh()
			return nil, frame.None
		}
		return nil, frame.Back
	case key.Matches(msg, keySave):
		return nil, frame.Save
	case key.Matches(msg, keyReload):
		return nil, frame.Reload
	case key.Matches(msg, keyFilter):
		return t.filter.Focus(), frame.None
	case key.Matches(msg, keyCopy):
		if v != nil {
			t.status = "copied the value of " + v.name
			return tea.SetClipboard(v.current().Data), frame.None
		}
	case key.Matches(msg, keyCopyName):
		if v != nil {
			t.status = "copied the name " + v.name
			return tea.SetClipboard(v.name), frame.None
		}
	case key.Matches(msg, keyAdd):
		if t.editable() {
			return t.openDialog(dialogAdd, nil), frame.None
		}
	case key.Matches(msg, keyEdit, keyText):
		if v != nil && t.editable() {
			if v.removed {
				t.status = "removed: d keeps it first"
				return nil, frame.None
			}
			if kind, isList := t.prefs.kind(v.name); isList && key.Matches(msg, keyEdit) {
				return t.openList(v, kind), frame.None
			}
			return t.openDialog(dialogValue, v), frame.None
		}
	case key.Matches(msg, keyRename):
		if v != nil && !v.removed && t.editable() {
			return t.openDialog(dialogName, v), frame.None
		}
	case key.Matches(msg, keyRemove):
		if v != nil && t.editable() {
			if v.removed && t.find(v.name, v) != nil {
				t.status = "another variable is named " + v.name + " now"
				return nil, frame.None
			}
			t.remember()
			if v.orig == nil { // never saved: nothing to keep around
				t.vars = slices.DeleteFunc(t.vars, func(w *variable) bool { return w == v })
				t.refresh()
				return nil, frame.None
			}
			v.removed = !v.removed
			t.redraw()
		}
	case key.Matches(msg, keyType):
		if v != nil && !v.removed && t.editable() {
			t.remember()
			v.fold()
			v.typeSet = true
			v.value.Type = pick(v.value.Expands(), winenv.SZ, winenv.ExpandSZ)
			if !v.value.Expands() && strings.Contains(v.value.Data, "%") {
				t.status = "REG_SZ: its %VARS% stay as they are"
			}
			t.redraw()
		}
	case key.Matches(msg, keyList):
		if v != nil {
			t.toggleList(v)
		}
	case key.Matches(msg, keyUndo, keyRedo):
		t.history1(msg)
	default:
		var cmd tea.Cmd
		t.table, cmd = t.table.Update(msg)
		t.redraw()
		return cmd, frame.None
	}
	return nil, frame.None
}

// toggleList marks a variable as a list or as text, and keeps the choice: it says which
// editor enter opens. Like x, it opens nothing.
func (t *varTab) toggleList(v *variable) {
	if err := t.prefs.toggle(v.name); err != nil {
		t.status = "could not keep the choice: " + err.Error()
		return
	}
	if _, isList := t.prefs.kind(v.name); isList {
		t.status = v.name + " is a list now: enter edits its entries"
	} else {
		v.fold()
		t.status = v.name + " is text now: enter edits it as one value"
	}
	t.redraw()
}

// openList shows the list editor for a variable, keeping the changes made in it before.
func (t *varTab) openList(v *variable, kind listedit.Kind) tea.Cmd {
	title := string(t.scope) + " › " + v.name
	if v.list == nil || v.listKind != kind {
		var saved []string
		if v.orig != nil {
			saved = winenv.Split(v.orig.Data)
		}
		v.list, v.listKind = listedit.New(title, kind, saved, winenv.Split(v.current().Data), nil), kind
	}
	v.list.Title = title
	v.list.Resize(t.w, t.h)
	v.list.Focus(true)
	t.open = v
	return nil
}

func (t *varTab) closeList() {
	t.open.list.Focus(false)
	t.open = nil
	t.refresh()
}

// OpenVariable puts the cursor on a variable and, if it is a list, opens its editor; for
// `enved edit NAME`.
func (t *varTab) OpenVariable(name string) bool {
	v := t.find(name, nil)
	if v == nil {
		return false
	}
	t.point(v)
	if kind, isList := t.prefs.kind(v.name); isList {
		t.openList(v, kind)
	}
	return true
}

// ---- drawing ----------------------------------------------------------------------------

func (t *varTab) Heading() string {
	if t.open != nil {
		return t.open.list.Heading()
	}
	h := theme.Accent.Render(t.Label()) + theme.Dim.Render(" · "+theme.Plural(len(t.vars), "variable", "variables"))
	if n := t.Pending(); n > 0 {
		h += theme.Dim.Render(" · " + theme.Plural(n, "change", "changes"))
	}
	return h
}

func (t *varTab) Overlay() string {
	if t.open != nil {
		return t.open.list.Overlay()
	}
	if t.dialog != nil {
		return t.dialog.view(t)
	}
	return ""
}

func (t *varTab) Body() string {
	if t.open != nil {
		return t.open.list.Body()
	}
	width := max(t.w, 0)
	detail := make([]string, 2)
	if v, _ := t.current(); v != nil {
		cur := v.current()
		line := theme.Accent.Render(v.name) + theme.Dim.Render(" = ") + cur.Data
		if x := winenv.Expand(cur.Data); cur.Expands() && x != cur.Data {
			line += theme.Dim.Render("  →  " + x)
		}
		detail[0] = line

		facts := []string{typeName(cur)}
		if kind, isList := t.prefs.kind(v.name); isList {
			facts = append(facts, fmt.Sprintf("a list of %s (%s)", theme.Plural(len(winenv.Split(cur.Data)), "entry", "entries"),
				kind))
		}
		if isSystem(v.name) {
			facts = append(facts, theme.Warn.Render("Windows relies on it"))
		}
		if users := t.usedBy(v.name, v); len(users) > 0 {
			facts = append(facts, "used by "+strings.Join(users, ", "))
		}
		switch v.change() {
		case theme.Removed:
			facts = append(facts, theme.Err.Render("removed on save")+theme.Dim.Render(" · d keeps it"))
		case theme.Added:
			facts = append(facts, theme.OK.Render("added"))
		case theme.Edited:
			was := "edited"
			if v.renamed() {
				was += " · was named " + v.orig.Name
			}
			facts = append(facts, theme.Warn.Render(was))
		}
		detail[1] = theme.Dim.Render(strings.Join(facts, " · "))
	}
	for i := range detail {
		detail[i] = ansi.Truncate(detail[i], width, "…")
	}

	help := t.footer(
		[]key.Binding{keyUp, keyDown, keyFilter, keyBack},
		[]key.Binding{keyEdit, keyText, keyAdd, keyRename, keyRemove},
		[]key.Binding{keyType, keyList},
		[]key.Binding{keyCopyName, keyCopy},
		[]key.Binding{keyUndo, keyRedo, keySave, keyReload})
	body := t.table.View()
	if len(t.shown) == 0 {
		note := "no variables · a adds one"
		if t.filter.Value() != "" {
			note = "nothing matches the filter · esc clears it"
		}
		body = lipgloss.Place(width, lipgloss.Height(body), lipgloss.Center, lipgloss.Center, theme.Dim.Render(note))
	}
	return strings.Join([]string{
		t.filterLine(),
		body, "",
		theme.Divider(width, theme.Legend(true, "system", "")),
		strings.Join(detail, "\n"),
		help,
	}, "\n")
}

func pick[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
