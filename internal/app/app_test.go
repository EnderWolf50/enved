package app

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EnderWolf50/enved/elevate"
	"github.com/EnderWolf50/enved/frame"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// fakeStore keeps variables in memory. Machine needs admin, and its UAC prompt is always
// declined. It checks changes the way the registry does.
func fakeStore(user, machine map[string]string) (winenv.Store, map[winenv.Scope]map[string]winenv.Value) {
	saved := map[winenv.Scope]map[string]winenv.Value{winenv.User: {}, winenv.Machine: {}}
	for s, vars := range map[winenv.Scope]map[string]string{winenv.User: user, winenv.Machine: machine} {
		for n, v := range vars {
			saved[s][n] = winenv.Value{Data: v, Type: theme.Pick(strings.Contains(v, "%"), winenv.ExpandSZ, winenv.SZ)}
		}
	}
	lookup := func(s winenv.Scope, name string) (string, bool) {
		for n := range saved[s] {
			if strings.EqualFold(n, name) {
				return n, true
			}
		}
		return "", false
	}
	return winenv.Store{
		ReadAll: func(s winenv.Scope) ([]winenv.Var, error) {
			var out []winenv.Var
			for n, v := range saved[s] {
				out = append(out, winenv.Var{Name: n, Value: v})
			}
			winenv.Sort(out)
			return out, nil
		},
		Apply: func(changes []winenv.Change) error {
			changes = winenv.Ordered(changes)
			for _, c := range changes {
				if c.Scope == winenv.Machine {
					return elevate.ErrDeclined
				}
			}
			err := winenv.Check(changes, func(s winenv.Scope, name string) (*winenv.Value, error) {
				if n, ok := lookup(s, name); ok {
					v := saved[s][n]
					return &v, nil
				}
				return nil, nil
			})
			if err != nil {
				return err
			}
			for _, c := range changes {
				if n, ok := lookup(c.Scope, c.Name); ok {
					delete(saved[c.Scope], n)
				}
				if c.New != nil {
					saved[c.Scope][c.Name] = *c.New
				}
			}
			return nil
		},
		CanWrite: func(s winenv.Scope) bool { return s == winenv.User },
	}, saved
}

func testModel(t *testing.T, st winenv.Store) frame.Model {
	t.Helper()
	prefs, err := loadLists(defaults, filepath.Join(t.TempDir(), "lists.toml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := newModel(defaults, st, prefs, winenv.User, "")
	if err != nil {
		t.Fatal(err)
	}
	return sized(m, 140, 30)
}

func sized(m frame.Model, w, h int) frame.Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(frame.Model)
}

func press(m frame.Model, keys ...string) frame.Model {
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		next, _ := m.Update(msg)
		m = next.(frame.Model)
	}
	return m
}

func typeText(m frame.Model, text string) frame.Model {
	for _, r := range text {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(frame.Model)
	}
	return m
}

func clear(m frame.Model, n int) frame.Model {
	for range n {
		m = press(m, "backspace")
	}
	return m
}

// save presses enter on the review and runs the save it starts, as the program would.
func save(t *testing.T, m frame.Model, keys ...string) frame.Model {
	t.Helper()
	if len(keys) == 0 {
		keys = []string{"enter"}
	}
	var cmd tea.Cmd
	for _, k := range keys {
		var next tea.Model
		next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: k})
		m = next.(frame.Model)
	}
	if cmd == nil {
		t.Fatal("enter on the review did not start saving")
	}
	next, _ := m.Update(cmd())
	return next.(frame.Model)
}

func screen(m frame.Model) string { return ansi.Strip(m.View().Content) }

func vars(m frame.Model) *varTab { return m.Tab().(*varTab) }

// cursorOn moves the cursor to the variable named name.
func cursorOn(t *testing.T, m frame.Model, name string) frame.Model {
	t.Helper()
	vt := vars(m)
	v := vt.find(name, nil)
	if v == nil {
		t.Fatalf("no variable %s", name)
	}
	vt.point(v)
	return m
}

