// Package listedit is the editor for a list value such as PATH: one entry per row, changes
// marked until they are saved (added green, edited amber, removed red and struck through),
// entries checked as they are shown and typed, and moved with K and J. enved opens one in place for any list variable.
package listedit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// entry is one entry and how it changed since the last save.
type entry struct {
	value   string // what will be saved
	orig    string // what was saved; "" for an entry added since
	removed bool   // marked to go, still shown until the save
}

func (e *entry) added() bool  { return e.orig == "" }
func (e *entry) edited() bool { return e.orig != "" && e.value != e.orig }

func (e *entry) change() theme.Change {
	switch {
	case e.removed:
		return theme.Removed
	case e.added():
		return theme.Added
	case e.edited():
		return theme.Edited
	}
	return theme.Unchanged
}

var (
	keyMoveUp = key.NewBinding(key.WithKeys("K", "shift+up"), key.WithHelp("K/J", "move"))
	keyMoveDn = key.NewBinding(key.WithKeys("J", "shift+down"))
	keyAppend = key.NewBinding(key.WithKeys("a"), key.WithHelp("a/i", "add after/before"))
	keyInsert = key.NewBinding(key.WithKeys("i"))
	keyEdit   = key.NewBinding(key.WithKeys("enter", "e"), key.WithHelp("enter/e", "edit"))
	keyClean  = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clean"))
	keyOpen   = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open"))
)

// The keys of a dialog and of a filter being typed, which the program's own use too.
var (
	KeyAccept      = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "keep"))
	KeyCancel      = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	KeyFilterKeep  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "keep filter"))
	KeyFilterClear = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter"))
)

// Model is one list being edited.
type Model struct {
	Title    string // the heading: "User PATH", "User › PSModulePath"
	ReadOnly bool   // a list that cannot be changed, only looked at

	kind    Kind
	exists  func(string) bool
	saved   []string
	entries []*entry

	Grid    // the table of entries, its filter and the body around it
	input   *inputBox
	status  string     // a one-off note, cleared by the next key
	history []Snapshot // the entries before each change, for u
	future  []Snapshot // the entries before each undo, for z
	edits   int        // how many changes were made, undos and redos included
}

// Snapshot is the entries at one moment, for undo.
type Snapshot struct{ entries []entry }

// Snapshot copies the entries as they are now.
func (m *Model) Snapshot() Snapshot {
	s := Snapshot{entries: make([]entry, len(m.entries))}
	for i, e := range m.entries {
		s.entries[i] = *e
	}
	return s
}

// Restore puts the entries back as they were in s.
func (m *Model) Restore(s Snapshot) {
	m.entries = make([]*entry, len(s.entries))
	for i := range s.entries {
		e := s.entries[i]
		m.entries[i] = &e
	}
	m.refresh()
}

// Edits counts the changes made so far: a holder compares it before and after a key to
// learn whether the key changed the list.
func (m *Model) Edits() int { return m.edits }

// remember keeps the entries as they are, before a change; the change ends what z could
// redo.
func (m *Model) remember() {
	m.history, m.future = append(m.history, m.Snapshot()), nil
	m.edits++
}

// step moves one state from one stack to the other: from history for undo, from future for
// redo. It says whether there was one.
func (m *Model) step(from, to *[]Snapshot) bool {
	if len(*from) == 0 {
		return false
	}
	last := (*from)[len(*from)-1]
	*from, *to = (*from)[:len(*from)-1], append(*to, m.Snapshot())
	m.edits++
	m.Restore(last)
	return true
}

// New is an editor for a list saved as saved and now current (the same, unless it was
// changed as text first). exists checks entries on disk; nil means Exists(kind).
func New(title string, kind Kind, saved, current []string, exists func(string) bool) *Model {
	if exists == nil {
		exists = Exists(kind)
	}
	m := &Model{Title: title, kind: kind, exists: exists, Grid: NewGrid("filter the entries")}
	m.Reset(saved, current)
	return m
}

