//go:build !windows

package winenv

import (
	"errors"
	"os"
	"strings"
)

// Elsewhere there is no registry: the programs build, so their tests run, but reading or
// writing fails.
var errNoRegistry = errors.New("the persistent environment is a Windows setting")

var Registry = Store{ReadAll: ReadAll, Apply: Apply, CanWrite: CanWrite}

func ReadAll(Scope) ([]Var, error)                     { return nil, errNoRegistry }
func Read(Scope, string) (v Value, ok bool, err error) { return Value{}, false, errNoRegistry }
func Apply([]Change) error                             { return errNoRegistry }
func CanWrite(Scope) bool                              { return false }
func Broadcast()                                       {}

// Expand expands %VARS% with this process's environment, names compared case-insensitively
// as on Windows.
func Expand(s string) string {
	return ExpandWith(s, func(name string) (string, bool) {
		for _, kv := range os.Environ() {
			if k, v, _ := strings.Cut(kv, "="); strings.EqualFold(k, name) {
				return v, true
			}
		}
		return "", false
	})
}