func TestEditAddRenameRemoveAndSave(t *testing.T) {
	st, saved := fakeStore(map[string]string{"EDITOR": "vi", "GOPATH": `%USERPROFILE%\go`, "OLD": "1", "TEMP": `C:\t`}, nil)
	m := testModel(t, st)
	m = press(m, "enter") // into the User table

	// Edit a value: enter opens the dialog on it.
	m = cursorOn(t, m, "EDITOR")
	m = press(m, "enter")
	if vars(m).dialog == nil {
		t.Fatal("enter did not open the dialog")
	}
	m = clear(m, 2)
	m = typeText(m, "nvim")
	m = press(m, "enter")

	// Add one: a name, enter, a value, enter.
	m = press(m, "a")
	m = typeText(m, "NEW_ONE")
	m = press(m, "enter")
	m = typeText(m, `%GOPATH%\bin`)
	m = press(m, "enter")
	if v := vars(m).find("NEW_ONE", nil); v == nil || !v.current().Expands() {
		t.Fatalf("NEW_ONE not added as REG_EXPAND_SZ: %+v", v)
	}

	// Rename OLD, remove GOPATH (NEW_ONE still uses it: the review says so).
	m = cursorOn(t, m, "OLD")
	m = press(m, "r")
	m = clear(m, 3)
	m = typeText(m, "RENAMED")
	m = press(m, "enter")
	m = cursorOn(t, m, "GOPATH")
	m = press(m, "d")

	m = press(m, "s")
	review := screen(m)
	if !strings.Contains(review, "what saving will write") {
		t.Fatalf("s did not show the review:\n%s", review)
	}
	for _, want := range []string{"~ EDITOR", "vi  →  nvim", "+ NEW_ONE", "- GOPATH", "still used by NEW_ONE", "OLD  →  RENAMED"} {
		if !strings.Contains(review, want) {
			t.Errorf("the review lacks %q:\n%s", want, review)
		}
	}
	m = save(t, m)
	u := saved[winenv.User]
	if u["EDITOR"].Data != "nvim" || u["NEW_ONE"].Data != `%GOPATH%\bin` || u["RENAMED"].Data != "1" {
		t.Fatalf("saved %v", u)
	}
	if _, ok := u["OLD"]; ok {
		t.Fatal("OLD is still there after the rename")
	}
	if _, ok := u["GOPATH"]; ok {
		t.Fatal("GOPATH was not removed")
	}
	if vars(m).Dirty() || !strings.Contains(screen(m), "User variables saved") {
		t.Fatalf("after saving: dirty %v, screen:\n%s", vars(m).Dirty(), screen(m))
	}
}

func TestPathInTheListEditor(t *testing.T) {
	st, saved := fakeStore(map[string]string{"Path": `C:\a;C:\b`}, nil)
	m := testModel(t, st)
	m = press(m, "enter")
	m = cursorOn(t, m, "Path")
	m = press(m, "enter")
	vt := vars(m)
	if vt.open == nil {
		t.Fatal("enter on Path did not open the list editor")
	}
	if !strings.Contains(screen(m), "User › Path") {
		t.Fatalf("the list editor's heading is not there:\n%s", screen(m))
	}
	// a adds after the cursor, d removes C:\b; esc goes back to the variables.
	m = press(m, "a")
	m = typeText(m, `%USERPROFILE%\bin`)
	m = press(m, "enter", "j", "d", "esc")
	if vt.open != nil {
		t.Fatal("esc did not leave the list editor")
	}
	if got := vt.find("Path", nil).current(); got.Data != `C:\a;%USERPROFILE%\bin` || !got.Expands() {
		t.Fatalf("Path is now %+v", got)
	}
	// Opening it again keeps the marks.
	m = press(m, "enter")
	if vt.open.list.Pending() != 2 {
		t.Fatalf("the list editor forgot its changes: %d pending", vt.open.list.Pending())
	}
	m = press(m, "s")
	review := screen(m)
	if !strings.Contains(review, "+ #2") || !strings.Contains(review, "- #3") || !strings.Contains(review, `C:\b`) {
		t.Fatalf("the review does not show the entries:\n%s", review)
	}
	m = save(t, m)
	if saved[winenv.User]["Path"].Data != `C:\a;%USERPROFILE%\bin` {
		t.Fatalf("saved Path %q", saved[winenv.User]["Path"].Data)
	}
}

