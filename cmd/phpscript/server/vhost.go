package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/ps/database"
	"github.com/titpetric/phpscript/stdlib/ps/mail"
	"github.com/titpetric/phpscript/telemetry"
)

// hostMux routes a request to the site matching its Host, answering 404 otherwise.
type hostMux struct {
	hosts map[string]http.Handler
}

// newHostMux returns a mux over sites, normalizing each configured domain key once.
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

// normalizeHost lower-cases host, dropping its port, trailing dot and IPv6 brackets.
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

// registerVirtualHosts wires one site per configured domain behind a host mux.
func registerVirtualHosts(ctx context.Context, svc *Service, appConfig config.Config, globals *flags.Options, cover *coverageModule) error {
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

// buildVirtualHosts returns the host mux and site modules, refusing a broken configuration.
func buildVirtualHosts(ctx context.Context, appConfig config.Config, globals *flags.Options, cover *coverageModule) (http.Handler, []LifecycleModule, error) {
	if err := appConfig.CheckVirtualHosts(); err != nil {
		return nil, nil, err
	}

	var modules []LifecycleModule
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
		// name is another way to spell the site, not another copy of it;
		// it shares the site's routes, recorder and connections.
		for _, domain := range host.Domains() {
			sites[domain] = handler
		}
	}

	return newHostMux(sites), modules, nil
}

// newVirtualHost builds one site's router and lifecycle modules, isolated from the other sites.
func newVirtualHost(ctx context.Context, host config.VirtualHost, siteConfig config.Config, globals *flags.Options, cover *coverageModule) (http.Handler, []LifecycleModule, error) {
	router := newRouter()

	// A site answering to more than one name is still one site, reported and
	// named after the first of them.
	name := host.Name()

	// The site owns its telemetry block and it owns the tracer and the debug
	// front end that block describes. chi takes every middleware before the
	// first route and this comes first.
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

	// The site's connections come from the site env and nowhere else. A
	// provider holds the credentials; a site cannot name a connection another
	// site configures. Relative sqlite paths anchor to the site's root: a site
	// writes its DSN against the site tree, and the directory the server
	// process runs in is not the site's to know.
	runnerOptions := siteConfig.Runner
	runnerOptions.Database = database.NewAt(host.Root, siteConfig.Env)

	// So do the mail servers. A provider holds the credentials; a site can name
	// the servers its configuration declares, no others; no script there sees
	// the credentials behind them.
	runnerOptions.Mail = mail.NewProvider(siteConfig.Mail)

	// The environment its scripts read is separate as well. A site holds its declared env,
	// no part of the process environment; getenv() reports nothing about the
	// values an operator or another site declares.
	runnerOptions.Env = siteConfig.Env

	// A site names its prelude in the runner block of its phpscript.yml: each
	// site has a separate vendor directory. --include is the operator's setting
	// for a tenant that named none; use it to set the value in a shared
	// environment: a site's own file is not the operator's to edit.
	if runnerOptions.Include == "" {
		runnerOptions.Include = globals.Include
	}

	documentRoot := documentRoot(siteConfig)
	root := os.DirFS(host.Root)

	// newHandler runs first because the handler owns the site's caches, and
	// the annotated files run off the same pair.
	files, err := newHandler(root, host.Root, documentRoot, runnerOptions, siteConfig.Flatstack.Enabled, siteConfig.Autoindex, observers...)
	if err != nil {
		return nil, nil, fmt.Errorf("virtualhost %q: %w", name, err)
	}

	annotationOptions := sharedCaches(files, annotationOptions(siteConfig, runnerOptions, observers, host.Root, name))
	if cover != nil {
		cover.watch(root)
		annotationOptions = append(annotationOptions, annotations.WithCoverage(cover.aggregator))
		files.coverage = cover.aggregator
	}

	// A site's error pages belong to the site: its document root holds them;
	// its handler renders them. No other domain reaches what a site puts up.
	if siteConfig.Routes.Enabled {
		options := routeOptions(annotationOptions, runnerOptions.WritablePaths, documentRoot)
		options = append(options, annotations.WithErrorPages(files.serveErrorPage))
		routes := annotations.NewRoute(root, options...)
		if err := routes.Mount(ctx, router); err != nil {
			return nil, nil, fmt.Errorf("virtualhost %q: %w", name, err)
		}
	}

	// This site's @startup jobs bind to this site's router. The mount is here
	// because this function alone holds the router. The platform passes the
	// server's router to a module it mounts, and routes a site adds there would
	// answer on every domain.
	if err := annotations.NewBindings(annotationOptions...).Mount(ctx, router); err != nil {
		return nil, nil, fmt.Errorf("virtualhost %q: %w", name, err)
	}

	router.Handle("/*", files)

	// Each site precompiles its tree inside the runner block; one site paying
	// the startup cost says nothing about what another does.
	precompile(files, name)

	// A site's startup failure is the site's alone. That site's recorder
	// records the failure; the server keeps serving: the alternative is a
	// single tenant's broken job stopping the other tenants.
	return router, []LifecycleModule{
		nonFatal(annotations.NewStartup(root, annotationOptions...), observers),
		annotations.NewScheduler(root, annotationOptions...),
	}, nil
}
