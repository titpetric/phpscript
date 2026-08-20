package main

import (
	"database/sql"
	"log"
	"os"

	_ "embed"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/cmd/phpscript/ast"
	"github.com/titpetric/phpscript/cmd/phpscript/fmt"
	"github.com/titpetric/phpscript/cmd/phpscript/info"
	"github.com/titpetric/phpscript/cmd/phpscript/lint"
	"github.com/titpetric/phpscript/cmd/phpscript/list"
	"github.com/titpetric/phpscript/cmd/phpscript/run"
	"github.com/titpetric/phpscript/cmd/phpscript/server"
	"github.com/titpetric/phpscript/cmd/phpscript/test"
	"github.com/titpetric/phpscript/cmd/phpscript/version"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/database"
)

func main() {
	if err := start(); err != nil {
		log.Fatalf("Unexpected error: %v", err)
	}
}

func start() error {
	configFile, args, err := parseConfigFile(os.Args[1:])
	if err != nil {
		return err
	}
	appConfig, err := loadConfig(configFile)
	if err != nil {
		return err
	}

	// The process environment comes first so `phpscript run` keeps the
	// connections it had, with config/config.yml env overriding them.
	env := append(append([]string{}, os.Environ()...), appConfig.Env...)
	database.Default = database.New(env)

	if os.Getenv("DEBUG") != "" {
		log.Println("sql_drivers", sql.Drivers())

		if val, ok := database.Default.(model.ExtendedDatabaseProvider); ok {
			connectionList := val.List()
			log.Println("connections", len(connectionList))
			for k, v := range connectionList {
				log.Println(k, v)
			}
		}
	}

	app := cli.NewApp("phpscript")
	app.AddCommand("ast", ast.Name, ast.NewCommand)
	app.AddCommand("fmt", fmt.Name, fmt.NewCommand)
	app.AddCommand("info", info.Name, info.NewCommand)
	app.AddCommand("lint", lint.Name, lint.NewCommand)
	app.AddCommand("list", list.Name, list.NewCommand)
	app.AddCommand("run", run.Name, func() *cli.Command {
		return run.NewCommand(appConfig)
	})
	app.AddCommand("server", server.Name, func() *cli.Command {
		return server.NewCommand(appConfig)
	})
	app.AddCommand("test", test.Name, test.NewCommand)
	app.AddCommand("version", version.Name, func() *cli.Command {
		return version.NewCommand(version.Info{
			Version:    Version,
			Commit:     Commit,
			CommitTime: CommitTime,
			Branch:     Branch,
		})
	})
	app.DefaultCommand = "run"
	return app.RunWithArgs(args)
}
