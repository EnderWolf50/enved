package listedit

import (
	"slices"
	"testing"
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
