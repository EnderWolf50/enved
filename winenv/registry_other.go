//go:build !windows

package winenv

import (
	"errors"
)

// Elsewhere there is no registry: the programs build, so their tests run, but reading or
// writing fails.
var errNoRegistry = errors.New("the persistent environment is a Windows setting")

func ReadAll(Scope) ([]Var, error) { return nil, errNoRegistry }
func Apply([]Change) error         { return errNoRegistry }
func CanWrite(Scope) bool          { return false }
func Broadcast()                   {}

// Expand leaves s as it is: %VARS% are a Windows thing.
func Expand(s string) string { return s }
