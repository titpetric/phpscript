package ast

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
)

// Name is the command title.
const Name = "Print php script AST"

// NewCommand creates a new ast command.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "ast",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(args)
		},
	}
}

// Run tokenizes a PHP file and prints its PHP-style token stream.
func Run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: phpscript ast <file.php>")
	}
	src, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	printTokens(parser.TokenGetAll(string(src)))
	return nil
}

func printTokens(tokens *model.Array) {
	tokens.Range(func(_, val any) bool {
		if tok, ok := val.(*model.Array); ok {
			id, _ := tok.Get(int64(0))
			text, _ := tok.Get(int64(1))
			line, _ := tok.Get(int64(2))
			fmt.Printf("%4d  %-28s  %q\n", line, parser.TokenName(int(id.(int64))), text)
			return true
		}
		fmt.Printf("      %-28s  %q\n", "CHAR", val)
		return true
	})
}
