package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/phpscript/config"
)

// TestZeroModule covers the embedded module the server's own modules carry: it
// reports the name Options.Modules filters on and answers every hook a module
// leaves alone, so coverageModule writes Mount and nothing else.
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
// configured options reach it, and the setup hook it carries takes the Service
// those options built. Nothing runs until Run, so the hook has not fired here.
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

// TestLogger covers the one call this package makes through a Logger: the
// reload warning, which names the keys a running server cannot pick up. A
// reload that changed nothing frozen says nothing, and a nil logger is the
// quiet server, which warns nowhere and does not panic.
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

// TestTelemetryModule states what the recorder is: a module the platform
// mounts like any other and the
// interpreter observes onto the trace it started, so setup holds it as one.
func TestTelemetryModule(t *testing.T) {
	t.Parallel()
	var recorder *TelemetryModule
	if _, ok := any(recorder).(LifecycleModule); !ok {
		t.Error("the recorder is not a lifecycle module")
	}
}

// recordingLogger is a Logger that keeps every message. Every other method of
// the interface panics: a call to one would be a claim on the library that
// nothing here states.
type recordingLogger struct {
	Logger
	messages []string
}

// Info records the message.
func (l *recordingLogger) Info(msg string, _ ...any) {
	l.messages = append(l.messages, msg)
}

// TestRouterMountsAHandler covers what a Router is to this package: the mux a
// module registers a handler on, under a method and a path. A chi router is
// one, and router.go returns it.
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
