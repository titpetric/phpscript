package main

import (
	"log"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/cmd/phpscript/lint"
	"github.com/titpetric/phpscript/cmd/phpscript/run"
	"github.com/titpetric/phpscript/cmd/phpscript/server"
	"github.com/titpetric/phpscript/cmd/phpscript/version"
)

func main() {
	if err := start(); err != nil {
		log.Fatalf("Unexpected error: %+v", err)
	}
}

func start() error {
	app := cli.NewApp("phpscript")
	app.AddCommand("lint", lint.Name, lint.NewCommand)
	app.AddCommand("run", run.Name, run.NewCommand)
	app.AddCommand("server", server.Name, server.NewCommand)
	app.AddCommand("version", version.Name, func() *cli.Command {
		return version.NewCommand(version.Info{
			Version:    Version,
			Commit:     Commit,
			CommitTime: CommitTime,
			Branch:     Branch,
		})
	})
	app.DefaultCommand = "run"
	return app.Run()
}
