# enved: design notes

Status: an idea and a module; no code yet. This records what was asked for and the analysis
behind the choices still open, so the work can start from here.

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

## What it should do, carried over from bump and pathed

- **Layout**: full screen; a sidebar with the scopes (User, Machine; perhaps the current
  process, read-only) and a scrolling table of `NAME`, `VALUE`, and whether the value holds
  `%VARS%` (`REG_EXPAND_SZ`). `enter`/`→`/`l` into the table, `←`/`h`/`esc`/`q` back.
- **Edits are marked, not written**: added rows green, edited amber, removed red and struck
  through, until `s` shows a review of every change and `enter` writes them.
- **Registry, written directly**: keep `REG_SZ` vs `REG_EXPAND_SZ` (the .NET API turns
  everything into `REG_SZ` and breaks `%VARS%`), back up old values, broadcast
  `WM_SETTINGCHANGE` so new programs see the change.
- **Machine variables through UAC**: editable without admin; saving starts an elevated copy
  with the change on its command line (pathed v0.3.0 does exactly this), and a declined
  prompt marks the scope and keeps its changes.
- **Details** of the variable under the cursor, `/` filter, `r` reload, `u` undo, a quit
  dialog that only asks about unsaved changes, a TOML settings file with the theme
  (`enved --default-config`), and chezmoi rendering that theme from `themes.toml`.
- **CLI**: `enved get NAME`, `enved set NAME VALUE [-m] [--expand]`, `enved unset NAME [-m]`,
  `enved list [-m]`.

## Editing list values such as PATH: two ways

### 1. Run pathed.exe for PATH

enved suspends its screen (`tea.ExecProcess`), runs `pathed`, and redraws when it exits; if
`pathed` is not on `PATH`, the value opens in a plain text editor instead.

- Quick to build; the two programs stay independent.
- The screen switches programs; pathed has its own settings and theme; pathed edits only
  `PATH`, so `PATHEXT`, `PSModulePath` and other lists get plain text; pathed's changes are
  saved by pathed, outside enved's review.

### 2. One list editor, shared (recommended)

Move pathed's core out of `package main` into packages enved imports: the registry store,
the UAC elevation, the entry model (added/edited/removed, missing folders, duplicates) and
the list-editor TUI model. enved then opens that editor in place, in the same screen, for any
`;`-separated value; other values get a text field.

- No switch between programs; one theme and one settings file.
- The list editor works for every list variable, not just `PATH`.
- enved needs the elevation code for Machine variables anyway; this way there is one copy.
- pathed can become a thin command over the same packages ("open the PATH editor"), or a
  shortcut into enved.
- Costs a refactor of pathed first, and the packages' API has to be kept stable for two
  programs.

Either way, a value that is not a list (or when the list editor is not wanted) is edited as
text.

## Open questions

- Which way to integrate (1 or 2)? The analysis leans to 2.
- If 2: does pathed stay its own program, or become `enved path`?
- Which variables count as lists: a fixed set (`PATH`, `PATHEXT`, `PSModulePath`, ...), any
  value with `;`, or the user's choice per variable?
- Show the process's own environment (read-only) as a third scope?
- Guard rails for variables Windows relies on (`SystemRoot`, `ComSpec`, `windir`, ...):
  refuse to remove them, or only warn in the review?
