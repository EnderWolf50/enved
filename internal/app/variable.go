package app

// What a scope's variables are, and what saving them writes: the varTab's model, apart from
// its keys and drawing (vars.go).

import (
	"strings"

	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// variable is one variable and how it changed since the last save.
type variable struct {
	name     string       // what it will be saved as
	value    winenv.Value // what it will be saved as, unless list holds its entries
	orig     *winenv.Var  // what was saved; nil for a variable added since
	removed  bool         // marked to go, still shown until the save
	typeSet  bool         // the type was chosen with x: a % typed later does not change it
	list     *listedit.Model
	listKind listedit.Kind
}

// current is the value saving would write: from the list editor, if it was opened. A value
// that gains a %VAR% becomes REG_EXPAND_SZ, unless x said otherwise.
func (v *variable) current() winenv.Value {
	cur := v.value
	if v.list != nil {
		cur.Data = winenv.Join(v.list.Result())
	}
	if !v.typeSet && strings.Contains(cur.Data, "%") {
		cur.Type = winenv.ExpandSZ
	}
	return cur
}

func (v *variable) renamed() bool { return v.orig != nil && v.name != v.orig.Name }

func (v *variable) change() theme.Change {
	switch {
	case v.removed:
		return theme.Removed
	case v.orig == nil:
		return theme.Added
	case v.renamed() || v.current() != v.orig.Value:
		return theme.Edited
	}
	return theme.Unchanged
}

// fold ends list editing of the value: the entries become its text.
func (v *variable) fold() {
	if v.list != nil {
		v.value = v.current()
		v.list = nil
	}
}

// Changes are the writes saving makes. A rename is a removal and an add.
func (t *varTab) Changes() []winenv.Change {
	var out []winenv.Change
	for _, v := range t.vars {
		cur := v.current()
		switch v.change() {
		case theme.Added:
			out = append(out, winenv.Change{Scope: t.scope, Name: v.name, New: &cur})
		case theme.Removed:
			out = append(out, winenv.Change{Scope: t.scope, Name: v.orig.Name, Old: &v.orig.Value})
		case theme.Edited:
			if v.renamed() {
				out = append(out, winenv.Change{Scope: t.scope, Name: v.orig.Name, Old: &v.orig.Value},
					winenv.Change{Scope: t.scope, Name: v.name, New: &cur})
			} else {
				out = append(out, winenv.Change{Scope: t.scope, Name: v.name, Old: &v.orig.Value, New: &cur})
			}
		}
	}
	return out
}

func (t *varTab) Dirty() bool { return len(t.Changes()) > 0 }

func (t *varTab) Pending() int {
	n := 0
	for _, v := range t.vars {
		if v.change() != theme.Unchanged {
			n++
		}
	}
	return n
}

// Warnings are the system variables saving would remove or empty.
func (t *varTab) Warnings() []string {
	var out []string
	for _, v := range t.vars {
		switch {
		case v.orig == nil || !isSystem(v.orig.Name):
		case v.removed || v.renamed():
			out = append(out, "removes "+v.orig.Name+", which Windows and many programs rely on")
		case v.current().Data == "" && v.orig.Data != "":
			out = append(out, "empties "+v.orig.Name+", which Windows and many programs rely on")
		}
	}
	return out
}

func typeName(v winenv.Value) string {
	if v.Expands() {
		return "REG_EXPAND_SZ"
	}
	return "REG_SZ"
}

// Review lists every changed variable; a list shows its entries' changes.
func (t *varTab) Review() []string {
	var lines []string
	arrow := theme.Dim.Render("  →  ")
	for _, v := range t.vars {
		cur := v.current()
		switch v.change() {
		case theme.Added:
			lines = append(lines, theme.OK.Render("  + ")+v.name+theme.Dim.Render(" = ")+cur.Data)
		case theme.Removed:
			lines = append(lines, theme.Err.Render("  - ")+v.orig.Name+theme.Dim.Render("  was  "+v.orig.Data))
			if users := t.usedBy(v.orig.Name, v); len(users) > 0 {
				lines = append(lines, theme.Warn.Render("      still used by "+strings.Join(users, ", ")))
			}
		case theme.Edited:
			name := v.name
			if v.renamed() {
				name = v.orig.Name + arrow + v.name
			}
			switch {
			case v.list != nil && v.current().Data != v.orig.Data:
				lines = append(lines, theme.Warn.Render("  ~ ")+name)
				for _, l := range v.list.Review() {
					lines = append(lines, "    "+l)
				}
			case cur.Data != v.orig.Data:
				lines = append(lines, theme.Warn.Render("  ~ ")+name+theme.Dim.Render("  ")+v.orig.Data+arrow+cur.Data)
			default:
				lines = append(lines, theme.Warn.Render("  ~ ")+name)
			}
			if cur.Type != v.orig.Type {
				lines = append(lines, theme.Dim.Render("      "+typeName(v.orig.Value)+"  →  "+typeName(cur)))
			}
		}
	}
	return lines
}

// usedBy names the variables, in any scope, whose value refers to name; skip is left out.
func (t *varTab) usedBy(name string, skip *variable) []string {
	var out []string
	for _, p := range t.peers {
		for _, v := range p.vars {
			if v != skip && !v.removed && uses(v.current().Data, name) {
				label := v.name
				if p != t {
					label += " (" + string(p.scope) + ")"
				}
				out = append(out, label)
			}
		}
	}
	return out
}
