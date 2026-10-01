package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/EnderWolf50/enved/theme"
)

// defaultConfig is both the defaults and their documentation: enved reads it before the
// user's file, and `enved --default-config` prints it as a starting point.
const defaultConfig = `# enved's settings. Every key is optional: one left out keeps the value shown here.
# Colors are "#rrggbb" or an ANSI color number, "0" to "255".

# Width of the sidebar, in cells.
sidebar_width = 30

# Variables to edit as lists (entries split at ;) or as text, whatever enved would guess.
# It guesses lists for Path, PATHEXT, PSModulePath, INCLUDE, LIB, LIBPATH and CLASSPATH.
# The L key marks one either way too, and keeps that in %LOCALAPPDATA%\enved\lists.toml.
lists     = []
not_lists = []

` + theme.Default

type config struct {
	SidebarWidth int         `toml:"sidebar_width"`
	Lists        []string    `toml:"lists"`
	NotLists     []string    `toml:"not_lists"`
	Theme        theme.Theme `toml:"theme"`
}

// defaults is defaultConfig, decoded.
var defaults = func() config {
	c, err := parseConfig(config{}, defaultConfig)
	if err != nil {
		panic("the default config is broken: " + err.Error())
	}
	return c
}()

// configPath is $ENVED_CONFIG, else enved/config.toml in $XDG_CONFIG_HOME or ~/.config.
func configPath() string {
	if p := os.Getenv("ENVED_CONFIG"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "enved", "config.toml")
}

// loadConfig is the settings file over the defaults (no file: the defaults); its theme is
// put in force.
func loadConfig() (config, error) {
	path := configPath()
	text, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	c, err := parseConfig(defaults, string(text))
	if err != nil {
		return defaults, fmt.Errorf("%s: %w", path, err)
	}
	theme.Apply(c.Theme)
	return c, nil
}

// parseConfig decodes text over base, keeping what text leaves out, and checks the result: a
// typo in a key or a value is an error, not a setting silently ignored.
func parseConfig(base config, text string) (config, error) {
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
	if c.SidebarWidth < 20 {
		return base, errors.New("sidebar_width must be at least 20")
	}
	if err := c.Theme.Check(); err != nil {
		return base, err
	}
	return c, nil
}
