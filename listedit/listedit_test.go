package listedit

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestResetAlignsTextEdits(t *testing.T) {
	// The value was edited as text: b went, c came, a moved to the end.
	m := New("t", Text, []string{"a", "b"}, []string{"c", "a"}, nil)
	var got []string
	for _, e := range m.entries {
		got = append(got, e.change().Sign()+e.value)
	}
	if want := []string{"+c", "-b", " a"}; !slices.Equal(got, want) {
		t.Fatalf("entries %q, want %q", got, want)
	}
	if !slices.Equal(m.Result(), []string{"c", "a"}) || !m.Dirty() {
		t.Fatalf("result %q", m.Result())
	}
	m.Undo()
	if m.Dirty() {
		t.Fatal("Undo left changes")
	}
}

func TestCleanByKind(t *testing.T) {
	ext := New("t", Extensions, []string{".EXE", "PS1", ".exe", ".BAT"}, []string{".EXE", "PS1", ".exe", ".BAT"}, nil)
	if n := ext.Clean(); n != 2 || !slices.Equal(ext.Result(), []string{".EXE", ".BAT"}) {
		t.Fatalf("clean removed %d: %q", n, ext.Result())
	}
	dirs := []string{`C:\a`, `C:\gone`, `c:\A\`, `C:\b`}
	exists := func(e string) bool { return e != `C:\gone` }
	f := New("t", Folders, dirs, dirs, exists)
	if n := f.Clean(); n != 2 || !slices.Equal(f.Result(), []string{`C:\a`, `C:\b`}) {
		t.Fatalf("clean removed %d: %q", n, f.Result())
	}
	text := New("t", Text, []string{"x", "X", "y"}, []string{"x", "X", "y"}, nil)
	if n := text.Clean(); n != 1 {
		t.Fatalf("text clean removed %d", n)
	}
}

func TestKnown(t *testing.T) {
	for name, want := range map[string]Kind{"PATH": Folders, "PathExt": Extensions, "CLASSPATH": Paths} {
		if k, ok := Known(name); !ok || k != want {
			t.Errorf("Known(%s) = %v, %v", name, k, ok)
		}
	}
	if _, ok := Known("JAVA_HOME"); ok {
		t.Error("JAVA_HOME is not a list")
	}
}

// A background holds only until the next reset, so the cursor row must paint every run of
// text itself, padding and the panel's full width included.
func TestCursorRowPaintedEdgeToEdge(t *testing.T) {
	token := regexp.MustCompile(`\x1b\[([0-9;]*)m|[^\x1b]+`)
	entries := []string{`C:\a`, `C:\missing`}
	for _, w := range []int{60, 90, 170} {
		m := New("t", Folders, entries, entries, func(e string) bool { return e == `C:\a` })
		m.Resize(w, 20)
		m.Focus(true)
		m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"}) // cursor on a removed, missing entry
		for _, line := range strings.Split(m.table.View(), "\n") {
			if !strings.Contains(ansi.Strip(line), `C:\missing`) {
				continue
			}
			active, bare := false, 0
			for _, tok := range token.FindAllStringSubmatch(line, -1) {
				if strings.HasPrefix(tok[0], "\x1b") {
					active = tok[1] != "" && tok[1] != "0" && (active || strings.Contains(tok[1], "48;"))
				} else if !active {
					bare += len(tok[0])
				}
			}
			if bare > 0 {
				t.Errorf("width %d: %d cells of the cursor row have no background", w, bare)
			}
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{` C:\x `: `C:\x`, `%A%\b`: `%A%\b`, `\\srv\share`: `\\srv\share`} {
		if got := Folders.normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Text.normalize(" a b "); got != "a b" {
		t.Errorf("text normalize = %q", got)
	}
}
