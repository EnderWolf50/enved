package theme

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

type conf struct {
	Width int   `toml:"width"`
	Theme Theme `toml:"theme"`
}

func TestParseMistakesAreErrors(t *testing.T) {
	base, err := Parse(conf{}, "width = 20\n"+Default, func(c conf) error { return c.Theme.Check() })
	if err != nil {
		t.Fatal(err)
	}
	check := func(c conf) error { return c.Theme.Check() }
	for text, want := range map[string]string{
		"[theme]\nacent = \"#112233\"": "theme.acent",
		"[theme]\naccent = \"red\"":    "theme.accent",
		"wdth = 3":                     "wdth",
	} {
		if _, err := Parse(base, text, check); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want one naming %s", text, err, want)
		}
	}
	c, err := Parse(base, "[theme]\nok = \"42\"", check)
	if err != nil || c.Theme.OK != "42" || c.Theme.Accent != "#ffc799" || c.Width != 20 {
		t.Errorf("override: %+v, %v", c, err)
	}
}

func TestDividerFitsItsWidth(t *testing.T) {
	legend := Legend(true, "missing")
	for _, w := range []int{0, 10, 40, 80, 120} {
		if got := lipgloss.Width(Divider(w, legend)); got != w {
			t.Errorf("width %d: the divider is %d wide", w, got)
		}
	}
	if !strings.Contains(Divider(80, legend), "missing") {
		t.Error("the legend is not on a wide divider")
	}
}

func TestNoteTintsOnlyUnchangedRows(t *testing.T) {
	if NewRow(Unchanged, true, false).bg.GetBackground() != bgNote {
		t.Error("an unchanged row with a note is not tinted")
	}
	if NewRow(Removed, true, false).bg.GetBackground() != bgRemoved {
		t.Error("a change's tint does not win over the note's")
	}
	if NewRow(Unchanged, true, true).bg.GetBackground() != bgNoteCursor {
		t.Error("the cursor on a noted row lacks the note's cursor tint")
	}
}
