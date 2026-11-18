package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/phpscript/config"
)

// TestZeroModule covers the embedded module the server's own modules share. It
// reports the name Options.Modules filters on. It answers every hook a module
// leaves alone; coverageModule writes Mount alone.
func TestZeroModule(t *testing.T) {
	t.Parallel()
	var module LifecycleModule = newZeroModule("phpprobe")
	if module.Name() != "phpprobe" {
		t.Errorf("Name() = %q, want phpprobe", module.Name())
	}
	if err := module.Start(context.Background()); err != nil {
		t.Errorf("Start() = %v, want nil from the zero module", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Errorf("Stop() = %v, want nil from the zero module", err)
	}
}

// TestManager covers what this package asks of a Manager before it serves: the
// configured options reach the manager. The setup hook it holds takes the
// Service those options built. The manager stays idle until Run. The hook has
// not fired here.
func TestManager(t *testing.T) {
	t.Parallel()
	cfg := config.NewTestConfig()
	cfg.Server.Addr = "127.0.0.1:0"

	options, err := cfg.PlatformOptions()
	if err != nil {
		t.Fatal(err)
	}
	var manager *Manager = newPlatformManager(options)
	if manager == nil {
		t.Fatal("newPlatformManager returned nil")
	}

	var configured *Service
	manager.Setup = func(svc *Service) error {
		configured = svc
		return nil
	}
	if manager.Setup == nil || configured != nil {
		t.Error("the setup hook ran before the manager served")
	}
}

// TestLogger covers the single call this package makes through a Logger: the
// reload warning, which names the keys a running server cannot pick up. A
// reload that changed no frozen key says nothing. A nil logger is the quiet
// server, which warns nowhere and does not panic.
func TestLogger(t *testing.T) {
	t.Parallel()
	started := config.NewTestConfig()
	reloaded := started
	reloaded.Server.Addr = "127.0.0.1:9999"

	recorder := &recordingLogger{}
	var log Logger = recorder
	warnFrozen(log, started, reloaded)
	if len(recorder.messages) != 1 {
		t.Fatalf("messages = %v, want the one warning", recorder.messages)
	}

	recorder.messages = nil
	warnFrozen(log, started, started)
	if len(recorder.messages) != 0 {
		t.Errorf("a reload that changed nothing frozen said %v", recorder.messages)
	}

	warnFrozen(nil, started, reloaded)
}

// TestTelemetryModule states the recorder's role: a module the platform mounts
// like the rest. The interpreter observes onto the trace it started, and
// setup holds that trace as a single trace.
func TestTelemetryModule(t *testing.T) {
	t.Parallel()
	var recorder *TelemetryModule
	if _, ok := any(recorder).(LifecycleModule); !ok {
		t.Error("the recorder is not a lifecycle module")
	}
}

// recordingLogger is a Logger that keeps every message. Every remaining
// interface method panics: a call to it would claim behaviour of the library
// that this package never makes.
type recordingLogger struct {
	Logger
	messages []string
}

// Info records the message.
func (l *recordingLogger) Info(msg string, _ ...any) {
	l.messages = append(l.messages, msg)
}

// TestRouterMountsAHandler covers the Router's role in this package: the mux a
// module registers a handler on, with a method plus a path. A chi router serves
// that role, and router.go returns it.
func TestRouterMountsAHandler(t *testing.T) {
	t.Parallel()
	var mounted Router = newRouter()
	mounted.Method(http.MethodGet, "/probe", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("probed"))
	}))

	recorder := httptest.NewRecorder()
	mounted.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if got := recorder.Body.String(); got != "probed" {
		t.Errorf("the mounted handler answered %q, want probed", got)
	}
}
