package app

import (
	"strings"
	"testing"
)

func parseTestConfig(text string) (config, error) { return parseConfig(defaults, text) }

func TestConfigMistakesAreErrors(t *testing.T) {
	for _, text := range []string{`sidebar_width = 3`, `lists = "Path"`, `nope = 1`} {
		if _, err := parseTestConfig(text); err == nil {
			t.Errorf("%q was accepted", text)
		}
	}
}

func TestThemeMistakesNameTheKey(t *testing.T) {
	for text, want := range map[string]string{
		"[theme]\nacent = \"#112233\"": "theme.acent",
		"[theme]\naccent = \"red\"":    "theme.accent",
	} {
		if _, err := parseTestConfig(text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want one naming %s", text, err, want)
		}
	}
	c, err := parseTestConfig("[theme]\nok = \"42\"")
	if err != nil || c.Theme.OK != "42" || c.Theme.Accent != defaults.Theme.Accent || c.SidebarWidth != defaults.SidebarWidth {
		t.Errorf("override: %+v, %v", c, err)
	}
}