func TestMachineDeclinedKeepsChanges(t *testing.T) {
	st, saved := fakeStore(map[string]string{"A": "1"}, map[string]string{"M": "x"})
	m := testModel(t, st)
	if !strings.Contains(screen(m), "Machine  admin") {
		t.Fatal("the sidebar does not say Machine asks for UAC")
	}
	m = press(m, "j", "enter", "d", "esc", "k", "enter", "d", "s")
	m = save(t, m)
	if _, ok := saved[winenv.User]["A"]; ok {
		t.Fatal("User was not saved")
	}
	if !strings.Contains(screen(m), "Machine variables not saved") {
		t.Fatalf("outcome:\n%s", screen(m))
	}
	m = press(m, "esc", "esc", "j")
	if !m.Tab().(*varTab).Dirty() {
		t.Fatal("Machine lost its change")
	}
	m = press(m, "q")
	if !strings.Contains(screen(m), "Quit without saving?") {
		t.Fatal("quitting with a change did not ask")
	}
}

func TestSystemVariableAsksTwice(t *testing.T) {
	st, saved := fakeStore(map[string]string{"TEMP": `C:\t`}, nil)
	m := testModel(t, st)
	m = press(m, "enter", "d", "s")
	if !strings.Contains(screen(m), "! removes TEMP") {
		t.Fatalf("no warning in the review:\n%s", screen(m))
	}
	m = press(m, "enter")
	if !strings.Contains(screen(m), "Save the changes marked ! as well?") {
		t.Fatal("the first enter did not ask again")
	}
	m = press(m, "esc")
	if !strings.Contains(screen(m), "what saving will write") {
		t.Fatal("esc on the second question left the review")
	}
	m = save(t, m, "enter", "y")
	if _, ok := saved[winenv.User]["TEMP"]; ok {
		t.Fatal("TEMP was not removed after the second yes")
	}
}

func TestTypeAndListToggles(t *testing.T) {
	st, saved := fakeStore(map[string]string{"DIRS": `C:\a;C:\b`, "PCT": "50%"}, nil)
	m := testModel(t, st)
	m = press(m, "enter")
	m = cursorOn(t, m, "DIRS")
	m = press(m, "enter")
	if vars(m).open != nil || vars(m).dialog == nil {
		t.Fatal("DIRS opened as a list before L said so")
	}
	// L only marks it a list, and keeps that; enter then opens the list editor.
	m = press(m, "esc", "L")
	if vars(m).open != nil || vars(m).dialog != nil {
		t.Fatal("L opened an editor")
	}
	prefs := vars(m).prefs
	again, _ := loadLists(defaults, prefs.path)
	if _, isList := again.kind("dirs"); !isList {
		t.Fatal("the choice was not kept")
	}
	m = press(m, "enter", "a")
	m = typeText(m, `C:\c`)
	m = press(m, "enter", "esc", "enter")
	if vars(m).open == nil || vars(m).open.list.Pending() != 1 {
		t.Fatal("enter did not open DIRS as a list again, with its change")
	}
	// e edits a list as one value, with the entries' changes in it, and leaves it a list.
	m = press(m, "esc", "e")
	if vars(m).open != nil || vars(m).dialog == nil || vars(m).dialog.value.Value() != `C:\a;C:\c;C:\b` {
		t.Fatalf("e did not edit DIRS as text: %+v", vars(m).dialog)
	}
	m = press(m, "esc", "L")
	if _, isList := prefs.kind("DIRS"); isList {
		t.Fatal("DIRS is still a list after a second L")
	}

	// x switches the type; after it, the % in the value does not switch it back.
	m = cursorOn(t, m, "PCT")
	m = press(m, "x")
	if v := vars(m).find("PCT", nil); !v.typeSet || v.current().Expands() {
		t.Fatalf("x: %+v", v.current())
	}
	m = press(m, "s")
	m = save(t, m)
	if saved[winenv.User]["PCT"].Type != winenv.SZ || saved[winenv.User]["PCT"].Data != "50%" {
		t.Fatalf("PCT saved as %+v", saved[winenv.User]["PCT"])
	}
}

