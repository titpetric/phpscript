package helpdocs_test

import (
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/helpdocs"
)

// TestNewFlagSet covers the set the help is rendered from: it is named for
// what it holds, it starts empty, and a flag added to it is readable through
// the four fields the flag table prints.
func TestNewFlagSet(t *testing.T) {
	var fs *helpdocs.FlagSet = helpdocs.NewFlagSet("test")

	if got := fs.Name(); got != "test" {
		t.Errorf("Name() = %q, want test", got)
	}
	if fs.HasFlags() {
		t.Error("a new set already has flags")
	}

	fs.BoolP("matrix", "m", false, "Run every runtime")

	var flag *helpdocs.Flag = fs.Lookup("matrix")
	if flag == nil {
		t.Fatal("Lookup(matrix) = nil, want the flag that was added")
	}
	if got, want := flag.Shorthand, "m"; got != want {
		t.Errorf("Shorthand = %q, want %q", got, want)
	}
	if got, want := flag.DefValue, "false"; got != want {
		t.Errorf("DefValue = %q, want %q", got, want)
	}
	if got, want := flag.Usage, "Run every runtime"; got != want {
		t.Errorf("Usage = %q, want %q", got, want)
	}
}

// TestNewFlagSetContinuesOnError covers the error handling a set built for the
// help carries: an unknown flag is an error returned, and not an exit.
func TestNewFlagSetContinuesOnError(t *testing.T) {
	fs := helpdocs.NewFlagSet("test")

	if err := fs.Parse([]string{"--nothing"}); err == nil {
		t.Error("Parse(--nothing) = nil, want an error on a flag the set does not define")
	}
}
