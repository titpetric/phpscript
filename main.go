package main

import (
	_ "embed"
	"log"
	"os"

	"github.com/goccy/go-yaml"

	"github.com/titpetric/cli"
	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/cmd/phpscript/ast"
	"github.com/titpetric/phpscript/cmd/phpscript/fmt"
	"github.com/titpetric/phpscript/cmd/phpscript/lint"
	"github.com/titpetric/phpscript/cmd/phpscript/list"
	"github.com/titpetric/phpscript/cmd/phpscript/run"
	"github.com/titpetric/phpscript/cmd/phpscript/server"
	"github.com/titpetric/phpscript/cmd/phpscript/test"
	"github.com/titpetric/phpscript/cmd/phpscript/version"
	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/model"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	if err := start(); err != nil {
		log.Fatalf("Unexpected error: %v", err)
	}
}

func start() error {
	appConfig, err := loadConfig()
	if err != nil {
		return err
	}

	platform.SetupConnections(appConfig.Env)

	if os.Getenv("DEBUG") != "" {
		if val, ok := platform.Database.(model.ExtendedDatabaseProvider); ok {
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
	return app.Run()
}

func loadConfig() (config.Config, error) {
	result := config.New()
	if err := yaml.Unmarshal(config.DefaultRuntimeConfig, &result); err != nil {
		log.Printf("Could not parse default runtime config: %v", err)
		return result, err
	}
	return result, nil
}
