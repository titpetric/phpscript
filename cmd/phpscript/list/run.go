package list

import (
	"context"
	"fmt"
	"os"

	"github.com/titpetric/cli"

	phplist "github.com/titpetric/phpscript/list"
)

// Name is the command title.
const Name = "List routes, files and classes"

// NewCommand creates a new list command.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(args)
		},
	}
}

// Run prints a markdown table for the given path arguments.
func Run(args []string) error {
	rows, err := phplist.Paths(args)
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, phplist.Markdown(rows))
	return nil
}
