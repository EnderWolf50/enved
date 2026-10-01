// Package app is enved, and pathed, its PATH on its own: they view and edit the persistent
// Windows environment variables (User and Machine) from a TUI or the command line. They
// write the registry directly, keeping REG_EXPAND_SZ values and their %VARS% intact, save
// the old value before each write, and tell running programs that the environment changed.
// List values such as PATH are edited entry by entry.
package app

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/EnderWolf50/enved/elevate"
	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

const usage = `enved - edit the persistent environment variables

  enved                          interactive editor (User, Machine, and this process)
  enved list [-m] [--names]      NAME=VALUE, one per line; --names for the names only
  enved get NAME [-m] [--expand] the saved value; exits 1 when it is not set
  enved set NAME VALUE [-m] [--expand | --no-expand]
  enved unset NAME [-m] [--force]
  enved edit NAME [-m]           the editor, opened on that variable
  enved path [-m]                the editor, opened on Path
  enved path list [-m]           Path's entries, numbered; missing folders and duplicates marked
  enved path add <dir> [-m] [--front]
  enved path rm <dir|N> [-m]     N is the number shown by 'enved path list'
  enved path clean [-m]          drop duplicates and folders that do not exist
  enved init pwsh                print an enved function that also updates this shell;
                                 in $PROFILE: enved.exe init pwsh | Out-String | Invoke-Expression
  enved --version                print the version
  enved --config                 show where the settings file is
  enved --default-config         print the default settings, a starting point for your own

Without -m a command works on the User variables; -m works on the Machine ones, and saving
them asks for admin (UAC) unless enved already runs elevated.
set keeps the type a variable has; a new one is REG_EXPAND_SZ when its value holds a %.
--expand and --no-expand choose. unset of a variable Windows relies on needs --force.
Every write first saves the old value to %LOCALAPPDATA%\enved\.
Settings (theme, sidebar width, which variables are lists) are read from
~/.config/enved/config.toml, or the file named by $ENVED_CONFIG.`

// pwshInit is the PowerShell wrapper 'enved init pwsh' and 'pathed init pwsh' print, with
// the program's name for __PROG__: the program cannot change the environment of the shell
// that runs it, so a function in that shell has to.
//
//go:embed init.ps1
var pwshInit string

// shellInit returns prog's wrapper for a shell.
func shellInit(prog, shell string) (string, error) {
	switch strings.ToLower(shell) {
	case "pwsh", "powershell":
		return strings.ReplaceAll(pwshInit, "__PROG__", prog), nil
	}
	return "", fmt.Errorf("no init for %q; the shells are: pwsh (or powershell)", shell)
}

// Main runs prog, enved or pathed (enved path on its own), on the command line's arguments,
// and returns the exit code.
func Main(prog string, args []string) int {
	if handled, code := elevate.HandleArgs(args); handled {
		return code
	}
	if err := runAs(prog, args, elevate.Registry); err != nil {
		fmt.Fprintln(os.Stderr, prog+":", err)
		return 1
	}
	return 0
}

// flags are the options, wherever they are among the arguments.
type flags struct {
	scope                          winenv.Scope
	names, expand, noExpand, force bool
	front                          bool // path add: at the front
}

func run(args []string, st winenv.Store) error { return runAs("enved", args, st) }

func runAs(prog string, args []string, st winenv.Store) error {
	f := flags{scope: winenv.User}
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--": // what follows is arguments, even if it starts with -
			rest = append(rest, args[i+1:]...)
			i = len(args)
		case "-m", "--machine":
			f.scope = winenv.Machine
		case "--names":
			f.names = true
		case "--expand":
			f.expand = true
		case "--no-expand":
			f.noExpand = true
		case "--force":
			f.force = true
		case "--front":
			f.front = true
		case "-h", "--help", "help":
			fmt.Println(pick(prog == "pathed", pathedUsage, usage))
			return nil
		case "-v", "--version":
			fmt.Println(prog, versionString())
			return nil
		case "--default-config":
			fmt.Print(defaultConfig)
			return nil
		case "--config":
			path := theme.Path("enved")
			if _, err := os.Stat(path); err != nil {
				path += "  (not there yet: enved --default-config > it, then edit)"
			}
			fmt.Println(path)
			return nil
		default:
			rest = append(rest, a)
		}
	}
	if f.expand && f.noExpand {
		return errors.New("--expand or --no-expand, not both")
	}
	// pathed is enved path: its commands are path's, and init is its own.
	if prog == "pathed" && (len(rest) == 0 || rest[0] != "init") {
		rest = append([]string{"path"}, rest...)
	}
	if len(rest) == 0 {
		return interactive(st, f.scope, "")
	}
	switch cmd := rest[0]; {
	case cmd == "init":
		if len(rest) != 2 {
			return fmt.Errorf("init needs a shell: %s init pwsh", prog)
		}
		script, err := shellInit(prog, rest[1])
		if err != nil {
			return err
		}
		fmt.Print(script)
		return nil
	case cmd == "path":
		return runPath(st, f, rest[1:])
	case cmd == "edit" && len(rest) == 2:
		return interactive(st, f.scope, rest[1])
	case cmd == "list" || cmd == "ls":
		vars, err := st.ReadAll(f.scope)
		if err != nil {
			return err
		}
		for _, v := range vars {
			if f.names {
				fmt.Println(v.Name)
			} else {
				fmt.Printf("%s=%s\n", v.Name, v.Data)
			}
		}
		return nil
	case cmd == "get" && len(rest) == 2:
		v, ok, err := find(st, f.scope, rest[1])
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s is not a %s variable", rest[1], f.scope)
		}
		if f.expand {
			v.Data = winenv.Expand(v.Data)
		}
		fmt.Println(v.Data)
		return nil
	case cmd == "set" && len(rest) == 3:
		return set(st, f, rest[1], rest[2])
	case (cmd == "unset" || cmd == "rm") && len(rest) == 2:
		return unset(st, f, rest[1])
	}
	return errors.New("unknown command\n\n" + usage)
}

