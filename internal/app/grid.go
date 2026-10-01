package app

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/EnderWolf50/enved/theme"
)

// grid is a filtered table of variables, the part the User, Machine and Process tabs share.
type grid struct {
	table   table.Model
	filter  textinput.Model
	help    help.Model
	shown   []int // positions in the tab's items that the table shows, narrowed by the filter
	focused bool
	w, h    int
}

func newGrid() grid {
	g := grid{table: theme.NewTable(), filter: textinput.New(), help: help.New()}
	g.filter.Prompt = "/ "
	g.filter.Placeholder = "filter by name or value"
	return g
}

// Lines of the body around the table: the filter above; a blank, the divider and two detail
// lines below. The help takes as many more as footer needs.
const gridChrome = 5

func (g *grid) resize(w, h int, cols func() []table.Column) {
	g.w, g.h = w, h
	g.table.SetColumns(cols())
	g.table.SetWidth(w)
	g.filter.SetWidth(w - 2)
	g.help.SetWidth(w)
}

// narrow keeps the items whose text holds the filter.
func (g *grid) narrow(n int, text func(int) string) {
	query := strings.ToLower(g.filter.Value())
	g.shown = nil
	for i := range n {
		if strings.Contains(strings.ToLower(text(i)), query) {
			g.shown = append(g.shown, i)
		}
	}
}

// paint fills the table, row by row; row gets the item and whether the cursor is on it.
func (g *grid) paint(row func(i int, cursor bool) table.Row) {
	if len(g.table.Columns()) == 0 {
		return // not sized yet
	}
	cursor := min(g.table.Cursor(), max(len(g.shown)-1, 0))
	rows := make([]table.Row, len(g.shown))
	for r, i := range g.shown {
		rows[r] = row(i, r == cursor && g.focused)
	}
	g.table.SetRows(rows)
	g.table.SetCursor(cursor)
}

// current is the item under the cursor, or -1.
func (g *grid) current() int {
	c := g.table.Cursor()
	if c < 0 || c >= len(g.shown) {
		return -1
	}
	return g.shown[c]
}

// point puts the cursor on item i, if it is shown.
func (g *grid) point(i int) {
	for r, j := range g.shown {
		if j == i {
			g.table.SetCursor(r)
		}
	}
}

// updateFilter takes a key while the filter is being typed; changed says the table must
// narrow again.
func (g *grid) updateFilter(msg tea.KeyPressMsg) (cmd tea.Cmd, changed bool) {
	switch msg.String() {
	case "enter":
		g.filter.Blur()
		return nil, false
	case "esc":
		g.filter.Blur()
		g.filter.SetValue("")
		return nil, true
	}
	g.filter, cmd = g.filter.Update(msg)
	g.table.SetCursor(0)
	return cmd, true
}

// footer is the help under the table: groups of related keys, over as many lines as they
// need, which the table gives up. While the filter is typed its own keys take those lines.
func (g *grid) footer(groups ...[]key.Binding) string {
	lines := theme.HelpLines(g.help, g.w, groups...)
	g.table.SetHeight(max(g.h-gridChrome-len(lines), 1))
	if g.filter.Focused() {
		n := len(lines)
		lines = theme.HelpLines(g.help, g.w, []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "keep filter")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
		})
		for len(lines) < n {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func (g *grid) filterLine() string {
	if g.filter.Focused() || g.filter.Value() != "" {
		return g.filter.View()
	}
	return ""
}
