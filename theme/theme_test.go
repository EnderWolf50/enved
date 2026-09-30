package theme

import (
	"strings"
	"testing"
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