// Reset starts over from a list saved as saved and now current. Entries of current found in
// saved count as kept; saved ones current lacks are shown removed, near where they were.
func (m *Model) Reset(saved, current []string) {
	m.saved, m.entries, m.history, m.future = slices.Clone(saved), nil, nil, nil
	unused := slices.Clone(saved)
	for _, v := range current {
		e := &entry{value: v}
		if i := slices.Index(unused, v); i >= 0 {
			e.orig, unused[i] = v, "\x00"
		}
		m.entries = append(m.entries, e)
	}
	for i, v := range saved {
		if unused[i] != "\x00" {
			at := min(i, len(m.entries))
			m.entries = slices.Insert(m.entries, at, &entry{value: v, orig: v, removed: true})
		}
	}
	m.refresh()
}

// Kind is what the entries are.

// Result is the list as saving would write it.
func (m *Model) Result() []string {
	var out []string
	for _, e := range m.entries {
		if !e.removed {
			out = append(out, e.value)
		}
	}
	return out
}

// Dirty says whether saving would change anything.
func (m *Model) Dirty() bool { return !slices.Equal(m.Result(), m.saved) }

// Changes counts what differs from the last save.
func (m *Model) Changes() (added, edited, removed int) {
	for _, e := range m.entries {
		switch e.change() {
		case theme.Removed:
			removed++
		case theme.Added:
			added++
		case theme.Edited:
			edited++
		}
	}
	return
}

// Pending is the number of changes, a new order counting as one.
func (m *Model) Pending() int {
	a, e, r := m.Changes()
	return a + e + r + theme.Pick(m.Reordered(), 1, 0)
}

// Reordered says whether the entries kept from the last save are in another order.
func (m *Model) Reordered() bool {
	var kept []string
	for _, e := range m.entries {
		if !e.added() && !e.removed {
			kept = append(kept, e.orig)
		}
	}
	var was []string
	for _, v := range m.saved {
		if slices.Contains(kept, v) {
			was = append(was, v)
		}
	}
	return !slices.Equal(kept, was)
}

// health is what is wrong with each entry: a problem of its own, or it repeats an earlier
// entry (dupOf is that entry's position, from 1). Removed entries are left out of both.
func (m *Model) health() (problem []string, dupOf []int) {
	problem, dupOf = make([]string, len(m.entries)), make([]int, len(m.entries))
	var live []int // positions of the entries kept
	var values []string
	for i, e := range m.entries {
		if !e.removed {
			live, values = append(live, i), append(values, e.value)
		}
	}
	p, d := m.kind.Health(values, m.exists)
	for n, i := range live {
		problem[i] = p[n]
		if d[n] > 0 {
			dupOf[i] = live[d[n]-1] + 1
		}
	}
	return problem, dupOf
}

// Clean marks every entry with a problem, and every repeat, for removal; it says how many.
func (m *Model) Clean() int {
	problem, dupOf := m.health()
	bad := func(i int) bool { return !m.entries[i].removed && (problem[i] != "" || dupOf[i] > 0) }
	n := 0
	for i := range m.entries {
		if bad(i) {
			n++
		}
	}
	if n > 0 {
		m.remember()
	}
	for i, e := range m.entries {
		if bad(i) {
			e.removed = true
		}
	}
	m.redraw()
	return n
}

// Status is the one-off note the last key left, or "".
func (m *Model) Status() string { return m.status }

// Focus says whether the editor has the keys; the cursor row is only painted while it does.
func (m *Model) Focus(on bool) {
	m.Focused = on
	m.redraw()
}

// Busy says whether a dialog or the filter is taking the keys.
func (m *Model) Busy() bool { return m.input != nil || m.Filter.Focused() }

// Resize fits the editor to a body of w by h cells.
func (m *Model) Resize(w, h int) {
	m.SetSize(w, h, m.columns)
	m.redraw()
}

