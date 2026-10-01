# enved

View and edit the persistent Windows environment variables, User and Machine, from a TUI or
the command line. A sibling of [bump](https://github.com/EnderWolf50/bump) and
[pathed](https://github.com/EnderWolf50/pathed), with the same keys, theme and settings.

It writes the registry directly, so `REG_EXPAND_SZ` values and their `%VARS%` stay intact
(.NET's `SetEnvironmentVariable` rewrites them as `REG_SZ`). Every write first saves the old
value to `%LOCALAPPDATA%\enved\`, refuses to overwrite a value something else changed since
enved read it, and tells running programs the environment changed, so new windows see it
without signing out.

List values (`Path`, `PATHEXT`, `PSModulePath`, `INCLUDE`, `LIB`, `LIBPATH`, `CLASSPATH`, and
any you choose) open in pathed's list editor, in place: entries checked as you type,
duplicates and missing folders marked, reordered with `K`/`J`.

## Install

Download `enved.exe` from the [latest release](https://github.com/EnderWolf50/enved/releases/latest)
and put it on your `PATH`, or build it with Go 1.27+:

```sh
go install github.com/EnderWolf50/enved@latest
```

## Use

```
enved                          interactive editor (User, Machine, and this process)
enved list [-m] [--names]      NAME=VALUE, one per line
enved get NAME [-m] [--expand] the saved value; exits 1 when it is not set
enved set NAME VALUE [-m] [--expand | --no-expand]
enved unset NAME [-m] [--force]
enved edit NAME [-m]           the editor, opened on that variable
enved init pwsh                print a wrapper that also updates the current shell
```

`-m` works on the Machine variables. Writing them needs admin: unless enved already runs
elevated, saving asks UAC once, and an elevated copy of enved does the write. `set` keeps a
variable's type; a new one is `REG_EXPAND_SZ` when its value holds a `%`. Removing a variable
Windows relies on (`SystemRoot`, `ComSpec`, `TEMP`, `Path`, ...) needs `--force`.

In the editor the sidebar holds User, Machine and Process, each with its number of variables
(a `uac` badge: saving asks for admin; `ro`: read-only; `*` unsaved changes, `!` could not
be read or saved); `enter` opens one. Changes
are only marked until you save: added variables show green, edited ones amber, removed ones
red and struck through; an unchanged row worth a look (a system variable; in a list, a
missing folder or a duplicate; in Process, a stale value) gets a light blue-gray tint. The
divider under the table carries the key to these colors; `s` shows every change, and removing or emptying a system variable
asks a second time.

| Key | In the variables |
| --- | --- |
| `enter` | edit: a list opens in the list editor, anything else in a text field |
| `e` | edit as text, a list included (its entries joined with `;`) |
| `a` | add a variable (name, then value); the table stays sorted by name, as Windows keeps no order |
| `r` | rename |
| `d` | remove, or keep again |
| `x` | switch how it is saved: `REG_SZ` (as it is) or `REG_EXPAND_SZ` (`%VARS%` expanded when read) |
| `L` | mark it a list or text, which `enter` then follows (the `list` in the KIND column); it opens nothing |
| `v` | copy the value |
| `n` | copy the name |
| `u` | undo the last change, here or in a list editor; again for the one before |
| `z` | redo what `u` undid, one change at a time; a new change ends what can be redone |
| `/` | filter by name or value |
| `R` | read the variables again |
| `s` | review the changes, then save them |
| `←` `h` `esc` `q` | back (out of the list editor, then to the sidebar) |

| Key | In the list editor |
| --- | --- |
| `enter`, `e` | edit the entry |
| `a` / `i` | add an entry after / before the cursor |
| `d` | remove the entry, or keep it again |
| `K` / `J` | move the entry up / down |
| `c` | mark every missing folder and duplicate for removal |
| `o` | open the folder in Explorer |
| `u` / `z` | undo the last change / redo it (one history with the table) |
| `←` `h` `esc` `q` | back to the variables, keeping the changes marked |

The details under the table show the expanded value, the registry type, and which variables
use this one (`%JAVA_HOME%` in `Path`); the review warns before removing one still in use.

**Process** is the environment enved started with, read-only. A value that differs from
what the saved variables give now is marked `stale`: that shell has an old copy.

Settings (theme colors, `sidebar_width` (30 by default), which variables are lists) live in
`~/.config/enved/config.toml`, or the file named by `$ENVED_CONFIG`; `enved --default-config`
prints a commented starting point. `L` keeps its choices in `%LOCALAPPDATA%\enved\lists.toml`.

enved changes the saved variables, but the shell it runs in keeps its own copy, which a
program the shell starts cannot change. `enved init pwsh` prints a PowerShell function named
`enved` that runs `enved.exe` and then applies the change to the shell as well: changed
variables get their new value, removed ones go, and `Path` is merged the way pathed's wrapper
does (added entries go to the end, entries only this session has stay). In your `$PROFILE`:

```powershell
enved.exe init pwsh | Out-String | Invoke-Expression
```

## Packages

pathed and enved share their code through this module:

| Package | What |
| --- | --- |
| `winenv` | the registry: read, check and write changes, back up, broadcast |
| `elevate` | writing through UAC: one prompt per save, the changes on the command line |
| `listedit` | the list editor, as a Bubble Tea component |
| `frame` | sidebar, panel, review, save and quit dialog around a program's tabs |
| `theme` | the settings file's `[theme]`, the styles, painted table rows |

## License

MIT
