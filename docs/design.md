# enved: design

Status: written as designed, steps 1 to 5 below; pathed runs on the shared packages. The
first half records what was asked for; the second half the decisions, the packages, the
screens and the CLI. Where the code went another way than first planned, the text says so.

## What it is for

A TUI and CLI to view and edit Windows environment variables, User and Machine, grown the
same way as [bump](https://github.com/EnderWolf50/bump) and
[pathed](https://github.com/EnderWolf50/pathed): the same layout, keys, theme and settings
file, so the three feel like one family.

The request that started it:

- A project for managing environment variables, developed like bump and pathed.
- For a `PATH` value, hand the editing to pathed; if pathed is not installed, edit it as
  plain text.
- Better still: integrate the two so it is painless.

The name mirrors pathed (path editor): **enved**, env editor. Nothing on the machine or on
the GitHub account used it.

## Carried over from bump and pathed

- **Layout**: full screen; a sidebar with the scopes and a scrolling table of variables.
  `enter`/`→`/`l` into the table, `←`/`h`/`esc`/`q` back.
- **Edits are marked, not written**: added rows green, edited amber, removed red and struck
  through, until `s` shows a review of every change and `enter` writes them.
- **Registry, written directly**: keep `REG_SZ` vs `REG_EXPAND_SZ` (the .NET API turns
  everything into `REG_SZ` and breaks `%VARS%`), back up old values, broadcast
  `WM_SETTINGCHANGE` so new programs see the change.
- **Machine variables through UAC**: editable without admin; saving starts an elevated copy
  with the change on its command line (pathed v0.3.0 does exactly this), and a declined
  prompt marks the scope and keeps its changes.
- `/` filter, `R` reload, `u` undo, a quit dialog that only asks about unsaved changes, a
  TOML settings file with the theme (`enved --default-config`), and chezmoi rendering that
  theme from `themes.toml`.

## Decisions

### Integration: one shared list editor, in enved's module

Way 2 of the first draft: pathed's core moves out of `package main` into packages, and both
programs use them. enved opens the list editor in place, in its own screen, for any list
variable; nothing switches programs, and there is one theme.

The packages live in **enved's** module, not pathed's: enved is the general tool (any
variable, any list), pathed is the special case (one variable). The dependency then points
from the special to the general: `pathed` imports `github.com/EnderWolf50/enved/...`.

### pathed stays a program, as a thin one

`pathed` keeps its name, its CLI (`list`, `add`, `rm`, `clean`, `init pwsh`) and its install
line, because it is already installed, upgraded by bump and wired into the dotfiles
(`pathed.exe init pwsh`). Its `main` shrinks to the CLI plus "open the shared frame with one
tab per scope, each holding that scope's `Path` in the list editor". No `enved path`
alias at first; it can come later if pathed is ever retired.

### Which variables are lists

A variable is edited as a list when either holds:

1. its name is in a built-in table, which also gives the **kind** of its entries:

   | Kind | Variables | Entry check |
   | --- | --- | --- |
   | folders | `Path`, `PSModulePath`, `INCLUDE`, `LIB`, `LIBPATH` | the folder exists; duplicates (compared expanded, case-insensitive, no trailing `\`) |
   | files or folders | `CLASSPATH` | the file or folder exists |
   | extensions | `PATHEXT` | starts with `.`, no spaces; duplicates |

2. the user said so: in the settings (`lists = ["MY_DIRS"]`, `not_lists = ["LIB"]`), or
   with `L` on a variable, which switches it between list and text. `L`'s choices are kept
   in `%LOCALAPPDATA%\enved\lists.toml`, not in the settings file: chezmoi writes that one,
   and a program rewriting it would fight chezmoi. A variable made a list this way is of
   the kind "text" (unless the table knows it): duplicates are marked, nothing else is
   checked.

A value that merely contains `;` is not a list on its own: plenty of values (connection
strings, `JAVA_TOOL_OPTIONS`) hold semicolons that are not separators.

### A third, read-only scope: this process

The sidebar holds **User**, **Machine** and **Process** (the environment enved started
with). Process cannot be edited; it is there to answer "why doesn't my shell see it": a
Process value that differs from what User and Machine would give now is marked `stale`, and
the details panel says to open a new window, or to use the shell wrapper (below). A
variable no saved one explains (set by Windows at logon, or by the shell) is marked
`here only`. `Path` is stale only when a saved entry is missing from it: shells add their
own entries.

### Guard rails: warn, and confirm twice

Variables Windows or common tools rely on are marked `sys` in the table:
`SystemRoot`, `windir`, `ComSpec`, `OS`, `PATHEXT`, `Path`, `PSModulePath`, `TEMP`, `TMP`,
`PROCESSOR_*`, `NUMBER_OF_PROCESSORS`, `DriverData`. Changing one is allowed (people fix a
broken `TEMP`), but removing it or making it empty puts it under a red "system variables"
heading in the review, and saving needs `enter` on a second question. The CLI's `unset` of
one needs `--force`.

enved never touches `HKCU\Volatile Environment` (set by Windows at logon) and does not list
it.

## Screens

```
╭ enved ───────────────╮╭ User · 23 variables · 2 changes ──────────────────────────────╮
│ ▸ User          23 * ││ NAME                 VALUE                            %  KIND │
│   Machine uac   12   ││ GOPATH               %USERPROFILE%\go                 %       │
│   Process ro         ││ JAVA_HOME            C:\Program Files\Java\jdk-25             │
│                      ││ Path                 C:\Users\me\bin; … (14)          %  list │
│                      ││ TEMP                 %USERPROFILE%\AppData\Local\Temp %   sys │
│                      │├────────────────────────────────────────────────────────────────┤
│                      ││ JAVA_HOME = C:\Program Files\Java\jdk-25                       │
│                      ││ REG_SZ · used by: Path (entry 3)                               │
╰──────────────────────╯╰────────────────────────────────────────────────────────────────╯
 enter edit · e as text · a add · d remove · r rename · x %expand · L list on/off · u undo
```

- **Table**: name, the value on one line (a list shows its first entry and a count), `%` for
  `REG_EXPAND_SZ`, and `list` for list variables. Sorted by name, case-insensitive, the way
  Windows compares names.
- **Details panel** under the table, for the variable under the cursor: the value in full,
  wrapped; the expanded value when it holds `%VARS%`; the registry type; which other
  variables use it (`%JAVA_HOME%` in `Path`), so removing one that is still used can say so
  in the review.
- **Editing** a text value opens a field in a dialog, as pathed's entry editor does; `enter`
  on a list variable replaces the table with the list editor for that value (pathed's
  screen, heading `User › Path`), and `esc` comes back with its changes marked on the row.
- **Keys** in the table: `a` add, `enter` edit (a list in the list editor), `e` edit as
  text, `d` remove or keep again, `r` rename (a remove plus an add, shown as one change), `x`
  switch `REG_SZ`/`REG_EXPAND_SZ`, `L` mark a list or text (it only sets what `enter` opens,
  as `x` only sets the type), `y`/`Y` copy the value/name, `u` undo the last change and `U`
  every change, `/`, `R` reload, `s` save. In the list editor, `a` and `i` add after and
  before the cursor; `x` does nothing there, so it never means two things.
- **Undo** is one history per scope, the list editors' changes included: `u` takes back
  the last change wherever it was made, `U` is itself a step `u` can take back.
- A value typed with a `%` in it becomes `REG_EXPAND_SZ` unless the user switched it with
  `x`; the review shows every type change.
- Names are checked as they are typed: not empty, no `=`, not already in this scope
  (compared case-insensitively).

## CLI

```
enved                          interactive editor
enved list [-m] [--names]      NAME=VALUE, one per line; --names for names only
enved get NAME [-m] [--expand] the stored value; exits 1 when it is not set
enved set NAME VALUE [-m] [--expand | --no-expand]
enved unset NAME [-m] [--force]
enved edit NAME [-m]           the editor, opened on that variable
enved init pwsh                a wrapper that also updates the current shell
enved --version | --config | --default-config
```

Without `-m` a command works on User. `set` keeps the type a variable already has; a new
one gets `REG_EXPAND_SZ` when the value holds `%`. List edits from the command line stay
pathed's job (`pathed add`), and only for `Path`.

`enved init pwsh` generalizes pathed's wrapper: it reads every User and Machine variable
before and after `enved.exe` runs, then sets or removes in `$env:` whatever changed. `Path`
keeps pathed's merge (added entries appended, removed ones dropped, session-only entries
kept); every other variable is replaced outright. The dotfiles load it the way they load
pathed's.

## Packages

All under `github.com/EnderWolf50/enved`, taken from pathed's `package main` and widened
from "the `Path` value" to "any value":

- **`winenv`** — the registry.
  - `Scope` (a string: `User`, `Machine`); where each lives in the registry is private.
  - `Value{Data string, Type uint32}`.
  - `ReadAll(Scope) ([]Var, error)` sorted by name, `Read(Scope, name) (Value, bool, error)`.
  - `Store{ReadAll, Apply, CanWrite}`: the registry, or a fake in tests.
  - `Change{Scope, Name, Old, New *Value}` (nil `Old` adds, nil `New` removes).
  - `Apply([]Change) error`: backs up, writes, broadcasts once. Before writing it rereads
    each value and refuses a change whose `Old` no longer matches, so a value changed by
    something else since enved read it is not overwritten blind (pathed does not check this
    yet).
  - `CanWrite(Scope) bool`, `Split`, `Join`, `Expand`, `Broadcast`.
  - Backups go to `%LOCALAPPDATA%\enved\`, one file per change; pathed's
    `%LOCALAPPDATA%\pathed\` stays readable but nothing new is written there.
- **`elevate`** — running a `winenv.Apply` through UAC.
  - `Apply([]winenv.Change) error`: writes directly when it may, else starts
    `os.Executable()` elevated with the changes on its command line; **one** UAC prompt for
    every Machine change in a save.
  - `HandleArgs(args) (handled bool, code int)`: called first in each program's `main`, so
    `enved.exe` and `pathed.exe` both answer the hidden `--elevated-write` command.
  - The command line holds 32767 characters. Changes that do not fit are split over more
    elevated runs; one change that does not fit on its own is refused before any prompt.
- **`listedit`** — the list editor: pathed's `entry` (added, edited, removed), the table,
  the add/edit dialog, `K`/`J`, `c` clean, `o` open folder, and the health checks, as a
  Bubble Tea component. `New(title, kind, saved, current, exists)`, `Update` (answering
  with an `Event`), `Heading`, `Body`, `Overlay`, `Result() []string`, `Dirty()`, `Review()`.
  A value changed as text and then opened as a list is lined up with the saved one, so its
  entries still show as added, removed or kept. The kinds above decide the checks and whether `o` applies.
- **`frame`** — what both programs draw around their content: the sidebar with its `*`,
  `uac`, `!`, `ro` marks, the panel, the review screen (with the second question for
  warnings), the quit dialog, the save running off the UI loop. A `Tab` supplies its name,
  heading, body, overlay, its changes as `[]winenv.Change`, their review lines and
  warnings; a key in it answers with an `Event` (`Back`, `Save`, `Reload`).
- **`theme`** — the settings file's `[theme]` (`theme.Default`, `Parse`, `Load`, `Path`),
  the styles made from it, and the helpers that paint table rows. Each program keeps its
  own settings struct around it: `sidebar_width` in both, `lists` and `not_lists` in enved.
  Each reads its own file (`~/.config/enved/config.toml`, `$ENVED_CONFIG`), and the
  dotfiles render both from `themes.toml`, with a `roles.enved` next to `roles.pathed`.

Tests keep pathed's style: a fake store in memory (`winenv` behind an interface in `frame`),
key presses driven through `Update`, the rendered screen checked with ANSI stripped. CI runs
on `windows-latest`.

## Order of work

1. **enved v0.1.0, the library and the CLI.** Move `winenv` and `elevate` over from pathed,
   widened to any variable, with the `Old` check and batched elevation. Ship `list`, `get`,
   `set`, `unset`, and the release workflow copied from pathed.
2. **pathed on the packages.** pathed's registry and UAC code are replaced with imports;
   its behavior and tests stay the same. This is the first real user of the API, before
   enved's TUI depends on it.
3. **`listedit` and `frame`.** Move them out of pathed; pathed becomes the thin program
   described above. Release pathed.
4. **enved's TUI.** Sidebar, variable table, details, text editing, review and save, then
   the list editor for list variables.
5. **The rest.** `enved init pwsh` and the dotfiles hook, guard rails, the Process scope and
   `stale`, "used by" in the details.

## Later, maybe

- `enved export [-m] > env.toml` and `enved import env.toml`: the User environment kept in
  the dotfiles and applied by a chezmoi `run_onchange` script, with the same review of what
  would change (`--dry-run`).
- `enved restore`: pick a backup and put it back through the same review.