// u takes back one change at a time, the list editor's included; U all of them; z redoes.
func TestUndoOneStepAtATime(t *testing.T) {
	st, _ := fakeStore(map[string]string{"A": "1", "B": "2", "Path": `C:\a;C:\b`}, nil)
	m := testModel(t, st)
	m = press(m, "enter")
	m = cursorOn(t, m, "A")
	m = press(m, "d") // 1: remove A
	m = cursorOn(t, m, "B")
	m = press(m, "r") // 2: rename B to C
	m = clear(m, 1)
	m = typeText(m, "C")
	m = press(m, "enter")
	m = cursorOn(t, m, "Path")
	m = press(m, "enter", "j", "d", "esc") // 3: remove C:\b from Path
	vt := vars(m)
	state := func() string {
		var out []string
		for _, v := range vt.vars {
			out = append(out, v.change().Sign()+v.name+"="+v.current().Data)
		}
		return strings.Join(out, " ")
	}
	want := []string{
		`-A=1 ~C=2 ~Path=C:\a`,
		`-A=1 ~C=2  Path=C:\a;C:\b`,
		`-A=1  B=2  Path=C:\a;C:\b`,
		` A=1  B=2  Path=C:\a;C:\b`,
	}
	if got := state(); got != want[0] {
		t.Fatalf("before undo: %s", got)
	}
	m = cursorOn(t, m, "Path")
	m = press(m, "enter", "u") // in the list editor
	if got := state(); got != want[1] {
		t.Fatalf("u in the list editor: %s", got)
	}
	m = press(m, "esc", "u", "u")
	if got := state(); got != want[3] {
		t.Fatalf("u, u: %s", got)
	}
	m = press(m, "u")
	if vt.Status() != "nothing to undo" {
		t.Fatalf("status %q", vt.Status())
	}
	// z redoes them in turn.
	m = press(m, "z", "z", "z")
	if got := state(); got != want[0] {
		t.Fatalf("z, z, z: %s", got)
	}
	// A new change ends what z could redo.
	m = cursorOn(t, m, "Path")
	m = press(m, "x", "z")
	if vt.Status() != "nothing to redo" {
		t.Fatalf("z after a new change: status %q", vt.Status())
	}
}

func TestCopyNameAndValue(t *testing.T) {
	st, _ := fakeStore(map[string]string{"A": "1"}, nil)
	m := testModel(t, st)
	for k, want := range map[string]string{"v": "1", "n": "A"} {
		next, cmd := press(m, "enter").Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(frame.Model)
		if cmd == nil || !strings.Contains(fmt.Sprint(cmd()), want) {
			t.Errorf("%s copied %v, want %q", k, cmd(), want)
		}
		m = press(m, "esc")
	}
}

// The sidebar's counts line up whatever else a row shows.
func TestSidebarCountsAlign(t *testing.T) {
	st, _ := fakeStore(map[string]string{"A": "1", "B": "2"}, map[string]string{"M": "x", "N": "y", "O": "z"})
	m := testModel(t, st)
	m = press(m, "enter", "d", "esc") // User has a change
	cols := map[string]int{}
	for _, line := range strings.Split(screen(m), "\n") {
		for _, name := range []string{"User", "Machine"} {
			side, _, _ := strings.Cut(line, "││")
			if strings.Contains(side, " "+name+" ") {
				cols[name] = utf8.RuneCountInString(side[:strings.LastIndexAny(side, "0123456789")])
			}
		}
	}
	if cols["User"] == 0 || cols["User"] != cols["Machine"] {
		t.Fatalf("the counts end at %v:\n%s", cols, screen(m))
	}
}

