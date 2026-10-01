package theme

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

func TestDividerFitsItsWidth(t *testing.T) {
	legend := Legend(true, "", "missing")
	for _, w := range []int{0, 10, 40, 80, 120} {
		if got := lipgloss.Width(Divider(w, legend)); got != w {
			t.Errorf("width %d: the divider is %d wide", w, got)
		}
	}
	if !strings.Contains(Divider(80, legend), "missing") {
		t.Error("the legend is not on a wide divider")
	}
}

func TestNoteTintsOnlyUnchangedRows(t *testing.T) {
	if NewRow(Unchanged, Info, false).bg.GetBackground() != bgNote || NewRow(Unchanged, Problem, false).bg.GetBackground() != bgProblem {
		t.Error("an unchanged row with a note does not have the note's tint")
	}
	if NewRow(Removed, Problem, false).bg.GetBackground() != bgRemoved {
		t.Error("a change's tint does not win over the note's")
	}
	if NewRow(Unchanged, Info, true).bg.GetBackground() != bgNoteCursor {
		t.Error("the cursor on a noted row lacks the note's cursor tint")
	}
}

func TestHelpLinesKeepGroupsTogether(t *testing.T) {
	b := func(k, what string) key.Binding { return key.NewBinding(key.WithKeys(k), key.WithHelp(k, what)) }
	nav, edit := []key.Binding{b("j", "down"), b("k", "up")}, []key.Binding{b("a", "add"), b("d", "remove"), b("r", "rename")}
	h := help.New()
	if lines := HelpLines(h, 200, nav, edit); len(lines) != 1 || !strings.Contains(lines[0], "│") {
		t.Fatalf("wide: %q", lines)
	}
	lines := HelpLines(h, 30, nav, edit)
	if len(lines) != 2 || strings.Contains(lines[0], "add") {
		t.Fatalf("narrow: a group was split or joined: %q", lines)
	}
	for _, l := range HelpLines(h, 12, edit) {
		if lipgloss.Width(l) > 12 || strings.Contains(l, "…") {
			t.Fatalf("a group wider than the line was cut, not wrapped: %q", l)
		}
	}
}
