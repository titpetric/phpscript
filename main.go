package main

import (
	"database/sql"
	"log"
	"os"

	_ "embed"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/spf13/pflag"
	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/cmd/phpscript/ast"
	"github.com/titpetric/phpscript/cmd/phpscript/fmt"
	"github.com/titpetric/phpscript/cmd/phpscript/helpdocs"
	"github.com/titpetric/phpscript/cmd/phpscript/info"
	"github.com/titpetric/phpscript/cmd/phpscript/lint"
	"github.com/titpetric/phpscript/cmd/phpscript/list"
	"github.com/titpetric/phpscript/cmd/phpscript/run"
	"github.com/titpetric/phpscript/cmd/phpscript/server"
	"github.com/titpetric/phpscript/cmd/phpscript/test"
	"github.com/titpetric/phpscript/cmd/phpscript/version"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/internal/table"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/database"
)

func main() {
	if err := start(); err != nil {
		log.Fatalf("Unexpected error: %v", err)
	}
}

func start() error {
	// -f and -w decide what every command is handed, so both are read before
	// one is built: -w moves the process, and the configuration file is read
	// from where it lands.
	globals, args, err := flags.Pre(os.Args[1:])
	if err != nil {
		return err
	}
	if err := globals.Chdir(); err != nil {
		return err
	}
	appConfig, err := loadConfig(globals.ConfigFile)
	if err != nil {
		return err
	}
	globals.FromConfig(appConfig)

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

	commands := []registration{
		{"ast", ast.Name, ast.NewCommand},
		{"fmt", fmt.Name, fmt.NewCommand},
		{"info", info.Name, func() *cli.Command { return info.NewCommand(globals) }},
		{"lint", lint.Name, func() *cli.Command { return lint.NewCommand(globals) }},
		{"list", list.Name, list.NewCommand},
		{"run", run.Name, func() *cli.Command { return run.NewCommand(appConfig, globals) }},
		{"server", server.Name, func() *cli.Command { return server.NewCommand(appConfig, globals) }},
		{"test", test.Name, func() *cli.Command { return test.NewCommand(globals) }},
		{"version", version.Name, func() *cli.Command {
			return version.NewCommand(version.Info{
				Version:    Version,
				Commit:     Commit,
				CommitTime: CommitTime,
				Branch:     Branch,
			})
		}},
	}

	app := cli.NewApp("phpscript")
	for _, command := range commands {
		app.AddCommand(command.name, command.title, decorate(globals, command))
	}
	app.DefaultCommand = "run"

	// The library's own help is the command list and nothing else. A request
	// for the whole document is answered here, before a command is selected;
	// `phpscript <command> --help` still goes through the library, which prints
	// the command's flags and the Usage text decorate attached.
	if wantsHelp(args) {
		return writeHelp(os.Stdout, commands)
	}
	return app.RunWithArgs(flags.Hoist(args, app.HasCommand))
}

// registration is one command as main knows it: what it is called, what it
// does, and how to build it. The list is kept because the help document is
// built from it; cli.App holds the same entries and does not publish them.
type registration struct {
	name  string
	title string
	new   func() *cli.Command
}

// decorate binds the shared flags onto a command and wraps its Run with the
// work they imply, so no command package repeats either. It also attaches the
// command's examples, which is what `phpscript <command> --help` prints under
// its usage line.
func decorate(globals *flags.Options, command registration) func() *cli.Command {
	return func() *cli.Command {
		c := command.new()
		c.Bind = globals.BindWith(c.Bind)
		c.Run = globals.RunWith(c.Run)
		if c.Usage == nil {
			c.Usage = func() string {
				return helpdocs.Examples(command.name, !table.IsTerminal(os.Stdout))
			}
		}
		return c
	}
}

// wantsHelp reports whether the arguments ask for the whole document rather
// than for a command. A help flag after a command name is that command's, and
// the library answers it.
func wantsHelp(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			return true
		case "help":
			return true
		default:
			if len(arg) > 0 && arg[0] != '-' {
				return false
			}
		}
	}
	return false
}

// writeHelp renders the long help: the shared flags once, then every command
// with the flags it adds and its examples.
func writeHelp(f *os.File, commands []registration) error {
	markdown := !table.IsTerminal(f)

	shared := pflag.NewFlagSet("phpscript", pflag.ContinueOnError)
	(&flags.Options{}).Bind(shared)

	entries := make([]helpdocs.Command, 0, len(commands))
	for _, command := range commands {
		// The command is built and bound against a throwaway set holding both,
		// then the shared names are dropped: what is left is what the command
		// adds, which is the only part worth repeating per section.
		both := pflag.NewFlagSet(command.name, pflag.ContinueOnError)
		if bind := command.new().Bind; bind != nil {
			bind(both)
		}
		own := pflag.NewFlagSet(command.name, pflag.ContinueOnError)
		both.VisitAll(func(flag *pflag.Flag) {
			if shared.Lookup(flag.Name) == nil {
				own.AddFlag(flag)
			}
		})
		entries = append(entries, helpdocs.Command{Name: command.name, Title: command.title, Flags: own})
	}
	return helpdocs.Write(f, "phpscript", shared, entries, markdown)
}