func TestProcessStale(t *testing.T) {
	st, _ := fakeStore(map[string]string{"A": "new", "Path": `C:\u`}, map[string]string{"Path": `C:\m`, "B": "same"})
	m := testModel(t, st)
	var pt *processTab
	for range 2 {
		m = press(m, "j")
	}
	pt = m.Tab().(*processTab)
	pt.environ = func() []string {
		return []string{"A=old", "B=same", `PATH=C:\m;C:\shell-only`, "SESSION=1", "=C:=C:\\"}
	}
	pt.Load()
	states := map[string]string{}
	for _, v := range pt.vars {
		states[v.Name] = pt.state(v)
	}
	want := map[string]string{"A": "stale", "B": "", "PATH": "stale", "SESSION": "here only"}
	for n, s := range want {
		if states[n] != s {
			t.Errorf("%s: %q, want %q", n, states[n], s)
		}
	}
	if len(pt.vars) != 4 {
		t.Errorf("%d variables; the =C: entry should be skipped", len(pt.vars))
	}
	m = press(m, "enter", "d")
	if !strings.Contains(pt.Status(), "read-only") {
		t.Fatal("the Process tab took an edit")
	}
}

func TestEditOpensOnTheVariable(t *testing.T) {
	st, _ := fakeStore(map[string]string{"A": "1", "PATHEXT": ".COM;.EXE"}, nil)
	prefs, _ := loadLists(defaults, "")
	m, err := newModel(defaults, st, prefs, winenv.User, "pathext")
	if err != nil {
		t.Fatal(err)
	}
	m = sized(m, 120, 30)
	if vars(m).open == nil || !strings.Contains(screen(m), "User › PATHEXT") {
		t.Fatalf("edit did not open PATHEXT's list:\n%s", screen(m))
	}
	m = press(m, "a")
	m = typeText(m, "PS1")
	if !strings.Contains(screen(m), "an extension is a dot") {
		t.Fatalf("a bad extension was not flagged:\n%s", screen(m))
	}
	if _, err := newModel(defaults, st, prefs, winenv.User, "NOPE"); err == nil {
		t.Fatal("edit of a missing variable did not fail")
	}
}

func TestCLI(t *testing.T) {
	st, saved := fakeStore(map[string]string{"A": "1", "Path": `C:\x`, "E": "%A%"}, nil)
	if err := runAs("enved", []string{"set", "a", "2"}, st); err != nil {
		t.Fatal(err)
	}
	if saved[winenv.User]["A"].Data != "2" {
		t.Fatalf("set: %v", saved[winenv.User])
	}
	if err := runAs("enved", []string{"set", "--no-expand", "NEW", "--", "-50%"}, st); err != nil {
		t.Fatal(err)
	}
	if v := saved[winenv.User]["NEW"]; v.Data != "-50%" || v.Type != winenv.SZ {
		t.Fatalf("set --no-expand: %+v", v)
	}
	if err := runAs("enved", []string{"set", "E", "%B%"}, st); err != nil || !saved[winenv.User]["E"].Expands() {
		t.Fatalf("set kept the type? %v %+v", err, saved[winenv.User]["E"])
	}
	if err := runAs("enved", []string{"unset", "Path"}, st); err == nil {
		t.Fatal("unset of Path without --force did not fail")
	}
	if err := runAs("enved", []string{"unset", "Path", "--force"}, st); err != nil {
		t.Fatal(err)
	}
	if err := runAs("enved", []string{"get", "Path"}, st); err == nil {
		t.Fatal("get of a removed variable did not fail")
	}
	if err := runAs("enved", []string{"set", "-m", "M", "1"}, st); err == nil {
		t.Fatal("a declined UAC prompt did not fail set -m")
	}
	if err := runAs("enved", []string{"set", "BAD=NAME", "1"}, st); err == nil {
		t.Fatal("a name with = was accepted")
	}
}