func (m *Model) columns() []table.Column {
	return theme.Columns(m.W,
		table.Column{Title: "", Width: 2}, table.Column{Title: "#", Width: 3},
		table.Column{Title: m.kind.column()}, table.Column{Title: "STATUS", Width: 12})
}

// refresh recomputes which entries the table shows.
func (m *Model) refresh() {
	m.Narrow(len(m.entries), func(i int) string { return m.entries[i].value })
	m.redraw()
}

// redraw turns the shown entries into table cells; it runs after every change or cursor
// move, because the cursor mark, the status and the row colors live in the cells.
func (m *Model) redraw() {
	problem, dupOf := m.health()
	cols := m.Table.Columns()
	m.Paint(func(i int, onCursor bool) table.Row {
		e := m.entries[i]
		r := theme.NewRow(e.change(), theme.Pick(!e.removed && (problem[i] != "" || dupOf[i] > 0), theme.Problem, theme.NoNote), onCursor)
		plain := lipgloss.NewStyle()
		value, state := r.Paint(plain, e.value), r.Paint(theme.OK, "ok")
		switch {
		case e.removed:
			value, state = r.Paint(theme.Dim.Strikethrough(true), e.value), r.Paint(theme.Err, "removed")
		case dupOf[i] > 0:
			state = r.Paint(theme.Warn, fmt.Sprintf("same as #%d", dupOf[i]))
		case problem[i] != "":
			state = r.Paint(theme.Err, problem[i])
		}
		return r.Cells(cols, r.Mark(e.change(), onCursor), r.Paint(theme.Dim, fmt.Sprint(i+1)), value, state)
	})
}

// current is the entry under the cursor and its position.
func (m *Model) current() (*entry, int, bool) {
	i := m.Current()
	if i < 0 {
		return nil, -1, false
	}
	return m.entries[i], i, true
}

// Update takes a key (or the cursor's blink) and says what it asks of the holder.
func (m *Model) Update(msg tea.Msg) (tea.Cmd, frame.Event) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		if m.input != nil {
			m.input.field, cmd = m.input.field.Update(msg)
		} else {
			m.Filter, cmd = m.Filter.Update(msg)
		}
		return cmd, frame.None
	}
	m.status = ""
	switch {
	case m.input != nil:
		return m.updateInput(k), frame.None
	case m.Filter.Focused():
		return m.updateFilter(k), frame.None
	}
	return m.updateList(k)
}

// Typing a filter: the table narrows as you type; enter keeps it, esc drops it.
func (m *Model) updateFilter(msg tea.KeyPressMsg) tea.Cmd {
	cmd, changed := m.UpdateFilter(msg)
	if changed {
		m.refresh()
	}
	return cmd
}

// editable refuses changes to a list that may only be looked at.
func (m *Model) editable() bool {
	if m.ReadOnly {
		m.status = "this list cannot be changed"
		return false
	}
	return true
}

// Undo takes back the last change; false when there is none.
func (m *Model) Undo() bool { return m.step(&m.history, &m.future) }

// Redo makes the last change undone again; false when there is none.
func (m *Model) Redo() bool { return m.step(&m.future, &m.history) }

