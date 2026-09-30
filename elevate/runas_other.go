//go:build !windows

package elevate

import "errors"

func run(string) error { return errors.New("UAC is a Windows thing") }
