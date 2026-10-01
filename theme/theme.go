// Package theme is the look shared by enved and pathed: the settings file's [theme], the
// colors and styles made from it, and the helpers that paint table rows with them.
package theme

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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

# Row backgrounds: the cursor row, and rows changed since the last save.
row_cursor          = "#262626"
row_added           = "#16241f"
row_added_cursor    = "#223a30"
row_edited          = "#3a2c12"
row_edited_cursor   = "#4d3b19"
row_removed         = "#2e1616"
row_removed_cursor  = "#432020"
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
}

func (t Theme) colors() map[string]string {
	return map[string]string{
		"accent": t.Accent, "dim": t.Dim, "faint": t.Faint, "ok": t.OK, "bad": t.Bad, "warn": t.Warn,
		"row_cursor": t.RowCursor, "row_added": t.RowAdded, "row_added_cursor": t.RowAddedCursor,
		"row_edited": t.RowEdited, "row_edited_cursor": t.RowEditedCursor,
		"row_removed": t.RowRemoved, "row_removed_cursor": t.RowRemovedCursor,
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

// Path is $<PROGRAM>_CONFIG, else <program>/config.toml in $XDG_CONFIG_HOME or ~/.config.
func Path(program string) string {
	if p := os.Getenv(strings.ToUpper(program) + "_CONFIG"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, program, "config.toml")
}

// Parse decodes text over base, keeping what text leaves out, and checks the result: a typo
// in a key or a value is an error, not a setting silently ignored.
func Parse[T any](base T, text string, check func(T) error) (T, error) {
	c := base
	md, err := toml.Decode(text, &c)
	if err != nil {
		return base, err
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		var names []string
		for _, k := range keys {
			names = append(names, k.String())
		}
		return base, fmt.Errorf("unknown setting %s", strings.Join(names, ", "))
	}
	if err := check(c); err != nil {
		return base, err
	}
	return c, nil
}

// Load reads the file at path over base; no file means base.
func Load[T any](path string, base T, check func(T) error) (T, error) {
	text, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	if err != nil {
		return base, err
	}
	c, err := Parse(base, string(text), check)
	if err != nil {
		return base, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// The theme's colors and the styles made from them, set by Apply.
var (
	ColorAccent, ColorDim, ColorFaint, ColorOK, ColorBad, ColorWarn color.Color

	// Row backgrounds, by change, each plain and under the cursor.
	bgCursor                   color.Color
	bgAdded, bgAddedCursor     color.Color
	bgEdited, bgEditedCursor   color.Color
	bgRemoved, bgRemovedCursor color.Color

	Accent, OK, Err, Warn, Dim, Faint lipgloss.Style
	Panel, Modal, Dialog              lipgloss.Style
)

// onlyTheme is a settings file with nothing but a [theme].
type onlyTheme struct {
	Theme Theme `toml:"theme"`
}

func init() {
	c, err := Parse(onlyTheme{}, Default, func(c onlyTheme) error { return c.Theme.Check() })
	if err != nil {
		panic("the default theme is broken: " + err.Error())
	}
	Apply(c.Theme)
}

// Apply makes t the theme in force.
func Apply(t Theme) {
	c := lipgloss.Color
	ColorAccent, ColorDim, ColorFaint = c(t.Accent), c(t.Dim), c(t.Faint)
	ColorOK, ColorBad, ColorWarn = c(t.OK), c(t.Bad), c(t.Warn)
	bgCursor = c(t.RowCursor)
	bgAdded, bgAddedCursor = c(t.RowAdded), c(t.RowAddedCursor)
	bgEdited, bgEditedCursor = c(t.RowEdited), c(t.RowEditedCursor)
	bgRemoved, bgRemovedCursor = c(t.RowRemoved), c(t.RowRemovedCursor)

	fg := func(col color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(col) }
	Accent = fg(ColorAccent).Bold(true)
	OK, Err, Warn, Dim, Faint = fg(ColorOK), fg(ColorBad), fg(ColorWarn), fg(ColorDim), fg(ColorFaint)
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