func (m *Model) updateList(msg tea.KeyPressMsg) (tea.Cmd, frame.Event) {
	e, i, ok := m.current()
	switch {
	case key.Matches(msg, frame.KeyBack): // one level up: first out of a filter, then out
		if m.Filter.Value() != "" {
			m.Filter.SetValue("")
			m.refresh()
			return nil, frame.None
		}
		return nil, frame.Back
	case key.Matches(msg, frame.KeySave):
		return nil, frame.Save
	case key.Matches(msg, frame.KeyReload):
		return nil, frame.Reload
	case key.Matches(msg, frame.KeyFilter):
		return m.Filter.Focus(), frame.None
	case key.Matches(msg, keyOpen):
		if ok && m.kind.onDisk() {
			open(winenv.Expand(e.value))
		}
		return nil, frame.None
	case key.Matches(msg, keyAppend, keyInsert):
		if m.editable() {
			return m.openAdd(i, key.Matches(msg, keyInsert)), frame.None
		}
	case key.Matches(msg, keyEdit):
		if ok && m.editable() {
			return m.openInput(i, false), frame.None
		}
	case key.Matches(msg, frame.KeyRemove):
		if ok && m.editable() {
			m.remember()
			if e.added() { // never saved: nothing to keep around
				m.entries = slices.Delete(m.entries, i, i+1)
				m.refresh()
			} else {
				e.removed = !e.removed
				m.redraw()
			}
		}
	case key.Matches(msg, keyMoveUp, keyMoveDn):
		switch {
		case !ok || !m.editable():
		case m.Filter.Value() != "":
			m.status = "clear the filter (esc) to reorder"
		default:
			to := i + theme.Pick(key.Matches(msg, keyMoveDn), 1, -1)
			if to >= 0 && to < len(m.entries) {
				m.remember()
				m.entries[i], m.entries[to] = m.entries[to], m.entries[i]
				m.Table.SetCursor(to)
				m.refresh()
			}
		}
	case key.Matches(msg, keyClean):
		if m.editable() {
			m.status = fmt.Sprintf("clean: %s marked for removal", theme.Plural(m.Clean(), "entry", "entries"))
		}
	case key.Matches(msg, frame.KeyUndo):
		if m.editable() {
			m.status = theme.Pick(m.Undo(), "undone", "nothing to undo")
		}
	case key.Matches(msg, frame.KeyRedo):
		if m.editable() {
			m.status = theme.Pick(m.Redo(), "redone", "nothing to redo")
		}
	default:
		var cmd tea.Cmd
		m.Table, cmd = m.Table.Update(msg)
		m.redraw()
		return cmd, frame.None
	}
	return nil, frame.None
}

// open shows a folder in Explorer, or a file selected in its folder.
func open(path string) {
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		exec.Command("explorer", "/select,"+path).Start()
		return
	}
	exec.Command("explorer", path).Start()
}

var windowsAbs = regexp.MustCompile(`^([A-Za-z]:[\\/]|\\\\)`)

// normalize tidies a typed entry: a file or folder becomes absolute, unless it leans on
// %VARS% (kept as typed, so they expand wherever the list is read).
func (k Kind) normalize(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || !k.onDisk() || strings.Contains(v, "%") {
		return v
	}
	if windowsAbs.MatchString(v) && !filepath.IsAbs(v) { // off Windows: nothing to resolve
		return v
	}
	if abs, err := filepath.Abs(v); err == nil {
		return abs
	}
	return v
}

// Heading is the editor's title with its counts: entries, problems, repeats, changes.
func (m *Model) Heading() string {
	problem, dupOf := m.health()
	nBad, nDup := 0, 0
	for i := range m.entries {
		if dupOf[i] > 0 {
			nDup++
		} else if problem[i] != "" {
			nBad++
		}
	}
	h := theme.Accent.Render(m.Title) + theme.Dim.Render(" · "+theme.Plural(len(m.Result()), "entry", "entries"))
	if nBad > 0 {
		h += theme.Err.Render(fmt.Sprintf(" · %d %s", nBad, theme.Pick(m.kind == Extensions, "not extensions", "missing")))
	}
	if nDup > 0 {
		h += theme.Warn.Render(" · " + theme.Plural(nDup, "duplicate", "duplicates"))
	}
	if m.Pending() > 0 {
		h += theme.Dim.Render(" · unsaved changes")
	}
	return h
}