// find reads one variable, name compared in any case.
func find(st winenv.Store, s winenv.Scope, name string) (winenv.Var, bool, error) {
	vars, err := st.ReadAll(s)
	if err != nil {
		return winenv.Var{}, false, err
	}
	for _, v := range vars {
		if strings.EqualFold(v.Name, name) {
			return v, true, nil
		}
	}
	return winenv.Var{}, false, nil
}

func set(st winenv.Store, f flags, name, value string) error {
	if p := validName(name); p != "" {
		return errors.New(p)
	}
	old, exists, err := find(st, f.scope, name)
	if err != nil {
		return err
	}
	typ := pick(strings.Contains(value, "%"), winenv.ExpandSZ, winenv.SZ)
	if exists {
		typ, name = old.Type, old.Name // keep its type and the case of its name
	}
	switch {
	case f.expand:
		typ = winenv.ExpandSZ
	case f.noExpand:
		typ = winenv.SZ
	}
	c := winenv.Change{Scope: f.scope, Name: name, New: &winenv.Value{Data: value, Type: typ}}
	if exists {
		if old.Value == *c.New {
			fmt.Printf("%s: %s is already that.\n", f.scope, name)
			return nil
		}
		c.Old = &old.Value
	}
	if err := st.Apply([]winenv.Change{c}); err != nil {
		return err
	}
	fmt.Printf("%s: set %s.\n", f.scope, name)
	return nil
}

func unset(st winenv.Store, f flags, name string) error {
	old, exists, err := find(st, f.scope, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%s is not a %s variable", name, f.scope)
	}
	if isSystem(old.Name) && !f.force {
		return fmt.Errorf("Windows and many programs rely on %s; --force removes it anyway", old.Name)
	}
	if err := st.Apply([]winenv.Change{{Scope: f.scope, Name: old.Name, Old: &old.Value}}); err != nil {
		return err
	}
	fmt.Printf("%s: removed %s.\n", f.scope, old.Name)
	return nil
}

// newModel is the editor: a tab per scope, and the process's environment.
func newModel(st winenv.Store, prefs *listPrefs, on winenv.Scope, name string) (frame.Model, error) {
	var tabs []frame.Tab
	var scopes []*varTab
	start := 0
	for i, s := range winenv.Scopes {
		t := newVarTab(s, st, prefs)
		scopes = append(scopes, t)
		tabs = append(tabs, t)
		if s == on {
			start = i
		}
	}
	for _, t := range scopes {
		t.peers = scopes
	}
	tabs = append(tabs, newProcessTab(scopes))
	if name != "" {
		t := scopes[start]
		if t.err != nil {
			return frame.Model{}, t.err
		}
		if !t.OpenVariable(name) {
			return frame.Model{}, fmt.Errorf("%s is not a %s variable", name, on)
		}
	}
	opts := frame.Options{
		SidebarWidth: cfg.SidebarWidth,
		Apply:        st.Apply,
		AfterSave:    "New windows see the change; with the pwsh wrapper (enved init pwsh), this one does too once enved exits.",
	}
	return frame.New(opts, tabs, start, name != ""), nil
}

// loadConfig reads the settings file over the defaults and applies its theme.
func loadConfig() error {
	c, err := theme.Load(theme.Path("enved"), cfg, checkConfig)
	if err != nil {
		return err
	}
	cfg = c
	theme.Apply(cfg.Theme)
	return nil
}

func interactive(st winenv.Store, on winenv.Scope, name string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	prefs, err := loadLists(cfg, listsPath())
	if err != nil {
		return err
	}
	m, err := newModel(st, prefs, on, name)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m).Run()
	return err
}
