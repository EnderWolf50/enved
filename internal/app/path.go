package app

// `enved path`, or pathed: the PATH on its own. Without a subcommand it opens an editor of
// the User and Machine PATHs; list, add, rm and clean change one from the command line.

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

const pathedUsage = `pathed - edit the persistent PATH (the same as enved path)

  pathed                   the editor of the User and Machine PATHs
  pathed list [-m]         numbered entries; missing folders and duplicates are marked
  pathed add <dir> [-m] [--front]
  pathed rm <dir|N> [-m]   N is the number shown by 'pathed list'
  pathed clean [-m]        drop duplicates and folders that do not exist
  pathed init pwsh         print a pathed function that also updates this shell's PATH;
                           in $PROFILE: pathed.exe init pwsh | Out-String | Invoke-Expression
  pathed --version         print the version

-m works on the Machine PATH; saving it asks for admin (UAC) unless pathed already runs
elevated. Every write first saves the old value to %LOCALAPPDATA%\enved\.
Settings are enved's: ~/.config/enved/config.toml, or the file named by $ENVED_CONFIG.
For the other environment variables, see enved.`

// isFolder is whether a PATH entry is a folder.
var isFolder = listedit.Exists(listedit.Folders)

// pathKey is what two entries are compared by: expanded, case-insensitive, no trailing slash.
func pathKey(e string) string {
	return strings.ToLower(strings.TrimRight(winenv.Expand(e), `\/`))
}

// pathStatus reports, per entry, whether it repeats an earlier one and whether its folder
// exists.
func pathStatus(entries []string, exists func(string) bool) (dup, missing []bool) {
	seen := map[string]bool{}
	dup, missing = make([]bool, len(entries)), make([]bool, len(entries))
	for i, e := range entries {
		k := pathKey(e)
		dup[i], missing[i] = seen[k], !exists(e)
		seen[k] = true
	}
	return dup, missing
}

func cleanPath(entries []string, exists func(string) bool) []string {
	dup, missing := pathStatus(entries, exists)
	var out []string
	for i, e := range entries {
		if !dup[i] && !missing[i] {
			out = append(out, e)
		}
	}
	return out
}

// pathChange is the write that turns old (nil: none yet) into entries. A PATH is
// REG_EXPAND_SZ unless it was saved as REG_SZ and still has no %VARS%.
func pathChange(s winenv.Scope, old *winenv.Var, entries []string) winenv.Change {
	c := winenv.Change{Scope: s, Name: "Path", New: &winenv.Value{Data: winenv.Join(entries), Type: winenv.ExpandSZ}}
	if old != nil {
		c.Name, c.Old = old.Name, &old.Value
		if old.Type == winenv.SZ && !strings.Contains(c.New.Data, "%") {
			c.New.Type = winenv.SZ
		}
	}
	return c
}

func runPath(st winenv.Store, f flags, rest []string) error {
	old, ok, err := find(st, f.scope, "Path")
	if err != nil {
		return err
	}
	var v *winenv.Var
	var entries []string
	if ok {
		v, entries = &old, winenv.Split(old.Data)
	}
	if len(rest) == 0 {
		if err := loadConfig(); err != nil {
			return err
		}
		_, err := tea.NewProgram(newPathModel(st, isFolder, f.scope)).Run()
		return err
	}
	write := func(n []string, what string) error {
		if err := st.Apply([]winenv.Change{pathChange(f.scope, v, n)}); err != nil {
			return err
		}
		fmt.Printf("%s Path: %s.\n", f.scope, what)
		return nil
	}
	switch cmd := rest[0]; {
	case cmd == "list" || cmd == "ls":
		dup, missing := pathStatus(entries, isFolder)
		for i, e := range entries {
			note := ""
			if missing[i] {
				note += "  [missing]"
			}
			if dup[i] {
				note += "  [duplicate]"
			}
			fmt.Printf("%3d  %s%s\n", i+1, e, note)
		}
		return nil
	case cmd == "add" && len(rest) == 2:
		dir, err := filepath.Abs(rest[1])
		if err != nil {
			return err
		}
		for _, e := range entries {
			if pathKey(e) == pathKey(dir) {
				return fmt.Errorf("%s is already in the %s Path", dir, f.scope)
			}
		}
		if !isFolder(dir) {
			return fmt.Errorf("%s is not a folder", dir)
		}
		n := append(entries[:len(entries):len(entries)], dir)
		if f.front {
			n = append([]string{dir}, entries...)
		}
		return write(n, "added "+dir)
	case (cmd == "rm" || cmd == "remove") && len(rest) == 2:
		var n []string
		if i, err := strconv.Atoi(rest[1]); err == nil {
			if i < 1 || i > len(entries) {
				return fmt.Errorf("no entry %d in the %s Path (1-%d)", i, f.scope, len(entries))
			}
			n = append(append(n, entries[:i-1]...), entries[i:]...)
		} else {
			abs, _ := filepath.Abs(rest[1])
			for _, e := range entries {
				if pathKey(e) != pathKey(rest[1]) && pathKey(e) != pathKey(abs) {
					n = append(n, e)
				}
			}
		}
		if len(n) == len(entries) {
			return fmt.Errorf("%s is not in the %s Path", rest[1], f.scope)
		}
		return write(n, "removed "+theme.Plural(len(entries)-len(n), "entry", "entries"))
	case cmd == "clean":
		n := cleanPath(entries, isFolder)
		if len(n) == len(entries) {
			fmt.Println("nothing to clean")
			return nil
		}
		return write(n, "removed "+theme.Plural(len(entries)-len(n), "entry", "entries"))
	}
	return errors.New("unknown path command\n\n" + pathedUsage)
}
