package apidoc

import (
	"testing"
)

func TestAreaName(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"registerStrings": "strings",
		"registerJSON":    "json",
		"Register":        "",
		"RegisterFS":      "",
		"init":            "",
	}
	for in, want := range cases {
		if got := areaName(in); got != want {
			t.Errorf("areaName(%q) = %q, want %q", in, got, want)
		}
	}
}
