package stdlib_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/stdlib"
)

// TestParseProfile covers the --stdlib values. An unknown name is an error
// and no fallback, because falling back to every area would narrow no area
// while reading as a narrowed runtime.
func TestParseProfile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value string
		want  stdlib.Profile
		fail  bool
	}{
		{value: "", want: stdlib.All},
		{value: "all", want: stdlib.All},
		{value: "secure", want: stdlib.Secure},
		{value: "exec", want: stdlib.Exec},
		{value: "secure,exec", want: stdlib.Secure | stdlib.Exec},
		{value: " SECURE , Exec ", want: stdlib.Secure | stdlib.Exec},
		{value: "sekure", fail: true},
		{value: "secure,", fail: true},
	} {
		got, err := stdlib.ParseProfile(tc.value)
		if tc.fail {
			if err == nil {
				t.Errorf("ParseProfile(%q) = %b, want an error", tc.value, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseProfile(%q): %v", tc.value, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseProfile(%q) = %b, want %b", tc.value, got, tc.want)
		}
	}
}

// TestSetProfileNarrowsRegister is what makes --stdlib one flag for the whole
// process: Register reads the process profile and narrowing it reaches every
// runtime built afterwards without a call site passing anything.
func TestSetProfileNarrowsRegister(t *testing.T) {
	t.Cleanup(func() { stdlib.SetProfile(stdlib.All) })

	stdlib.SetProfile(stdlib.Secure)
	if got := runScript(t, listPexec()); got != "none" {
		t.Fatalf("under a secure process profile Register defined %q", got)
	}

	stdlib.SetProfile(stdlib.All)
	if got, want := runScript(t, listPexec()), strings.Join(pexecNames, " ")+" none"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestDefaultProfileIsSecure pins the constant the configuration story rests
// on: the default profile for an untrusted host names no insecure area.
func TestDefaultProfileIsSecure(t *testing.T) {
	t.Parallel()
	if stdlib.Default != stdlib.Secure {
		t.Fatalf("stdlib.Default = %b, want stdlib.Secure (%b)", stdlib.Default, stdlib.Secure)
	}
	if stdlib.Default&stdlib.Exec != 0 {
		t.Fatal("stdlib.Default names Exec")
	}
}
