package server

import (
	"context"
	"errors"
	"testing"

	"github.com/titpetric/platform"

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
// contract: the platform is told the module started, and the operator can still
// find out that it did not.
func TestNonFatalRecordsTheFailureInsteadOfReturningIt(t *testing.T) {
	tracer, err := telemetry.New(telemetry.NewOptions())
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
	if trace.Error == "" || trace.State != telemetry.StateError {
		t.Fatalf("trace = %+v, want it marked failed", trace)
	}
}

// A module that starts cleanly records as a successful trace and still returns
// nil, so the wrapper is not something to reach for only when things break.
func TestNonFatalRecordsASuccessfulStart(t *testing.T) {
	tracer, err := telemetry.New(telemetry.NewOptions())
	if err != nil {
		t.Fatal(err)
	}
	recorder := telemetry.NewModule(tracer)
	module := &failingModule{UnimplementedModule: *platform.NewUnimplementedModule("phpstartup:blog.example.com")}

	if err := nonFatal(module, []runner.Observer{recorder}).Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := recorder.Snapshot()
	if len(snapshot.Log) != 1 || snapshot.Log[0].Error != "" {
		t.Fatalf("traces = %+v, want one clean trace", snapshot.Log)
	}
}

// With telemetry off there is no observer to record onto. The failure still
// must not reach the platform, or one site's broken job stops every site.
func TestNonFatalSurvivesWithoutARecorder(t *testing.T) {
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
	module := &failingModule{UnimplementedModule: *platform.NewUnimplementedModule("phpstartup:shop.example.com")}
	if got := nonFatal(module, nil).Name(); got != "phpstartup:shop.example.com" {
		t.Fatalf("name = %q", got)
	}
}