func TestShellInit(t *testing.T) {
	for _, prog := range []string{"enved", "pathed"} {
		for _, shell := range []string{"pwsh", "PowerShell"} {
			got, err := shellInit(prog, shell)
			if err != nil || !strings.Contains(got, "function "+prog+" {") || !strings.Contains(got, prog+".exe @args") || strings.Contains(got, "__PROG__") {
				t.Errorf("shellInit(%q, %q) = %q, %v; want %s's wrapper", prog, shell, got, err, prog)
			}
		}
	}
	if _, err := shellInit("enved", "bash"); err == nil {
		t.Error("shellInit(bash) did not fail")
	}
}

// The first frame is drawn before any WindowSizeMsg, and a terminal can be tiny.
func TestViewAtAnySize(t *testing.T) {
	st, _ := fakeStore(map[string]string{"A": "1", "Path": `C:\a`}, map[string]string{"B": "2"})
	prefs, _ := loadLists(defaults, "")
	for _, size := range [][2]int{{0, 0}, {10, 3}, {30, 8}, {200, 60}} {
		m, _ := newModel(defaults, st, prefs, winenv.User, "")
		m.View()
		m = sized(m, size[0], size[1])
		m.View()
		for _, keys := range [][]string{{"enter", "a"}, {"esc", "j", "enter"}, {"esc", "d", "s"}, {"esc", "esc", "j", "j", "enter"}, {"esc", "q"}} {
			m = press(m, keys...)
			m.View()
		}
	}
}

// A background holds only until the next reset, so the cursor row must paint every run of
// text itself, padding and the panel's full width included.
func TestCursorRowPaintedEdgeToEdge(t *testing.T) {
	token := regexp.MustCompile(`\x1b\[([0-9;]*)m|[^\x1b]+`)
	st, _ := fakeStore(map[string]string{"AAA": "1", "BBB": "%AAA%"}, nil)
	for _, w := range []int{90, 120, 200} {
		m := testModel(t, st)
		m = sized(m, w, 20)
		m = press(m, "enter", "j", "d") // cursor on a removed variable
		for _, line := range strings.Split(vars(m).Table.View(), "\n") {
			if !strings.Contains(ansi.Strip(line), "BBB") {
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

func TestConfigListsOverrideGuesses(t *testing.T) {
	c, err := parseTestConfig(`lists = ["MY_DIRS"]` + "\n" + `not_lists = ["LIB"]`)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := loadLists(c, "")
	if _, isList := p.kind("my_dirs"); !isList {
		t.Error("lists did not make MY_DIRS a list")
	}
	if _, isList := p.kind("LIB"); isList {
		t.Error("not_lists did not make LIB text")
	}
	if kind, isList := p.kind("Path"); !isList || kind == 0 {
		t.Error("Path is not a list of folders")
	}
}

// The review scrolls when the changes do not fit, rather than cutting them off.
func TestReviewScrolls(t *testing.T) {
	vars := map[string]string{}
	for i := range 40 {
		vars[fmt.Sprintf("V%02d", i)] = "1"
	}
	st, _ := fakeStore(vars, nil)
	m := sized(testModel(t, st), 100, 20)
	m = press(m, "enter")
	for range 40 {
		m = press(m, "d", "j")
	}
	m = press(m, "s")
	if !strings.Contains(screen(m), "- V00") || strings.Contains(screen(m), "- V39") || !strings.Contains(screen(m), "scroll") {
		t.Fatalf("the top of the review:\n%s", screen(m))
	}
	for range 40 {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = next.(frame.Model)
	}
	if !strings.Contains(screen(m), "- V39") {
		t.Fatalf("scrolled down, the last change is not there:\n%s", screen(m))
	}
}
