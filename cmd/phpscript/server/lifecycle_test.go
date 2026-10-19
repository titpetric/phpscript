package server

import (
	"context"
	"errors"
	"testing"

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// failingModule stands in for a startup module whose jobs did not run.
type failingModule struct {
	platform.UnimplementedModule
	err     error
	started bool
}

func (m *failingModule) Start(context.Context) error {
	m.started = true
	return m.err
}

// TestNonFatalRecordsTheFailureInsteadOfReturningIt pins both halves of the
// contract: the platform sees the module start, and the operator can still
// find out that it did not.
func TestNonFatalRecordsTheFailureInsteadOfReturningIt(t *testing.T) {
	t.Parallel()
	traceOptions := telemetry.NewOptions("phpscript")
	// oida records only when the options enable it; a test that reads its
	// traces back needs that.
	traceOptions.Enabled = true
	tracer, err := telemetry.New(traceOptions)
	if err != nil {
		t.Fatal(err)
	}
	recorder := telemetry.NewModule(tracer)

	module := &failingModule{
		UnimplementedModule: *platform.NewUnimplementedModule("phpstartup:shop.example.com"),
		err:                 errors.New("startup boot.php: missing_function"),
	}

	if err := nonFatal(module, []runner.Observer{recorder}).Start(context.Background()); err != nil {
		t.Fatalf("error = %v, want nil so the platform keeps the other sites", err)
	}
	if !module.started {
		t.Fatal("the wrapped module did not run")
	}

	snapshot := recorder.Snapshot()
	if len(snapshot.Log) != 1 {
		t.Fatalf("traces = %d, want the failure recorded as lifecycle work", len(snapshot.Log))
	}
	trace := snapshot.Log[0]
	if trace.Name != "phpstartup:shop.example.com" {
		t.Fatalf("trace name = %q, want the module name", trace.Name)
	}
	if trace.ErrorText == "" || trace.State != telemetry.StateError {
		t.Fatalf("trace = %+v, want it marked failed", trace)
	}
}

// A module that starts cleanly records as a successful trace and still returns
// nil, so the wrapper is not something to reach for only when things break.
func TestNonFatalRecordsASuccessfulStart(t *testing.T) {
	t.Parallel()
	traceOptions := telemetry.NewOptions("phpscript")
	// oida records only when the options enable it; a test that reads its
	// traces back needs that.
	traceOptions.Enabled = true
	tracer, err := telemetry.New(traceOptions)
	if err != nil {
		t.Fatal(err)
	}
	recorder := telemetry.NewModule(tracer)
	module := &failingModule{UnimplementedModule: *platform.NewUnimplementedModule("phpstartup:blog.example.com")}

	if err := nonFatal(module, []runner.Observer{recorder}).Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := recorder.Snapshot()
	if len(snapshot.Log) != 1 || snapshot.Log[0].ErrorText != "" {
		t.Fatalf("traces = %+v, want one clean trace", snapshot.Log)
	}
}

// With telemetry off there is no observer to record onto. The failure still
// must not reach the platform, or one site's broken job stops every site.
func TestNonFatalSurvivesWithoutARecorder(t *testing.T) {
	t.Parallel()
	module := &failingModule{
		UnimplementedModule: *platform.NewUnimplementedModule("phpstartup:shop.example.com"),
		err:                 errors.New("startup boot.php: missing_function"),
	}
	if err := nonFatal(module, nil).Start(context.Background()); err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if !module.started {
		t.Fatal("the wrapped module did not run")
	}
}

// The wrapper delegates everything else, so the platform still sees the module
// it registered.
func TestNonFatalKeepsTheModuleName(t *testing.T) {
	t.Parallel()
	module := &failingModule{UnimplementedModule: *platform.NewUnimplementedModule("phpstartup:shop.example.com")}
	if got := nonFatal(module, nil).Name(); got != "phpstartup:shop.example.com" {
		t.Fatalf("name = %q", got)
	}
}

// TestReload covers what SIGHUP does, driving Reload directly so nothing
// depends on signal delivery timing.
func TestReload(t *testing.T) {
	t.Parallel()
	a := site(t, "a", "a")
	b := site(t, "b", "b")

	header := "telemetry:\n  enabled: false\nserver:\n  addr: \"127.0.0.1:0\"\n"
	oneSite := header + "virtualhost:\n  - domain: a.localhost\n    root: " + a + "\n"
	twoSites := oneSite + "  - domain: b.localhost\n    root: " + b + "\n"

	start := func(t *testing.T, filename string) *managerFixture {
		t.Helper()

		globals := &flags.Options{ConfigFile: filename}
		appConfig, err := config.Load(filename)
		if err != nil {
			t.Fatal(err)
		}

		manager, err := newManager(t.Context(), nil, appConfig, globals)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(manager.Stop)
		if err := manager.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		return &managerFixture{manager: manager, filename: filename}
	}

	t.Run("a reload reads the file again", func(t *testing.T) {
		filename := operatorConfig(t, oneSite)
		fixture := start(t, filename)

		if status, got := body(t, fixture.manager.URL(), "a.localhost"); status != 200 || got != "a" {
			t.Fatalf("a.localhost = %d %q", status, got)
		}
		if status, _ := body(t, fixture.manager.URL(), "b.localhost"); status != 404 {
			t.Fatalf("b.localhost = %d, want 404 before the reload", status)
		}

		fixture.rewrite(t, twoSites)
		if err := fixture.manager.Reload(t.Context()); err != nil {
			t.Fatal(err)
		}

		// The site the edit added, and the one already there.
		if status, got := body(t, fixture.manager.URL(), "b.localhost"); status != 200 || got != "b" {
			t.Fatalf("b.localhost = %d %q, want the reloaded site", status, got)
		}
		if status, got := body(t, fixture.manager.URL(), "a.localhost"); status != 200 || got != "a" {
			t.Fatalf("a.localhost = %d %q", status, got)
		}
	})

	t.Run("the address survives a reload", func(t *testing.T) {
		filename := operatorConfig(t, oneSite)
		fixture := start(t, filename)

		before := fixture.manager.URL()
		fixture.rewrite(t, twoSites)
		if err := fixture.manager.Reload(t.Context()); err != nil {
			t.Fatal(err)
		}
		if after := fixture.manager.URL(); after != before {
			t.Fatalf("url = %q, want %q", after, before)
		}
	})

	// The reason Check exists. A reload the manager cannot apply leaves the
	// generation that is serving alone, and never stops it first to then
	// discover the new configuration is unusable.
	t.Run("a broken configuration is refused and keeps serving", func(t *testing.T) {
		filename := operatorConfig(t, oneSite)
		fixture := start(t, filename)

		serving := fixture.manager.Platform()

		fixture.rewrite(t, twoSites+"  - domain: c.localhost\n    root: /nonexistent-root\n")
		err := fixture.manager.Reload(t.Context())
		if err == nil {
			t.Fatal("a reload with a missing root was applied")
		}

		if fixture.manager.Platform() != serving {
			t.Fatal("the generation that was serving was replaced by a refused reload")
		}
		if status, got := body(t, fixture.manager.URL(), "a.localhost"); status != 200 || got != "a" {
			t.Fatalf("a.localhost = %d %q, want the old generation still serving", status, got)
		}
	})
}
