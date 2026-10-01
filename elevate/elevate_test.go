package elevate

import (
	"strings"
	"testing"

	"github.com/EnderWolf50/enved/winenv"
)

func TestEncodeRoundTrip(t *testing.T) {
	in := []winenv.Change{
		{Scope: winenv.Machine, Name: "Path", Old: &winenv.Value{Data: `D:\a`, Type: 2}, New: &winenv.Value{Data: `D:\a;%ProgramFiles%\Tool "x"`, Type: 2}},
		{Scope: winenv.Machine, Name: "GONE", Old: &winenv.Value{Data: "x", Type: 1}},
	}
	arg, err := encode(in)
	if err != nil || strings.ContainsAny(arg, " \"") {
		t.Fatalf("argument %q (%v) would need quoting on a command line", arg, err)
	}
	back, err := decode(arg)
	if err != nil || len(back) != 2 || back[0].New.Data != in[0].New.Data || back[1].New != nil || back[1].Old.Type != 1 {
		t.Fatalf("round trip: %+v, %v", back, err)
	}
}

func TestBatches(t *testing.T) {
	big := strings.Repeat("x", 9000)
	var changes []winenv.Change
	for range 5 {
		changes = append(changes, winenv.Change{Scope: winenv.Machine, Name: "V", New: &winenv.Value{Data: big}})
	}
	runs, err := batches(changes)
	if err != nil || len(runs) < 2 {
		t.Fatalf("%d runs, %v; want the changes split", len(runs), err)
	}
	n := 0
	for _, r := range runs {
		if len(r) > maxArg {
			t.Errorf("a run of %d characters", len(r))
		}
		got, _ := decode(r)
		n += len(got)
	}
	if n != 5 {
		t.Errorf("%d changes after splitting, want 5", n)
	}
	huge := []winenv.Change{{Scope: winenv.Machine, Name: "V", New: &winenv.Value{Data: strings.Repeat("x", 30000)}}}
	if _, err := batches(huge); err == nil {
		t.Error("a change too long for any command line was not refused")
	}
}

func TestHandleArgs(t *testing.T) {
	if handled, _ := HandleArgs([]string{"list"}); handled {
		t.Error("HandleArgs took an ordinary command")
	}
}
