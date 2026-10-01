package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/EnderWolf50/enved/winenv"
)

func TestCleanPath(t *testing.T) {
	in := []string{`C:\a`, `C:\gone`, `c:\A\`, `C:\b`, `C:\a`}
	exists := func(e string) bool { return e != `C:\gone` }
	if got, want := cleanPath(in, exists), []string{`C:\a`, `C:\b`}; !slices.Equal(got, want) {
		t.Fatalf("cleanPath = %q, want %q", got, want)
	}
}

func TestPathCLI(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.Mkdir(a, 0o755)
	os.Mkdir(b, 0o755)
	st, saved := fakeStore(map[string]string{"Path": a}, nil)
	path := func() []string { return winenv.Split(saved[winenv.User]["Path"].Data) }
	if err := runAs("enved", []string{"path", "add", b, "--front"}, st); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(path(), []string{b, a}) {
		t.Fatalf("add --front: %q", path())
	}
	if err := runAs("enved", []string{"path", "add", b}, st); err == nil {
		t.Fatal("adding a folder twice did not fail")
	}
	if err := runAs("enved", []string{"path", "rm", "1"}, st); err != nil || !slices.Equal(path(), []string{a}) {
		t.Fatalf("rm 1: %v, %q", err, path())
	}
	if err := runAs("enved", []string{"path", "add", filepath.Join(dir, "nope")}, st); err == nil {
		t.Fatal("adding a missing folder did not fail")
	}
	if err := runAs("enved", []string{"path", "add", b, "-m"}, st); err == nil {
		t.Fatal("a declined UAC prompt did not fail path add -m")
	}
	if err := runAs("enved", []string{"path", "nope"}, st); err == nil || !strings.Contains(err.Error(), "pathed list") {
		t.Fatalf("an unknown path command: %v", err)
	}
	// pathed is enved path.
	if err := runAs("pathed", []string{"add", b}, st); err != nil || !slices.Equal(path(), []string{a, b}) {
		t.Fatalf("pathed add: %v, %q", err, path())
	}
}

// pathed's editor: the User and Machine PATHs, each in the list editor.
func TestPathEditor(t *testing.T) {
	st, saved := fakeStore(map[string]string{"Path": `C:\a;C:\b`}, map[string]string{"Path": `C:\Windows`})
	exists := func(p string) bool { return p != `C:\b` }
	m := sized(newPathModel(st, exists, winenv.User), 120, 30)
	if s := screen(m); !strings.Contains(s, "User") || !strings.Contains(s, "Machine  admin") {
		t.Fatalf("the sidebar:\n%s", s)
	}
	m = press(m, "enter", "c", "s") // clean marks the missing C:\b
	m = save(t, m)
	if got := saved[winenv.User]["Path"].Data; got != `C:\a` {
		t.Fatalf("saved %q", got)
	}
}

func TestPathChangeKeepsType(t *testing.T) {
	sz := &winenv.Var{Name: "PATH", Value: winenv.Value{Data: `C:\a`, Type: winenv.SZ}}
	if c := pathChange(winenv.User, sz, []string{`C:\b`}); c.New.Type != winenv.SZ || c.Name != "PATH" {
		t.Errorf("a REG_SZ PATH without %%VARS%% became %+v", c)
	}
	if c := pathChange(winenv.User, sz, []string{`%X%\b`}); c.New.Type != winenv.ExpandSZ {
		t.Error("a PATH that gains a %VAR% did not become REG_EXPAND_SZ")
	}
	if c := pathChange(winenv.User, nil, []string{`C:\b`}); c.New.Type != winenv.ExpandSZ || c.Old != nil {
		t.Errorf("a new PATH: %+v", c)
	}
}

func TestShellInitDefinesPathed(t *testing.T) {
	got, _ := shellInit("pathed", "pwsh")
	if !strings.Contains(got, "function pathed {") || !strings.Contains(got, "pathed.exe @args") {
		t.Error("pathed init pwsh does not define pathed")
	}
}
