package route

import (
	"context"
	"net/http"
	"os"

	"github.com/titpetric/cli"
	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/config"
	routesvc "github.com/titpetric/phpscript/route"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/status"
)

// Name is the command title.
const Name = "Run php route server"

// NewCommand creates a new route command.
func NewCommand(config config.Config) *cli.Command {
	return &cli.Command{
		Name:  "route",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args, config)
		},
	}
}

// Module registers annotated PHP routes with the platform.
type Module struct {
	platform.UnimplementedModule
	root          string
	runnerOptions runner.Options
	flatstack     bool
	observers     []runner.Observer
}

// NewModule creates an annotated route module.
func NewModule(root string, options runner.Options, flatstack bool, observers ...runner.Observer) *Module {
	return &Module{
		UnimplementedModule: *platform.NewUnimplementedModule("phproute"),
		root:                root,
		runnerOptions:       options,
		flatstack:           flatstack,
		observers:           observers,
	}
}

// Mount registers annotated PHP routes.
func (m *Module) Mount(_ context.Context, router platform.Router) error {
	mux := http.NewServeMux()
	options := []routesvc.Option{
		routesvc.WithRunnerOptions(m.runnerOptions),
		routesvc.WithFlatstack(m.flatstack),
	}
	for _, observer := range m.observers {
		options = append(options, routesvc.WithRuntimeFunc(func(rt *runner.Runtime) {
			rt.Observe(observer)
		}))
	}
	if _, err := routesvc.NewService(os.DirFS(m.root), mux, options...); err != nil {
		return err
	}
	router.Handle("/*", mux)
	return nil
}

// Run loads routes from the current directory or the first argument.
func Run(ctx context.Context, args []string, config config.Config) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	options := platform.NewOptions()
	service := platform.New(options)
	var observers []runner.Observer
	if config.Status.Enabled {
		serverStatus := status.NewModule(config.Status.Options)
		service.Use(serverStatus.Middleware)
		service.Register(serverStatus)
		observers = append(observers, serverStatus)
	}
	service.Register(NewModule(root, config.Runner, config.Flatstack.Enabled, observers...))

	if err := service.Start(ctx); err != nil {
		return err
	}
	service.Wait()
	return nil
}
