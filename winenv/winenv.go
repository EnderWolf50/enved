// Package winenv reads and writes the persistent Windows environment: the variables kept in
// the registry for the User and for the Machine. It writes the registry directly, keeping
// REG_EXPAND_SZ values and their %VARS% intact (.NET's SetEnvironmentVariable rewrites them
// as REG_SZ), saves each old value before writing over it, refuses to write over a value
// that changed since it was read, and tells running programs that the environment changed.
package winenv

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Scope is whose variables: the User's or the Machine's.
type Scope string

const (
	User    Scope = "User"
	Machine Scope = "Machine"
)

// Scopes are the scopes in the order they are shown.
var Scopes = []Scope{User, Machine}

// The registry types a variable can have.
const (
	SZ       uint32 = 1 // REG_SZ: the value as it is
	ExpandSZ uint32 = 2 // REG_EXPAND_SZ: %VARS% in it are expanded when it is read
)

// Value is a variable's value as stored.
type Value struct {
	Data string
	Type uint32
}

// Expands says whether %VARS% in the value are expanded (REG_EXPAND_SZ).
func (v Value) Expands() bool { return v.Type == ExpandSZ }

// Var is a named value.
type Var struct {
	Name string
	Value
}

// Key is what names are compared by: Windows treats them case-insensitively.
func Key(name string) string { return strings.ToLower(name) }

// Sort orders variables by name, case-insensitively, the way Windows compares them.
func Sort(vars []Var) {
	slices.SortFunc(vars, func(a, b Var) int {
		return cmp.Or(cmp.Compare(Key(a.Name), Key(b.Name)), cmp.Compare(a.Name, b.Name))
	})
}

// Change is one write: Old is what the value must still be (nil: it must not exist), New is
// what it becomes (nil: it is removed).
type Change struct {
	Scope Scope
	Name  string
	Old   *Value `json:",omitempty"`
	New   *Value `json:",omitempty"`
}

func (c Change) String() string {
	switch {
	case c.Old == nil:
		return fmt.Sprintf("add %s %s", c.Scope, c.Name)
	case c.New == nil:
		return fmt.Sprintf("remove %s %s", c.Scope, c.Name)
	}
	return fmt.Sprintf("change %s %s", c.Scope, c.Name)
}

// Store is where the variables live: the registry, or a fake in tests.
type Store struct {
	ReadAll  func(Scope) ([]Var, error)
	Apply    func([]Change) error
	CanWrite func(Scope) bool
}

var (
	// ErrNeedsAdmin is returned when this process may not write a scope: the Machine one
	// needs admin.
	ErrNeedsAdmin = errors.New("needs admin")
	// ErrChanged is returned when a value is no longer what a change expects: something
	// else wrote it since it was read.
	ErrChanged = errors.New("changed since it was read")
)

// Ordered puts removals first, so a rename (a removal and an add) that only changes the
// case of a name frees the name before it is written again.
func Ordered(changes []Change) []Change {
	var removals, rest []Change
	for _, c := range changes {
		if c.New == nil {
			removals = append(removals, c)
		} else {
			rest = append(rest, c)
		}
	}
	return append(removals, rest...)
}

// Check goes through ordered changes as if writing them, starting from the values current
// returns, and fails on the first whose Old is not what it would find.
func Check(changes []Change, current func(Scope, string) (*Value, error)) error {
	type at struct {
		s Scope
		k string
	}
	now := map[at]*Value{}
	for _, c := range changes {
		where := at{c.Scope, Key(c.Name)}
		v, seen := now[where]
		if !seen {
			var err error
			if v, err = current(c.Scope, c.Name); err != nil {
				return err
			}
		}
		if !sameValue(v, c.Old) {
			return fmt.Errorf("the %s variable %s %w; read it again", c.Scope, c.Name, ErrChanged)
		}
		now[where] = c.New
	}
	return nil
}

func sameValue(a, b *Value) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Split turns a list value into its entries, dropping empty ones.
func Split(v string) []string {
	var out []string
	for _, e := range strings.Split(v, ";") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// Join is the value a list of entries is stored as.
func Join(entries []string) string { return strings.Join(entries, ";") }

// ExpandWith replaces each %NAME% that lookup knows with its value, as Windows does: an
// unknown one stays as it is.
func ExpandWith(s string, lookup func(string) (string, bool)) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			break
		}
		name := s[i+1 : i+1+j]
		if v, ok := lookup(name); ok && name != "" {
			b.WriteString(s[:i] + v)
			s = s[i+j+2:]
		} else { // the closing % may open the next name
			b.WriteString(s[:i+1+j])
			s = s[i+1+j:]
		}
	}
	return b.String() + s
}

// BackupDir is where the old values go: %LOCALAPPDATA%\enved.
func BackupDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "enved")
}

// ponytail: backups are never pruned; they are a few KB each.
func backup(s Scope, name string, v Value) error {
	dir := BackupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	safe := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < ' ' {
			return '_'
		}
		return r
	}, name)
	file := fmt.Sprintf("%s-%s-%s.txt", s, safe, time.Now().Format("20060102-150405.000"))
	return os.WriteFile(filepath.Join(dir, file), []byte(v.Data), 0o644)
}
