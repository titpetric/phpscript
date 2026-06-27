package route

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/titpetric/cli"

	routesvc "github.com/titpetric/phpscript/route"
)

// Name is the command title.
const Name = "Run php route server"

// NewCommand creates a new route command.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "route",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args)
		},
	}
}

// Run loads routes from the current directory or the first argument.
func Run(ctx context.Context, args []string) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	mux := http.NewServeMux()
	if _, err := routesvc.NewService(os.DirFS(root), mux); err != nil {
		return err
	}

	server := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		<-ctx.Done()
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("Route server shutdown error: %v", err)
		}
	}()

	log.Printf("Route server listening on %s with root %s", server.Addr, root)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
