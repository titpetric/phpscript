package helpdocs

import (
	"github.com/titpetric/cli"
)

// FlagSet is the flags of one section of the help: the shared set, or the set
// one command adds to it.
type FlagSet = cli.FlagSet

// Flag is one flag of a set, which the help reads four fields off: the long
// name, the shorthand, the default and the usage line.
type Flag = cli.Flag

// NewFlagSet returns an empty set named for what it will hold.
//
// The error handling is ContinueOnError, which is the only thing a set built
// for the help can do: nothing parses it, so nothing it was handed is wrong,
// and a set that exited on a parse error would take the process with it while
// rendering a document.
func NewFlagSet(name string) *FlagSet {
	return cli.NewFlagSet(name, cli.ContinueOnError)
}
