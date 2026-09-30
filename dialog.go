package main

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// What the dialog asks for.
type dialogMode int

const (
	dialogAdd   dialogMode = iota // a name, then a value
	dialogValue                   // a new value
	dialogName                    // a new name
)

// varDialog asks for a variable's name, value or both, over the table, checking them as
// they are typed.
type varDialog struct {
	mode        dialogMode
	v           *variable // the variable changed; nil when adding
	name, value textinput.Model
	onValue     bool // adding: the value field has the keys
}

func (t *varTab) openDialog(mode dialogMode, v *variable) tea.Cmd {
	field := func(placeholder, value string) textinput.Model {
		f := textinput.New()
		f.Prompt = "› "
		f.Placeholder = placeholder
		f.SetWidth(min(70, max(t.w-16, 20)))
		f.SetValue(value)
		f.CursorEnd()
		return f
	}
	d := &varDialog{mode: mode, v: v}
	name, value := "", ""
	if v != nil {
		v.fold()
		name, value = v.name, v.value.Data
	}
	d.name, d.value = field("NAME", name), field(`a value, %VARS% allowed`, value)
	t.dialog = d
	if mode == dialogValue {
		d.onValue = true
		return d.value.Focus()
	}
	return d.name.Focus()
}

func (d *varDialog) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if d.onValue {
		d.value, cmd = d.value.Update(msg)
	} else {
		d.name, cmd = d.name.Update(msg)
	}
	return cmd
}

// nameProblem is what is wrong with the typed name, or "".
func (d *varDialog) nameProblem(t *varTab) string {
	n := d.name.Value()
	if p := validName(n); p != "" {
		return p
	}
	if other := t.find(n, d.v); other != nil {
		return "there is already a variable " + other.name
	}
	return ""
}

func (d *varDialog) view(t *varTab) string {
	title := map[dialogMode]string{dialogAdd: "Add a " + string(t.scope) + " variable",
		dialogValue: "Edit " + d.name.Value(), dialogName: "Rename " + d.name.Value()}[d.mode]
	if d.v != nil && d.mode == dialogName {
		title = "Rename " + d.v.name
	}
	var lines []string
	if d.mode != dialogValue {
		check := theme.OK.Render("ok")
		if p := d.nameProblem(t); p != "" {
			check = theme.Err.Render(p)
		}
		lines = append(lines, theme.Dim.Render("name"), d.name.View(), check, "")
	}
	if d.mode != dialogName {
		v := d.value.Value()
		note := theme.Dim.Render("REG_SZ")
		if d.expands(v) {
			note = theme.Dim.Render("REG_EXPAND_SZ")
			if x := winenv.Expand(v); x != v {
				note += theme.Dim.Render(" → " + x)
			}
		}
		lines = append(lines, theme.Dim.Render("value"), d.value.View(), note, "")
	}
	help := "enter keep · esc cancel"
	if d.mode == dialogAdd {
		help = "tab name/value · enter next, then keep · esc cancel"
	}
	return theme.Dialog.Render(theme.Accent.Render(title) + "\n\n" + strings.Join(lines, "\n") + "\n" + theme.Dim.Render(help))
}

// expands says what type the typed value would be saved as.
func (d *varDialog) expands(v string) bool {
	if d.v != nil && d.v.typeSet {
		return d.v.value.Expands()
	}
	return strings.Contains(v, "%") || (d.v != nil && d.v.value.Expands())
}

// The dialog: only its own keys count while it is open.
func (t *varTab) updateDialog(msg tea.KeyPressMsg) tea.Cmd {
	d := t.dialog
	switch msg.String() {
	case "esc":
		t.dialog = nil
		return nil
	case "tab", "shift+tab":
		if d.mode == dialogAdd {
			return d.switchField()
		}
		return nil
	case "enter":
		if d.mode == dialogAdd && !d.onValue {
			return d.switchField()
		}
		if d.mode != dialogValue {
			if p := d.nameProblem(t); p != "" {
				t.status = p
				return nil
			}
		}
		t.dialog = nil
		t.keep(d)
		return nil
	}
	return d.update(msg)
}

func (d *varDialog) switchField() tea.Cmd {
	d.onValue = !d.onValue
	if d.onValue {
		d.name.Blur()
		return d.value.Focus()
	}
	d.value.Blur()
	return d.name.Focus()
}

// keep applies what the dialog was given.
func (t *varTab) keep(d *varDialog) {
	v := d.v
	switch d.mode {
	case dialogAdd:
		typ := pick(strings.Contains(d.value.Value(), "%"), winenv.ExpandSZ, winenv.SZ)
		v = &variable{name: d.name.Value(), value: winenv.Value{Data: d.value.Value(), Type: typ}}
		at, _ := slices.BinarySearchFunc(t.vars, v, func(a, b *variable) int {
			return strings.Compare(winenv.Key(a.name), winenv.Key(b.name))
		})
		t.vars = slices.Insert(t.vars, at, v)
	case dialogValue:
		v.value.Data = d.value.Value()
	case dialogName:
		v.name = d.name.Value()
		v.fold() // its editor's title and kind follow the name: a new one opens next time
		if v.orig != nil && v.orig.Name == v.name {
			t.status = "the name is as it was"
		}
	}
	t.refresh()
	t.point(v)
	if d.mode == dialogAdd {
		t.status = fmt.Sprintf("added %s", v.name)
	}
}
