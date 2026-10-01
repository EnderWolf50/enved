package listedit

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EnderWolf50/enved/theme"
)

// Grid is a filtered table and the body around it: the part the list editor and the
// variables' tabs share. Its holder keeps the items; the grid knows them by position.
type Grid struct {
	Table   table.Model
	Filter  textinput.Model
	Help    help.Model
	Shown   []int // positions in the holder's items that the table shows, narrowed by the filter
	Focused bool  // the cursor row is only painted while the grid has the keys
	W, H    int   // the body's size
}

// NewGrid is an empty grid; placeholder is the filter's hint.
func NewGrid(placeholder string) Grid {
	g := Grid{Table: theme.NewTable(), Filter: textinput.New(), Help: help.New()}
	g.Filter.Prompt = "/ "
	g.Filter.Placeholder = placeholder
	return g
}

// Lines of the body around the table: the filter above; a blank, the divider and two detail
// lines below. The help takes as many more as it needs.
const chrome = 5

// SetSize fits the grid to a body of w by h cells; cols are the columns for that width.
func (g *Grid) SetSize(w, h int, cols func() []table.Column) {
	g.W, g.H = w, h
	g.Table.SetColumns(cols())
	g.Table.SetWidth(w)
	g.Filter.SetWidth(w - 2)
	g.Help.SetWidth(w)
}

// Narrow keeps the n items whose text holds the filter.
func (g *Grid) Narrow(n int, text func(int) string) {
	query := strings.ToLower(g.Filter.Value())
	g.Shown = nil
	for i := range n {
		if strings.Contains(strings.ToLower(text(i)), query) {
			g.Shown = append(g.Shown, i)
		}
	}
}

// Paint fills the table, row by row; row gets the item and whether the cursor is on it.
func (g *Grid) Paint(row func(i int, cursor bool) table.Row) {
	if len(g.Table.Columns()) == 0 {
		return // not sized yet
	}
	cursor := min(g.Table.Cursor(), max(len(g.Shown)-1, 0))
	rows := make([]table.Row, len(g.Shown))
	for r, i := range g.Shown {
		rows[r] = row(i, r == cursor && g.Focused)
	}
	g.Table.SetRows(rows)
	g.Table.SetCursor(cursor)
}

// Current is the item under the cursor, or -1.
func (g *Grid) Current() int {
	c := g.Table.Cursor()
	if c < 0 || c >= len(g.Shown) {
		return -1
	}
	return g.Shown[c]
}

// Point puts the cursor on item i, if it is shown.
func (g *Grid) Point(i int) {
	for r, j := range g.Shown {
		if j == i {
			g.Table.SetCursor(r)
		}
	}
}

// UpdateFilter takes a key while the filter is being typed: the table narrows as you type,
// enter keeps the filter, esc drops it. changed says the holder must narrow again.
func (g *Grid) UpdateFilter(msg tea.KeyPressMsg) (cmd tea.Cmd, changed bool) {
	switch {
	case key.Matches(msg, KeyFilterKeep):
		g.Filter.Blur()
		return nil, false
	case key.Matches(msg, KeyFilterClear):
		g.Filter.Blur()
		g.Filter.SetValue("")
		return nil, true
	}
	g.Filter, cmd = g.Filter.Update(msg)
	g.Table.SetCursor(0)
	return cmd, true
}

// Layout is the body: the filter, the table, a blank line, the divider with legend, the two
// detail lines and the help, made of groups of related keys over as many lines as they need,
// which the table gives up. While the filter is typed its own keys take those lines. With
// nothing shown, empty (when not "") stands in for the table.
func (g *Grid) Layout(legend string, detail [2]string, empty string, groups ...[]key.Binding) string {
	lines := theme.HelpLines(g.Help, g.W, groups...)
	g.Table.SetHeight(max(g.H-chrome-len(lines), 1))
	if g.Filter.Focused() {
		n := len(lines)
		lines = theme.HelpLines(g.Help, g.W, []key.Binding{KeyFilterKeep, KeyFilterClear})
		for len(lines) < n {
			lines = append(lines, "")
		}
	}

	width := max(g.W, 0)
	body := g.Table.View()
	if len(g.Shown) == 0 && empty != "" {
		if g.Filter.Value() != "" {
			empty = "nothing matches the filter · esc clears it"
		}
		body = lipgloss.Place(width, lipgloss.Height(body), lipgloss.Center, lipgloss.Center, theme.Dim.Render(empty))
	}
	filter := ""
	if g.Filter.Focused() || g.Filter.Value() != "" {
		filter = g.Filter.View()
	}
	return strings.Join([]string{
		filter,
		body, "",
		theme.Divider(width, legend),
		ansi.Truncate(detail[0], width, "…"),
		ansi.Truncate(detail[1], width, "…"),
		strings.Join(lines, "\n"),
	}, "\n")
}
