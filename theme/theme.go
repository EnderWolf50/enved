// Package theme is enved's look: the settings file's [theme], the
// colors and styles made from it, and the helpers that paint table rows with them.
package theme

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"

	"charm.land/lipgloss/v2"
	"github.com/BurntSushi/toml"
)

// Default is the [theme] section of a settings file, with the defaults and what each color
// is for. A program's default settings end with it.
const Default = `[theme]
accent = "#ffc799" # headings, the selection, the focused panel's border
dim    = "#8b8b8b" # secondary text
faint  = "#505050" # borders, dividers
ok     = "#99ffe4" # what is fine: a folder that exists, success
bad    = "#ff8080" # what is wrong: a missing folder, errors, the quit dialog
warn   = "#ffc799" # what to look at: duplicates, system variables

# Row backgrounds: the cursor row; rows changed since the last save; and unchanged rows with
# a note: a fact to keep in mind (a system variable, a stale value) or a problem (a missing
# folder, a duplicate).
row_cursor          = "#262626"
row_added           = "#16241f"
row_added_cursor    = "#223a30"
row_edited          = "#3a2c12"
row_edited_cursor   = "#4d3b19"
row_removed         = "#2e1616"
row_removed_cursor  = "#432020"
row_note            = "#1b2130"
row_note_cursor     = "#283046"
row_problem         = "#271d33"
row_problem_cursor  = "#36284a"
`

// Theme is the [theme] section.
type Theme struct {
	Accent           string `toml:"accent"`
	Dim              string `toml:"dim"`
	Faint            string `toml:"faint"`
	OK               string `toml:"ok"`
	Bad              string `toml:"bad"`
	Warn             string `toml:"warn"`
	RowCursor        string `toml:"row_cursor"`
	RowAdded         string `toml:"row_added"`
	RowAddedCursor   string `toml:"row_added_cursor"`
	RowEdited        string `toml:"row_edited"`
	RowEditedCursor  string `toml:"row_edited_cursor"`
	RowRemoved       string `toml:"row_removed"`
	RowRemovedCursor string `toml:"row_removed_cursor"`
	RowNote          string `toml:"row_note"`
	RowNoteCursor    string `toml:"row_note_cursor"`
	RowProblem       string `toml:"row_problem"`
	RowProblemCursor string `toml:"row_problem_cursor"`
}

func (t Theme) colors() map[string]string {
	return map[string]string{
		"accent": t.Accent, "dim": t.Dim, "faint": t.Faint, "ok": t.OK, "bad": t.Bad, "warn": t.Warn,
		"row_cursor": t.RowCursor, "row_added": t.RowAdded, "row_added_cursor": t.RowAddedCursor,
		"row_edited": t.RowEdited, "row_edited_cursor": t.RowEditedCursor,
		"row_removed": t.RowRemoved, "row_removed_cursor": t.RowRemovedCursor,
		"row_note": t.RowNote, "row_note_cursor": t.RowNoteCursor,
		"row_problem": t.RowProblem, "row_problem_cursor": t.RowProblemCursor,
	}
}

// Check reports the first color that is neither "#rrggbb" nor a number from 0 to 255.
func (t Theme) Check() error {
	for key, value := range t.colors() {
		if !validColor(value) {
			return fmt.Errorf("theme.%s: %q is not \"#rrggbb\" or a number from 0 to 255", key, value)
		}
	}
	return nil
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validColor(s string) bool {
	if hexColor.MatchString(s) {
		return true
	}
	n, err := strconv.Atoi(s)
	return err == nil && n >= 0 && n <= 255
}

// current is the theme in force; the rows take their backgrounds from it.
var current Theme

// The theme's colors and the styles made from them, set by Apply.
var (
	ColorAccent, ColorDim, ColorFaint, ColorOK, ColorBad, ColorWarn color.Color

	Accent, OK, Err, Warn, Dim, Faint lipgloss.Style
	Panel, Modal, Dialog              lipgloss.Style
	Badge                             lipgloss.Style // a short tag on a tinted ground: uac, ro
)

// onlyTheme is a settings file with nothing but a [theme].
type onlyTheme struct {
	Theme Theme `toml:"theme"`
}

func init() {
	var c onlyTheme
	if _, err := toml.Decode(Default, &c); err != nil || c.Theme.Check() != nil {
		panic(fmt.Sprint("the default theme is broken: ", err, c.Theme.Check()))
	}
	Apply(c.Theme)
}

// Apply makes t the theme in force.
func Apply(t Theme) {
	c := lipgloss.Color
	ColorAccent, ColorDim, ColorFaint = c(t.Accent), c(t.Dim), c(t.Faint)
	ColorOK, ColorBad, ColorWarn = c(t.OK), c(t.Bad), c(t.Warn)
	current = t

	fg := func(col color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(col) }
	Accent = fg(ColorAccent).Bold(true)
	OK, Err, Warn, Dim, Faint = fg(ColorOK), fg(ColorBad), fg(ColorWarn), fg(ColorDim), fg(ColorFaint)
	Badge = fg(ColorDim).Background(c(t.RowCursor)).Padding(0, 1)
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	Panel = border.BorderForeground(ColorFaint).Padding(0, 1)
	Modal = border.BorderForeground(ColorBad).Padding(1, 3)     // the quit question
	Dialog = border.BorderForeground(ColorAccent).Padding(1, 2) // adding or editing something
}

// Plural is "1 entry", "2 entries".
func Plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