// Body is the filter, the table, the entry under the cursor and the keys.
func (m *Model) Body() string {
	problem, dupOf := m.health()

	// The entry under the cursor: what it expands to and what is wrong with it; then how it
	// changed.
	var detail [2]string
	if e, i, ok := m.current(); ok {
		where := theme.Accent.Render(fmt.Sprintf("#%d", i+1)) + "  " + e.value
		if x := winenv.Expand(e.value); x != e.value {
			where += theme.Dim.Render("  →  " + x)
		}
		switch {
		case e.removed:
		case dupOf[i] > 0:
			where += theme.Warn.Render(fmt.Sprintf("  · same as #%d", dupOf[i]))
		case problem[i] != "":
			where += theme.Err.Render("  · " + m.kind.explain(problem[i]))
		case m.kind.onDisk():
			where += theme.OK.Render("  · "+m.kind.fine()) + theme.Dim.Render(" · o opens it")
		}
		detail[0] = where
		switch {
		case e.removed:
			detail[1] = theme.Err.Render("removed on save") + theme.Dim.Render(" · d keeps it")
		case e.added():
			detail[1] = theme.OK.Render("added")
		case e.edited():
			detail[1] = theme.Warn.Render("edited") + theme.Dim.Render(" · was  "+e.orig)
		}
	}
	return m.Layout(theme.Legend(!m.ReadOnly, "", m.kind.noted()), detail, "this list is empty · a adds an entry", m.keys()...)
}

// keys are the help under the table, in groups of related keys.
func (m *Model) keys() [][]key.Binding {
	edit := []key.Binding{keyEdit, keyAppend, frame.KeyRemove, keyMoveUp, keyClean}
	if m.kind.onDisk() {
		edit = append(edit, keyOpen)
	}
	move := []key.Binding{frame.KeyUp, frame.KeyDown, frame.KeyFilter, frame.KeyBack}
	if m.ReadOnly {
		return [][]key.Binding{move}
	}
	return [][]key.Binding{move, edit, {frame.KeyUndo, frame.KeyRedo, frame.KeySave, frame.KeyReload}}
}

// Review lists every change, for the review before saving.
func (m *Model) Review() []string {
	var lines []string
	moved, was := m.moved()
	for i, e := range m.entries {
		n := fmt.Sprintf("#%-3d ", i+1)
		switch e.change() {
		case theme.Removed:
			lines = append(lines, theme.Err.Render("  - ")+theme.Dim.Render(n)+e.value)
		case theme.Added:
			lines = append(lines, theme.OK.Render("  + ")+theme.Dim.Render(n)+e.value)
		case theme.Edited:
			lines = append(lines, theme.Warn.Render("  ~ ")+theme.Dim.Render(n)+e.orig+theme.Dim.Render("  →  ")+e.value)
		}
		if moved[i] {
			lines = append(lines, theme.Accent.Render("  ↕ ")+theme.Dim.Render(n)+e.value+theme.Dim.Render(fmt.Sprintf("  was #%d", was[i]+1)))
		}
	}
	return lines
}

// moved picks the kept entries that were moved: the fewest whose moving explains the new
// order, so moving one entry to the end names that one, not every entry it passed. was is
// where each entry stood in the saved list.
func (m *Model) moved() (moved map[int]bool, was map[int]int) {
	used := make([]bool, len(m.saved))
	var kept []int // positions in entries, in their new order
	was = map[int]int{}
	for i, e := range m.entries {
		if e.added() || e.removed {
			continue
		}
		for j, v := range m.saved {
			if !used[j] && v == e.orig {
				used[j], was[i] = true, j
				kept = append(kept, i)
				break
			}
		}
	}
	// The longest run of kept entries still in their saved order stays; the rest moved.
	// ponytail: O(n²), fine for lists of a few hundred entries.
	best, prev := make([]int, len(kept)), make([]int, len(kept))
	end := -1
	for a := range kept {
		best[a], prev[a] = 1, -1
		for b := range a {
			if was[kept[b]] < was[kept[a]] && best[b]+1 > best[a] {
				best[a], prev[a] = best[b]+1, b
			}
		}
		if end < 0 || best[a] > best[end] {
			end = a
		}
	}
	moved = map[int]bool{}
	for _, i := range kept {
		moved[i] = true
	}
	for a := end; a >= 0; a = prev[a] {
		delete(moved, kept[a])
	}
	return moved, was
}
