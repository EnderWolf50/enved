package main

import (
	"regexp"
	"strings"

	"github.com/EnderWolf50/enved/winenv"
)

// system are the variables Windows and common tools rely on: changing one is allowed, but
// removing one or making it empty asks twice.
var system = map[string]bool{}

func init() {
	for _, n := range []string{"SystemRoot", "windir", "ComSpec", "OS", "PATHEXT", "Path", "PSModulePath",
		"TEMP", "TMP", "NUMBER_OF_PROCESSORS", "DriverData"} {
		system[winenv.Key(n)] = true
	}
}

func isSystem(name string) bool {
	return system[winenv.Key(name)] || strings.HasPrefix(winenv.Key(name), "processor_")
}

var reference = regexp.MustCompile(`%([^%;=]+)%`)

// uses says whether a value refers to the variable name (%name%, any case).
func uses(value, name string) bool {
	for _, m := range reference.FindAllStringSubmatch(value, -1) {
		if strings.EqualFold(m[1], name) {
			return true
		}
	}
	return false
}

// validName says what is wrong with a variable name, or "".
func validName(name string) string {
	switch {
	case strings.TrimSpace(name) == "":
		return "a name is needed"
	case strings.Contains(name, "="):
		return "a name cannot hold ="
	case name != strings.TrimSpace(name):
		return "a name cannot start or end with a space"
	case len(name) > 255:
		return "a name is at most 255 characters"
	}
	return ""
}
