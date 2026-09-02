package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	chi "github.com/go-chi/chi/v5"
	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/database"
	"github.com/titpetric/phpscript/telemetry"
)

// hostMux dispatches a request to the site that answers its Host. It is what
// makes one process serve several websites: each site is a router of its own,
// built from its own application root and its own configuration, and the Host
// header is the only thing that selects between them.
//
// Matching is exact. A Host nobody claims gets 404 rather than falling through
// to some default site, because in a shared execution environment the wrong
// answer is a request landing in another tenant's code.
type hostMux struct {
	hosts map[string]http.Handler
}

// newHostMux returns a mux serving the given sites. The keys are domains as
// configured; they are normalized here so a lookup never has to be.
func newHostMux(sites map[string]http.Handler) *hostMux {
	hosts := make(map[string]http.Handler, len(sites))
	for domain, handler := range sites {
		hosts[normalizeHost(domain)] = handler
	}
	return &hostMux{hosts: hosts}
}

func (m *hostMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, ok := m.hosts[normalizeHost(r.Host)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	handler.ServeHTTP(w, r)
}

// normalizeHost reduces a Host header or a configured domain to the form the
// two compare equal in: lower case, no port, no trailing dot. A client is free
// to send any of "Example.COM", "example.com:8080" or the fully qualified
// "example.com.", and all three name the same site.
func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname
	}
	// A bracketed IPv6 literal without a port survives SplitHostPort as is.
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

// registerVirtualHosts wires one site per configured domain and puts a host mux
// in front of them.
func registerVirtualHosts(ctx context.Context, svc *platform.Platform, appConfig config.Config, globals *flags.Options, cover *coverageModule) error {
	handler, modules, err := buildVirtualHosts(ctx, appConfig, globals, cover)
	if err != nil {
		return err
	}
	for _, module := range modules {
		svc.Register(module)
	}
	svc.Register(newModule("phpvhost", handler))
	return nil
}

// buildVirtualHosts returns the host mux serving every configured site and the
// lifecycle modules the platform starts on their behalf. Every site is built
// here, before the server starts, so a broken configuration fails startup
// rather than one request.
func buildVirtualHosts(ctx context.Context, appConfig config.Config, globals *flags.Options, cover *coverageModule) (http.Handler, []platform.Module, error) {
	if err := appConfig.ValidateVirtualHosts(); err != nil {
		return nil, nil, err
	}

	var modules []platform.Module
	sites := make(map[string]http.Handler, len(appConfig.VirtualHost))
	for _, host := range appConfig.VirtualHost {
		host = host.Normalize()

		siteConfig, err := host.Load(appConfig)
		if err != nil {
			return nil, nil, err
		}

		handler, siteModules, err := newVirtualHost(ctx, host, siteConfig, globals, cover)
		if err != nil {
			return nil, nil, err
		}
		modules = append(modules, siteModules...)

		// Every name the entry lists reaches the same handler. A further
		// name is another way to spell the site, not another copy of it,
		// so it shares the site's routes, recorder and connections.
		for _, domain := range host.Domains() {
			sites[domain] = handler
		}
	}

	return newHostMux(sites), modules, nil
}

// newVirtualHost builds the router one site is served by and the lifecycle
// modules the platform starts on its behalf.
//
// The site gets a router of its own because it owns its routes, its telemetry
// and its databases. Nothing it registers is visible on another domain, and the
// connections its scripts can name are only the ones its own env configured.
func newVirtualHost(ctx context.Context, host config.VirtualHost, siteConfig config.Config, globals *flags.Options, cover *coverageModule) (http.Handler, []platform.Module, error) {
	router := chi.NewRouter()

	// A site answering to several names is still one site, reported and named
	// after the first of them.
	name := host.Name()

	// The site owns its telemetry block, so it owns the tracer and the debug
	// front end that block describes. chi wants every middleware before the
	// first route, so this comes first.
	var observers []runner.Observer
	telemetryOptions, err := siteConfig.Telemetry.Resolved()
	if err != nil {
		return nil, nil, fmt.Errorf("virtualhost %q: %w", name, err)
	}
	if telemetryOptions.Enabled {
		tracer, err := telemetry.New(telemetryOptions)
		if err != nil {
			return nil, nil, fmt.Errorf("virtualhost %q: telemetry: %w", name, err)
		}
		router.Use(tracer.Middleware)
		if err := telemetry.Mount(router, tracer); err != nil {
			return nil, nil, fmt.Errorf("virtualhost %q: telemetry: %w", name, err)
		}
		observers = append(observers, telemetry.NewModule(tracer))
	}

	// The site's connections come from its own env and nowhere else. A
	// provider holds the credentials it was given, so a site cannot name a
	// connection another site configured.
	runnerOptions := siteConfig.Runner
	runnerOptions.Database = database.New(siteConfig.Env)

	// So does the environment its scripts read. A site is handed the env it
	// declared rather than the process environment, so getenv() cannot be
	// used to read what the operator, or another site, was started with.
	runnerOptions.Env = siteConfig.Env

	// A site names its own prelude in the runner block of its phpscript.yml,
	// because each one has its own vendor directory. --include is what the
	// operator sets for a tenant that named none, which is the only way to set
	// it in a shared environment: a site's own file is not the operator's to
	// edit.
	if runnerOptions.Include == "" {
		runnerOptions.Include = globals.Include
	}

	documentRoot := documentRoot(siteConfig)
	annotationOptions := annotationOptions(siteConfig, runnerOptions, observers, host.Root, name)

	root := os.DirFS(host.Root)
	if cover != nil {
		cover.watch(root)
		annotationOptions = append(annotationOptions, annotations.WithCoverage(cover.aggregator))
	}
	files, err := newHandler(root, host.Root, documentRoot, runnerOptions, siteConfig.Flatstack.Enabled, siteConfig.Autoindex, observers...)
	if err != nil {
		return nil, nil, fmt.Errorf("virtualhost %q: %w", name, err)
	}
	files.smtp = siteConfig.SMTP
	if cover != nil {
		files.coverage = cover.aggregator
	}

	// A site's error pages are its own: they are found under its document root
	// and rendered by its own handler, so nothing a site puts up is reachable
	// from another domain.
	if siteConfig.Routes.Enabled {
		options := routeOptions(annotationOptions, runnerOptions.WritablePaths, documentRoot)
		options = append(options, annotations.WithErrorPages(files.serveErrorPage))
		routes := annotations.NewRoute(root, options...)
		if err := routes.Mount(ctx, router); err != nil {
			return nil, nil, fmt.Errorf("virtualhost %q: %w", name, err)
		}
	}

	router.Handle("/*", files)

	// A site's startup failure is its own. It is recorded on that site's
	// recorder and the server carries on, because the alternative is one
	// tenant's broken job stopping every other tenant.
	return router, []platform.Module{
		nonFatal(annotations.NewStartup(root, annotationOptions...), observers),
		annotations.NewScheduler(root, annotationOptions...),
	}, nil
}
