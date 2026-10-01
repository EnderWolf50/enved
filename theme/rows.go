package theme

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Change is how a row differs from the last save; it picks the row's background.
type Change int

const (
	Unchanged Change = iota
	Added
	Edited
	Removed
)

// Sign is the change's mark in a row's first column.
func (c Change) Sign() string {
	return [...]string{" ", "+", "~", "-"}[c]
}

// Row paints the text of one table row. A background only holds up to the next reset, so
// every piece of the row is painted with it: each colored run and each cell's padding.
type Row struct{ bg lipgloss.Style }

// NewRow is a row with the background for its change, under the cursor or not.
func NewRow(c Change, cursor bool) Row {
	var bg lipgloss.Style
	switch {
	case c == Removed:
		bg = bg.Background(pick(cursor, bgRemovedCursor, bgRemoved))
	case c == Added:
		bg = bg.Background(pick(cursor, bgAddedCursor, bgAdded))
	case c == Edited:
		bg = bg.Background(pick(cursor, bgEditedCursor, bgEdited))
	case cursor:
		bg = bg.Background(bgCursor)
	}
	return Row{bg}
}

// Paint renders text in s on the row's background.
func (r Row) Paint(s lipgloss.Style, text string) string {
	if c := r.bg.GetBackground(); c != nil {
		s = s.Background(c)
	}
	return s.Render(text)
}

// Mark is the first cell: the cursor's › and the change's sign.
func (r Row) Mark(c Change, cursor bool) string {
	mark := r.Paint(lipgloss.NewStyle(), " ")
	if cursor {
		mark = r.Paint(Accent, "›")
	}
	return mark + r.Paint(Accent, c.Sign())
}

// Cells fits painted cells to the table's columns, padding each by a cell on either side.
func (r Row) Cells(cols []table.Column, cells ...string) table.Row {
	for c := range cells {
		w := cols[c].Width
		cells[c] = r.bg.Width(w).Padding(0, 1).Render(ansi.Truncate(cells[c], max(w-2, 0), r.Paint(lipgloss.NewStyle(), "…")))
	}
	return cells
}

func pick[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

// Columns fits columns to a width: the one with no width takes what the others leave. The
// widths include a cell of padding on each side, which Row.Cells paints itself so a row's
// background runs unbroken.
func Columns(width int, cols ...table.Column) []table.Column {
	used, grow := 0, -1
	for i, c := range cols {
		if c.Width == 0 {
			grow = i
		}
		used += c.Width + 2
	}
	if grow >= 0 {
		cols[grow].Width = max(width-used, 10)
	}
	for i := range cols {
		cols[i].Width += 2
		cols[i].Title = " " + cols[i].Title
	}
	return cols
}

// NewTable is a table that only moves: every other key is the program's, and the rows come
// painted from Row.
func NewTable() table.Model {
	km := table.DefaultKeyMap()
	km.PageUp = key.NewBinding(key.WithKeys("pgup"))
	km.PageDown = key.NewBinding(key.WithKeys("pgdown"))
	km.HalfPageUp = key.NewBinding(key.WithKeys("ctrl+u"))
	km.HalfPageDown = key.NewBinding(key.WithKeys("ctrl+d"))
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Padding(0).Foreground(ColorDim).BorderForeground(ColorFaint)
	styles.Cell = lipgloss.NewStyle()     // Row.Cells pads and paints every cell itself
	styles.Selected = lipgloss.NewStyle() // the cursor row is painted by NewRow too
	return table.New(table.WithKeyMap(km), table.WithStyles(styles), table.WithFocused(true))
}
