// Package listedit is the editor for a list value such as PATH: one entry per row, changes
// marked until they are saved (added green, edited amber, removed red and struck through),
// entries checked as they are shown and typed, and moved with K and J. pathed shows one per
// PATH; enved opens one in place for any list variable.
package listedit

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// Event is what a key asks of whatever holds the editor.
type Event int

const (
	None   Event = iota
	Back         // leave the editor
	Save         // review and save the changes
	Reload       // read the value again
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
	keyUp     = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	keyDown   = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	keyMoveUp = key.NewBinding(key.WithKeys("K", "shift+up"), key.WithHelp("K/J", "move"))
	keyMoveDn = key.NewBinding(key.WithKeys("J", "shift+down"))
	keyAppend = key.NewBinding(key.WithKeys("a"), key.WithHelp("a/i", "add after/before"))
	keyInsert = key.NewBinding(key.WithKeys("i"))
	keyEdit   = key.NewBinding(key.WithKeys("enter", "e"), key.WithHelp("enter/e", "edit"))
	keyRemove = key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "remove"))
	keyClean  = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clean"))
	keyUndo   = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo"))
	keyRedo   = key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "redo"))
	keyOpen   = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open"))
	keyFilter = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	keyReload = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload"))
	keySave   = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save"))
	keyBack   = key.NewBinding(key.WithKeys("left", "h", "esc", "q"), key.WithHelp("←/h/esc/q", "back"))
)

// Model is one list being edited.
type Model struct {
	Title    string // the heading: "User PATH", "User › PSModulePath"
	ReadOnly bool   // a list that cannot be changed, only looked at
	// ExtraKeys are the holder's own keys that work in the editor, for its help line.
	ExtraKeys []key.Binding

	kind    Kind
	exists  func(string) bool
	saved   []string
	entries []*entry

	shown   []int // positions in entries that the table shows, narrowed by the filter
	table   table.Model
	filter  textinput.Model
	help    help.Model
	input   *inputBox
	focused bool
	w, h    int        // the body's size
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
	m := &Model{Title: title, kind: kind, exists: exists, filter: textinput.New(), help: help.New(), table: theme.NewTable()}
	m.filter.Prompt = "/ "
	m.filter.Placeholder = "filter the entries"
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
func (m *Model) Kind() Kind { return m.kind }

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
	return a + e + r + pick(m.Reordered(), 1, 0)
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
	first := map[string]int{}
	for i, e := range m.entries {
		if e.removed {
			continue
		}
		problem[i] = m.kind.problem(e.value, m.exists)
		k := m.kind.key(e.value)
		if j, seen := first[k]; seen {
			dupOf[i] = j + 1
		} else {
			first[k] = i
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
	m.focused = on
	m.redraw()
}

// Busy says whether a dialog or the filter is taking the keys.
func (m *Model) Busy() bool { return m.input != nil || m.filter.Focused() }

// Lines of the body around the table: the filter above; a blank, the divider, two detail
// lines and the help below.
const chrome = 6

// Resize fits the editor to a body of w by h cells.
func (m *Model) Resize(w, h int) {
	m.w, m.h = w, h
	m.table.SetColumns(m.columns())
	m.table.SetWidth(w)
	m.table.SetHeight(h - chrome)
	m.filter.SetWidth(w - 2)
	m.help.SetWidth(w)
	m.redraw()
}

func (m *Model) columns() []table.Column {
	return theme.Columns(m.w,
		table.Column{Title: "", Width: 2}, table.Column{Title: "#", Width: 3},
		table.Column{Title: m.kind.column()}, table.Column{Title: "STATUS", Width: 12})
}

// refresh recomputes which entries the table shows.
func (m *Model) refresh() {
	query := strings.ToLower(m.filter.Value())
	m.shown = nil
	for i, e := range m.entries {
		if strings.Contains(strings.ToLower(e.value), query) {
			m.shown = append(m.shown, i)
		}
	}
	m.redraw()
}

// redraw turns the shown entries into table cells; it runs after every change or cursor
// move, because the cursor mark, the status and the row colors live in the cells.
func (m *Model) redraw() {
	problem, dupOf := m.health()
	cursor := min(m.table.Cursor(), max(len(m.shown)-1, 0))
	cols := m.table.Columns()
	if len(cols) < 4 {
		return // not sized yet
	}
	cells := make([]table.Row, len(m.shown))
	for row, i := range m.shown {
		e := m.entries[i]
		onCursor := row == cursor && m.focused
		r := theme.NewRow(e.change(), !e.removed && (problem[i] != "" || dupOf[i] > 0), onCursor)
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
		cells[row] = r.Cells(cols, r.Mark(e.change(), onCursor), r.Paint(theme.Dim, fmt.Sprint(i+1)), value, state)
	}
	m.table.SetRows(cells)
	m.table.SetCursor(cursor)
}

// current is the entry under the cursor and its position.
func (m *Model) current() (*entry, int, bool) {
	c := m.table.Cursor()
	if c < 0 || c >= len(m.shown) {
		return nil, -1, false
	}
	i := m.shown[c]
	return m.entries[i], i, true
}

// Update takes a key (or the cursor's blink) and says what it asks of the holder.
func (m *Model) Update(msg tea.Msg) (tea.Cmd, Event) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		if m.input != nil {
			m.input.field, cmd = m.input.field.Update(msg)
		} else {
			m.filter, cmd = m.filter.Update(msg)
		}
		return cmd, None
	}
	m.status = ""
	switch {
	case m.input != nil:
		return m.updateInput(k), None
	case m.filter.Focused():
		return m.updateFilter(k), None
	}
	return m.updateList(k)
}

