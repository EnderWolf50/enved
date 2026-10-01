package app

// Which variables are edited as lists: the ones listedit knows by name, unless the settings
// or the L key said otherwise. L's choices are kept in %LOCALAPPDATA%\enved\lists.toml, not
// in the settings file, which may be written by something else (chezmoi).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/winenv"
)

type listPrefs struct {
	lists, notLists map[string]bool // by winenv.Key
	path            string          // where L's choices are kept; "" keeps them for this run
}

type listsFile struct {
	Lists    []string `toml:"lists"`
	NotLists []string `toml:"not_lists"`
}

// loadLists starts from the settings' lists, then L's earlier choices.
func loadLists(c config, path string) (*listPrefs, error) {
	p := &listPrefs{lists: map[string]bool{}, notLists: map[string]bool{}, path: path}
	p.add(listsFile{c.Lists, c.NotLists})
	if path == "" {
		return p, nil
	}
	var f listsFile
	_, err := toml.DecodeFile(path, &f)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("%s: %w", path, err)
	}
	p.add(f)
	return p, nil
}

func (p *listPrefs) add(f listsFile) {
	for _, n := range f.Lists {
		p.lists[winenv.Key(n)], p.notLists[winenv.Key(n)] = true, false
	}
	for _, n := range f.NotLists {
		p.lists[winenv.Key(n)], p.notLists[winenv.Key(n)] = false, true
	}
}

// kind says whether a variable is edited as a list, and what its entries are.
func (p *listPrefs) kind(name string) (listedit.Kind, bool) {
	k, known := listedit.Known(name)
	switch {
	case p.notLists[winenv.Key(name)]:
		return 0, false
	case p.lists[winenv.Key(name)]:
		return k, true
	}
	return k, known
}

// toggle marks a variable a list or text, the other way round, and keeps the choice.
func (p *listPrefs) toggle(name string) error {
	_, isList := p.kind(name)
	k := winenv.Key(name)
	p.lists[k], p.notLists[k] = !isList, isList
	if p.path == "" {
		return nil
	}
	var f listsFile
	for n, on := range p.lists {
		if on {
			f.Lists = append(f.Lists, n)
		}
	}
	for n, on := range p.notLists {
		if on {
			f.NotLists = append(f.NotLists, n)
		}
	}
	slices.Sort(f.Lists)
	slices.Sort(f.NotLists)
	var b strings.Builder
	b.WriteString("# Which variables enved edits as lists; written by its L key.\n")
	if err := toml.NewEncoder(&b).Encode(f); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p.path, []byte(b.String()), 0o644)
}

// listsPath is where L's choices are kept.
func listsPath() string {
	return filepath.Join(winenv.BackupDir(), "lists.toml")
}

// listNames are the variables made lists by the settings or L, for tests and messages.
func (p *listPrefs) listNames() []string {
	var out []string
	for n, on := range p.lists {
		if on {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}
