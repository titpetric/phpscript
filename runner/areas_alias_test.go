package runner

import (
	"testing"
)

// TestAreaFunc covers the resolution of a name written through its area: the
// area has to define the function, the match ignores case, and a name outside
// Runtime\PS and Runtime\PX never reaches the source.
func TestAreaFunc(t *testing.T) {
	SetAreaMembers(func() map[string][]string {
		return map[string][]string{`Runtime\PS\Test\Strings`: {"probe_len"}}
	})
	t.Cleanup(func() { SetAreaMembers(nil) })

	cases := []struct {
		name string
		want string
		ok   bool
	}{
		{`Runtime\PS\Test\Strings\probe_len`, "probe_len", true},
		{`runtime\ps\TEST\strings\PROBE_LEN`, "PROBE_LEN", true},
		{`Runtime\PS\Test\Arrays\probe_len`, "", false},
		{`Runtime\Go\Test\Strings\probe_len`, "", false},
		{`Other\Strings\probe_len`, "", false},
		{`probe_len`, "", false},
	}
	for _, tc := range cases {
		got, ok := areaFunc(tc.name)
		if ok != tc.ok || got != tc.want {
			t.Errorf("areaFunc(%q) = %q, %v; want %q, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

// TestUnderArea covers the allocation-free prefix test every failed lookup runs.
func TestUnderArea(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		`Runtime\PS\Core\strlen`: true,
		`\runtime\px\Dates\f`:    true,
		`Runtime\PS`:             false,
		`Runtime\PS\`:            false,
		`Runtime\PSX\a`:          false,
		`Runtime\Go\make`:        false,
		`strlen`:                 false,
		``:                       false,
	}
	for name, want := range cases {
		if got := underArea(name); got != want {
			t.Errorf("underArea(%q) = %v, want %v", name, got, want)
		}
	}
}
