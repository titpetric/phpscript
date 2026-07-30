package fmtcmd

import (
	"context"
	"fmt"
	"os"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/formatter"
)

// Name is the command title.
const Name = "Format php scripts"

// NewCommand creates a new fmt command.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "fmt",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(args)
		},
	}
}

// Run formats files or directories in place by pretty-printing the AST.
func Run(args []string) error {
	n, err := formatter.Paths(args)
	if err != nil {
		return err
	}
	if n > 0 {
		fmt.Fprintf(os.Stderr, "formatted %d file(s)\n", n)
	}
	return nil
}
