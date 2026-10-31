package lua

import (
	"strings"
	"testing"
)

func TestCompatNotEqual(t *testing.T) {
	if got := ApplyCompatReplacements("a != b"); got != "a ~= b" {
		t.Errorf("got %q, want %q", got, "a ~= b")
	}
}

func TestCompatBang(t *testing.T) {
	if got := ApplyCompatReplacements("if !(win) then"); got != "if not (win) then" {
		t.Errorf("got %q, want %q", got, "if not (win) then")
	}
}

func TestCompatNoBangLeftAfterNotEqual(t *testing.T) {
	got := ApplyCompatReplacements("a != b")
	if strings.Contains(got, "!") {
		t.Errorf("output %q still contains !", got)
	}
}

func TestCompatCompoundAssignments(t *testing.T) {
	cases := []struct{ in, want string }{
		{"chance += 2", "chance =chance + 2 "},
		{"chance -= 2", "chance =chance - 2 "},
		{"nextbet *= 2", "nextbet =nextbet * 2 "},
		{"nextbet /= 2", "nextbet =nextbet / 2 "},
	}
	for _, c := range cases {
		if got := ApplyCompatReplacements(c.in); got != c.want {
			t.Errorf("ApplyCompatReplacements(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCompatOrdering(t *testing.T) {
	// `!=` must run before `!` — otherwise != would corrupt into `~=`
	// via a double replacement.
	got := ApplyCompatReplacements("if a != b then c = !d end")
	want := "if a ~= b then c = not d end"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
