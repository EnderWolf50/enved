package main

import (
	"testing"

	"github.com/EnderWolf50/enved/theme"
)

func parseTestConfig(text string) (config, error) { return theme.Parse(cfg, text, checkConfig) }

func TestConfigMistakesAreErrors(t *testing.T) {
	for _, text := range []string{`sidebar_width = 3`, `lists = "Path"`, `nope = 1`} {
		if _, err := parseTestConfig(text); err == nil {
			t.Errorf("%q was accepted", text)
		}
	}
}
