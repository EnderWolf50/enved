package theme

import (
	"strings"

	"charm.land/bubbles/v2/help"
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

// Note is what an unchanged row has to say, which tints it lightly: a fact to keep in mind
// (a system variable, a stale value), or a problem (a missing folder, a duplicate).
type Note int

const (
	NoNote Note = iota
	Info
	Problem
)

// NewRow is a row with the background for its change, under the cursor or not; an
// unchanged row with a note gets the note's light tint.
func NewRow(c Change, note Note, cursor bool) Row {
	var bg lipgloss.Style
	switch {
	case c == Removed:
		bg = bg.Background(lipgloss.Color(Pick(cursor, current.RowRemovedCursor, current.RowRemoved)))
	case c == Added:
		bg = bg.Background(lipgloss.Color(Pick(cursor, current.RowAddedCursor, current.RowAdded)))
	case c == Edited:
		bg = bg.Background(lipgloss.Color(Pick(cursor, current.RowEditedCursor, current.RowEdited)))
	case note == Info:
		bg = bg.Background(lipgloss.Color(Pick(cursor, current.RowNoteCursor, current.RowNote)))
	case note == Problem:
		bg = bg.Background(lipgloss.Color(Pick(cursor, current.RowProblemCursor, current.RowProblem)))
	case cursor:
		bg = bg.Background(lipgloss.Color(current.RowCursor))
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

// Legend is the key to the row tints, for Divider: a swatch of each one and its meaning.
// changes adds added, edited and removed; info and problem name what those light tints
// mark, if anything.
func Legend(changes bool, info, problem string) string {
	swatch := func(bg string, what string) string {
		return lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render("  ") + Dim.Render(" "+what)
	}
	var items []string
	if changes {
		items = append(items, swatch(current.RowAdded, "added"), swatch(current.RowEdited, "edited"), swatch(current.RowRemoved, "removed"))
	}
	if info != "" {
		items = append(items, swatch(current.RowNote, info))
	}
	if problem != "" {
		items = append(items, swatch(current.RowProblem, problem))
	}
	return strings.Join(items, "  ")
}

// Divider is a line across the body with the legend at its right end, when it fits.
func Divider(width int, legend string) string {
	lw := lipgloss.Width(legend)
	if legend == "" || lw+6 > width {
		return Faint.Render(strings.Repeat("─", max(width, 0)))
	}
	return Faint.Render(strings.Repeat("─", width-lw-4)+" ") + legend + Faint.Render(" ──")
}

// Pick is a if cond, else b.
func Pick[T any](cond bool, a, b T) T {
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

// HelpLines lays groups of keys out over lines of width: a group's keys stay together, a
// group that does not fit starts a new line, and groups on one line are set apart by a dim │.
// A group wider than a line is split.
func HelpLines(h help.Model, width int, groups ...[]key.Binding) []string {
	h.SetWidth(0)
	sep := Faint.Render("  │  ")
	var lines []string
	line := ""
	// A group wider than a line goes in as several, each as wide as fits.
	var fitted [][]key.Binding
	for _, g := range groups {
		for len(g) > 0 {
			n := 1
			for n < len(g) && lipgloss.Width(h.ShortHelpView(g[:n+1])) <= width {
				n++
			}
			fitted, g = append(fitted, g[:n]), g[n:]
		}
	}
	for _, g := range fitted {
		view := h.ShortHelpView(g)
		switch {
		case line == "":
			line = view
		case lipgloss.Width(line)+lipgloss.Width(sep)+lipgloss.Width(view) <= width:
			line += sep + view
		default:
			lines = append(lines, line)
			line = view
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(width, 0), "…")
	}
	return lines
}
