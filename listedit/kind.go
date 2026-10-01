package listedit

import (
	"os"
	"strings"

	"github.com/EnderWolf50/enved/winenv"
)

// Kind is what a list's entries are, which decides how they are checked.
type Kind int

const (
	Text       Kind = iota // anything; only repeats are marked
	Folders                // each a folder that should exist
	Paths                  // each a file or folder that should exist
	Extensions             // each a file extension: ".EXE"
)

// known are the variables that are lists without being told, and what their entries are.
var known = map[string]Kind{
	"path":         Folders,
	"psmodulepath": Folders,
	"include":      Folders,
	"lib":          Folders,
	"libpath":      Folders,
	"classpath":    Paths,
	"pathext":      Extensions,
}

// Known says whether a variable is a list by its name, and of what.
func Known(name string) (Kind, bool) {
	k, ok := known[strings.ToLower(name)]
	return k, ok
}

// String is what the entries are, in a sentence: "folders".
func (k Kind) String() string {
	return [...]string{"text", "folders", "files or folders", "extensions"}[k]
}

// column is the heading of the entries' column.
func (k Kind) column() string {
	return [...]string{"ENTRY", "FOLDER", "PATH", "EXTENSION"}[k]
}

func (k Kind) placeholder() string {
	return [...]string{"an entry", `C:\Tools\bin  or  %LOCALAPPDATA%\Programs\thing`, `C:\libs\thing.jar`, ".PS1"}[k]
}

// onDisk says whether entries name files or folders (they expand, exist, can be opened).
func (k Kind) onDisk() bool { return k == Folders || k == Paths }

// Key is what two entries are compared by to find repeats.
func (k Kind) Key(e string) string {
	if k.onDisk() {
		return strings.ToLower(strings.TrimRight(winenv.Expand(e), `\/`))
	}
	return strings.ToLower(strings.TrimSpace(e))
}

// Health is what is wrong with each entry: a problem of its own ("missing", "not .ext"), or
// it repeats an earlier entry (dupOf is that entry's position, from 1).
func (k Kind) Health(entries []string, exists func(string) bool) (problem []string, dupOf []int) {
	problem, dupOf = make([]string, len(entries)), make([]int, len(entries))
	first := map[string]int{}
	for i, e := range entries {
		problem[i] = k.problem(e, exists)
		if j, seen := first[k.Key(e)]; seen {
			dupOf[i] = j + 1
		} else {
			first[k.Key(e)] = i
		}
	}
	return problem, dupOf
}

// problem is what is wrong with an entry on its own, or "".
func (k Kind) problem(e string, exists func(string) bool) string {
	switch k {
	case Folders:
		if !exists(e) {
			return "missing"
		}
	case Paths:
		if !exists(e) {
			return "missing"
		}
	case Extensions:
		if !strings.HasPrefix(e, ".") || len(e) < 2 || strings.ContainsAny(e, ` \/;`) {
			return "not .ext"
		}
	}
	return ""
}

// explain is the long form of a problem, for the details line and the input box.
func (k Kind) explain(problem string) string {
	switch {
	case problem == "missing" && k == Folders:
		return "no such folder"
	case problem == "missing":
		return "no such file or folder"
	case problem != "":
		return "an extension is a dot and letters, like .PS1"
	}
	return ""
}

// fine is what the details line says about an entry with no problem.
func (k Kind) fine() string {
	return [...]string{"", "folder exists", "exists", "extension"}[k]
}

// Exists is the check the editor uses unless told otherwise: for Folders, a folder; for
// Paths, a file or a folder.
func Exists(k Kind) func(string) bool {
	return func(e string) bool {
		fi, err := os.Stat(winenv.Expand(e))
		return err == nil && (k != Folders || fi.IsDir())
	}
}

// noted says what the light row tint marks in a list of this kind, for the legend.
func (k Kind) noted() string {
	switch {
	case k == Extensions:
		return "invalid/duplicate"
	case k.onDisk():
		return "missing/duplicate"
	}
	return "a duplicate"
}