// Typing a filter: the table narrows as you type; enter keeps it, esc drops it.
func (m *Model) updateFilter(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		m.filter.Blur()
		return nil
	case "esc":
		m.filter.Blur()
		m.filter.SetValue("")
		m.refresh()
		return nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.table.SetCursor(0)
	m.refresh()
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

func (m *Model) updateList(msg tea.KeyPressMsg) (tea.Cmd, Event) {
	e, i, ok := m.current()
	switch {
	case key.Matches(msg, keyBack): // one level up: first out of a filter, then out
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.refresh()
			return nil, None
		}
		return nil, Back
	case key.Matches(msg, keySave):
		return nil, Save
	case key.Matches(msg, keyReload):
		return nil, Reload
	case key.Matches(msg, keyFilter):
		return m.filter.Focus(), None
	case key.Matches(msg, keyOpen):
		if ok && m.kind.onDisk() {
			open(winenv.Expand(e.value))
		}
		return nil, None
	case key.Matches(msg, keyAppend, keyInsert):
		if m.editable() {
			return m.openAdd(i, key.Matches(msg, keyInsert)), None
		}
	case key.Matches(msg, keyEdit):
		if ok && m.editable() {
			return m.openInput(i, false), None
		}
	case key.Matches(msg, keyRemove):
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
		case m.filter.Value() != "":
			m.status = "clear the filter (esc) to reorder"
		default:
			to := i + pick(key.Matches(msg, keyMoveDn), 1, -1)
			if to >= 0 && to < len(m.entries) {
				m.remember()
				m.entries[i], m.entries[to] = m.entries[to], m.entries[i]
				m.table.SetCursor(to)
				m.refresh()
			}
		}
	case key.Matches(msg, keyClean):
		if m.editable() {
			m.status = fmt.Sprintf("clean: %s marked for removal", theme.Plural(m.Clean(), "entry", "entries"))
		}
	case key.Matches(msg, keyUndo):
		if m.editable() {
			m.status = pick(m.Undo(), "undone", "nothing to undo")
		}
	case key.Matches(msg, keyRedo):
		if m.editable() {
			m.status = pick(m.Redo(), "redone", "nothing to redo")
		}
	default:
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		m.redraw()
		return cmd, None
	}
	return nil, None
}

// open shows a folder in Explorer, or a file selected in its folder.
func open(path string) {
	if fi, err := statDir(path); err == nil && !fi {
		exec.Command("explorer", "/select,"+path).Start()
		return
	}
	exec.Command("explorer", path).Start()
}

func pick[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
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
		h += theme.Err.Render(fmt.Sprintf(" · %d %s", nBad, pick(m.kind == Extensions, "not extensions", "missing")))
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
	width := max(m.w, 0)
	problem, dupOf := m.health()

	filter := ""
	if m.filter.Focused() || m.filter.Value() != "" {
		filter = m.filter.View()
	}

	// The entry under the cursor: what it expands to and what is wrong with it; then how it
	// changed.
	detail := make([]string, 2)
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
	for i := range detail {
		detail[i] = ansi.Truncate(detail[i], width, "…")
	}

	body := m.table.View()
	if len(m.shown) == 0 {
		note := "this list is empty · a adds an entry"
		if m.filter.Value() != "" {
			note = "nothing matches the filter · esc clears it"
		}
		body = lipgloss.Place(width, lipgloss.Height(body), lipgloss.Center, lipgloss.Center, theme.Dim.Render(note))
	}

	keys := append([]key.Binding{keyUp, keyDown, keyEdit, keyAppend, keyRemove, keyMoveUp, keySave, keyBack}, m.ExtraKeys...)
	keys = append(keys, keyClean, keyUndo, keyRedo, keyFilter)
	if m.kind.onDisk() {
		keys = append(keys, keyOpen)
	}
	keys = append(keys, keyReload)
	if m.ReadOnly {
		keys = []key.Binding{keyUp, keyDown, keyFilter, keyBack}
	}
	if m.filter.Focused() {
		keys = []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "keep filter")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
		}
	}
	return strings.Join([]string{
		filter,
		body, "",
		theme.Divider(width, theme.Legend(!m.ReadOnly, m.kind.noted())),
		strings.Join(detail, "\n"),
		ansi.Truncate(m.help.ShortHelpView(keys), width, "…"),
	}, "\n")
}

// Review lists every change, for the review before saving.
func (m *Model) Review() []string {
	var lines []string
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
	}
	if m.Reordered() {
		lines = append(lines, theme.Dim.Render("  ↕ the order changes"))
	}
	return lines
}
