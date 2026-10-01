// pathed views and edits the persistent Windows PATH (User and Machine) from a TUI or the
// command line: enved path, as a command of its own.
package main

import (
	"os"

	"github.com/EnderWolf50/enved/internal/app"
)

func main() { os.Exit(app.Main("pathed", os.Args[1:])) }
