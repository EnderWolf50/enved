package winenv

import (
	"errors"
	"slices"
	"testing"
)

func TestCheck(t *testing.T) {
	a, b := &Value{"a", SZ}, &Value{"b", SZ}
	now := map[string]*Value{"Path": a}
	current := func(_ Scope, name string) (*Value, error) { return now[name], nil }

	// A rename that only changes case: the removal frees the name for the add.
	rename := Ordered([]Change{{Scope: User, Name: "PATH", New: a}, {Scope: User, Name: "Path", Old: a}})
	if rename[0].New != nil {
		t.Fatal("Ordered did not put the removal first")
	}
	if err := Check(rename, current); err != nil {
		t.Fatalf("case-only rename: %v", err)
	}
	// A value that is not what was read any more.
	if err := Check([]Change{{Scope: User, Name: "Path", Old: b, New: a}}, current); !errors.Is(err, ErrChanged) {
		t.Fatalf("changed value: %v", err)
	}
	// An add over a variable something else created.
	if err := Check([]Change{{Scope: User, Name: "path", New: b}}, func(Scope, string) (*Value, error) { return a, nil }); !errors.Is(err, ErrChanged) {
		t.Fatalf("add over an existing variable: %v", err)
	}
	// The same name in another scope is another variable.
	if err := Check([]Change{{Scope: Machine, Name: "Path", New: b}}, func(s Scope, _ string) (*Value, error) {
		return map[Scope]*Value{User: a}[s], nil
	}); err != nil {
		t.Fatalf("add in another scope: %v", err)
	}
}

func TestSplitJoinSort(t *testing.T) {
	if got := Split(` C:\a ;;C:\b;`); !slices.Equal(got, []string{`C:\a`, `C:\b`}) {
		t.Errorf("Split = %q", got)
	}
	if Join([]string{"a", "b"}) != "a;b" {
		t.Error("Join")
	}
	vars := []Var{{Name: "b"}, {Name: "A"}, {Name: "a"}}
	Sort(vars)
	if vars[0].Name != "A" || vars[1].Name != "a" || vars[2].Name != "b" {
		t.Errorf("Sort = %v", vars)
	}
}
