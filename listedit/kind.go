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

// column is the heading of the entries' column.
func (k Kind) column() string {
	return [...]string{"ENTRY", "FOLDER", "PATH", "EXTENSION"}[k]
}

func (k Kind) placeholder() string {
	return [...]string{"an entry", `C:\Tools\bin  or  %LOCALAPPDATA%\Programs\thing`, `C:\libs\thing.jar`, ".PS1"}[k]
}

// onDisk says whether entries name files or folders (they expand, exist, can be opened).
func (k Kind) onDisk() bool { return k == Folders || k == Paths }

// key is what two entries are compared by to find repeats.
func (k Kind) key(e string) string {
	if k.onDisk() {
		return strings.ToLower(strings.TrimRight(winenv.Expand(e), `\/`))
	}
	return strings.ToLower(strings.TrimSpace(e))
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
		return "not an extension, or a duplicate"
	case k.onDisk():
		return "missing, or a duplicate"
	}
	return "a duplicate"
}
