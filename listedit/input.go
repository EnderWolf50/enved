package listedit

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// inputBox asks for an entry, over the list, checking it as it is typed.
type inputBox struct {
	field  textinput.Model
	at     int // the entry being edited, or where a new entry goes
	adding bool
}

// openAdd asks for a new entry next to the one under the cursor (i, -1 for none): after it
// (a, append) or before it (i, insert).
func (m *Model) openAdd(i int, before bool) tea.Cmd {
	return m.openInput(theme.Pick(before, max(i, 0), i+1), true)
}

func (m *Model) openInput(at int, adding bool) tea.Cmd {
	f := textinput.New()
	f.Prompt = "› "
	f.Placeholder = m.kind.placeholder()
	f.SetWidth(min(70, max(m.W-16, 20)))
	if !adding {
		f.SetValue(m.entries[at].value)
		f.CursorEnd()
	}
	m.input = &inputBox{field: f, at: at, adding: adding}
	return m.input.field.Focus()
}

// Overlay is the box drawn over the editor, or "" when none is open.
func (m *Model) Overlay() string {
	b := m.input
	if b == nil {
		return ""
	}
	title := fmt.Sprintf("Add to %s as #%d", m.Title, b.at+1)
	if !b.adding {
		title = fmt.Sprintf("Edit #%d of %s", b.at+1, m.Title)
	}
	v := m.kind.normalize(b.field.Value())
	check := theme.Dim.Render("type " + m.kind.placeholder())
	if v != "" {
		check = theme.OK.Render(theme.Pick(m.kind.fine() != "", m.kind.fine(), "ok"))
		if p := m.kind.problem(v, m.exists); p != "" {
			check = theme.Err.Render(m.kind.explain(p) + " (it can still be added)")
		}
		for i, e := range m.entries {
			if !e.removed && (b.adding || i != b.at) && m.kind.Key(e.value) == m.kind.Key(v) {
				check = theme.Warn.Render(fmt.Sprintf("already listed as #%d", i+1))
				break
			}
		}
		if x := winenv.Expand(v); x != v {
			check = theme.Dim.Render("→ "+x) + "\n" + check
		}
	}
	return theme.Dialog.Render(theme.Accent.Render(title) + "\n\n" + b.field.View() + "\n\n" + check +
		"\n\n" + m.Help.ShortHelpView([]key.Binding{KeyAccept, KeyCancel}))
}

// The input box: only its own keys count while it is open.
func (m *Model) updateInput(msg tea.KeyPressMsg) tea.Cmd {
	b := m.input
	switch {
	case key.Matches(msg, KeyCancel):
		m.input = nil
		return nil
	case key.Matches(msg, KeyAccept):
		m.input = nil
		v := m.kind.normalize(b.field.Value())
		if v == "" {
			return nil
		}
		if strings.Contains(v, ";") {
			m.status = "an entry cannot hold ;"
			return nil
		}
		at := b.at
		if !b.adding && m.entries[at].value == v {
			return nil
		}
		m.remember()
		if b.adding {
			m.entries = slices.Insert(m.entries, at, &entry{value: v})
		} else {
			m.entries[at].value = v
		}
		// Clear the filter if it would hide what was just typed, then put the cursor on it.
		if !strings.Contains(strings.ToLower(v), strings.ToLower(m.Filter.Value())) {
			m.Filter.SetValue("")
		}
		m.refresh()
		if row := slices.Index(m.Shown, at); row >= 0 {
			m.Table.SetCursor(row)
			m.redraw()
		}
		return nil
	}
	var cmd tea.Cmd
	b.field, cmd = b.field.Update(msg)
	return cmd
}
