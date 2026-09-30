// Package elevate writes what this process may not (the Machine variables, unelevated): the
// program starts itself again through UAC ("runas") with the changes on its command line,
// waits for it, and reads back why it failed if it did. The changes travel in the arguments,
// which no other process can change between here and the elevated copy, rather than in a
// file that could be swapped.
//
// A program that uses it calls HandleArgs first thing in main, so its elevated copy does
// the write and exits.
package elevate

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/EnderWolf50/enved/winenv"
)

// Flag is the hidden command the elevated copy runs: Flag <changes> <result file>.
const Flag = "--elevated-write"

// ErrDeclined is returned when the UAC prompt was declined.
var ErrDeclined = errors.New("the UAC prompt was declined")

// Registry is winenv.Registry, with writes this process may not make done through UAC.
var Registry = winenv.Store{ReadAll: winenv.ReadAll, Apply: Apply, CanWrite: winenv.CanWrite}

// maxArg is how much of a command line (32767 characters, the program's path and the result
// file's included) the changes may take.
const maxArg = 30000

// Apply writes changes, scope by scope, User first: directly where this process may, else
// through an elevated copy of the program, after one UAC prompt (more only for changes too
// long for one command line).
func Apply(changes []winenv.Change) error {
	var errs []error
	for _, s := range winenv.Scopes {
		mine := slices.DeleteFunc(slices.Clone(changes), func(c winenv.Change) bool { return c.Scope != s })
		if len(mine) == 0 {
			continue
		}
		err := winenv.Apply(mine)
		if errors.Is(err, winenv.ErrNeedsAdmin) {
			err = applyElevated(mine)
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func applyElevated(changes []winenv.Change) error {
	runs, err := batches(winenv.Ordered(changes))
	if err != nil {
		return err
	}
	for _, arg := range runs {
		if err := run(arg); err != nil {
			return err
		}
	}
	return nil
}

// batches encodes changes into as few arguments as fit on a command line, in order. A
// change too long for one on its own is refused before anything is written.
func batches(changes []winenv.Change) ([]string, error) {
	var out []string
	for len(changes) > 0 {
		n := len(changes)
		arg, err := encode(changes[:n])
		for err == nil && len(arg) > maxArg && n > 1 {
			n = max(n/2, 1)
			arg, err = encode(changes[:n])
		}
		if err != nil {
			return nil, err
		}
		if len(arg) > maxArg {
			return nil, fmt.Errorf("%s is too long to hand to an elevated copy (%d characters)", changes[0], len(arg))
		}
		out = append(out, arg)
		changes = changes[n:]
	}
	return out, nil
}

func encode(changes []winenv.Change) (string, error) {
	b, err := json.Marshal(changes)
	return base64.RawURLEncoding.EncodeToString(b), err
}

func decode(s string) ([]winenv.Change, error) {
	var changes []winenv.Change
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &changes)
	}
	return changes, err
}

// HandleArgs is the elevated copy: when args are Flag's, it writes what it was asked to,
// leaves the reason for a failure in the result file, and says to exit with code.
func HandleArgs(args []string) (handled bool, code int) {
	if len(args) != 3 || args[0] != Flag {
		return false, 0
	}
	changes, err := decode(args[1])
	if err == nil {
		err = winenv.Apply(changes)
	}
	if err != nil {
		os.WriteFile(args[2], []byte(err.Error()), 0o600)
		return true, 1
	}
	return true, 0
}
