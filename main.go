// enved views and edits the persistent Windows environment variables (User and Machine) from
// a TUI or the command line. The program is in internal/app; cmd/pathed is its PATH alone.
package main

import (
	"os"

	"github.com/EnderWolf50/enved/internal/app"
)

func main() { os.Exit(app.Main("enved", os.Args[1:])) }
