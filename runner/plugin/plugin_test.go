package plugin_test

// These tests build a real .so with the toolchain running the test, which is
// the only way to be sure the plugin and the host agree: a plugin built by a
// different Go version, or against a different build of a shared package, is
// refused by the Go runtime rather than loaded and misbehaving.
//
// A build takes seconds, so the fixtures here are deliberately few and each one
// is built once and reused.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/plugin"
)

// hostPlugin declares its own Host interface and imports nothing of
// phpscript's, which is the property the package exists to make possible.
const hostPlugin = `package main

import (
	"context"
	"io"
)

type Host interface {
	RegisterConstructor(name string, ctor any)
	Output() io.Writer
}

type Greeter struct{}

func (Greeter) Greet() string { return "hello from the plugin" }

var initialised int

func Init(ctx context.Context) error { initialised++; return nil }

func Bind(ctx context.Context, h Host) error {
	h.RegisterConstructor("Greeter", func() Greeter { return Greeter{} })
	h.RegisterConstructor("InitCount", func() int64 { return int64(initialised) })
	return nil
}
`

// ctxOnlyPlugin uses the shape that takes no host, for a plugin that only has
// process setup to do.
const ctxOnlyPlugin = `package main

import "context"

func Init(ctx context.Context) error { return nil }
func Bind(ctx context.Context) error { return nil }
`

// badHostPlugin asks for a method the runtime does not have, which has to be
// refused at load with a message naming the method.
const badHostPlugin = `package main

import "context"

type Host interface {
	NoSuchMethodOnRuntime(name string)
}

func Init(ctx context.Context) error         { return nil }
func Bind(ctx context.Context, h Host) error { return nil }
`

// missingBindPlugin exports Init and nothing else.
const missingBindPlugin = `package main

import "context"

func Init(ctx context.Context) error { return nil }
`

// buildPlugin writes source to a temporary module-local directory and builds it
// as a plugin, returning the .so path. It skips rather than fails when the
// toolchain cannot produce one, so the suite still runs on a host without cgo.
func buildPlugin(t *testing.T, name, source string) string {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}

	// The plugin has to be built inside this module so it resolves the same
	// go.mod, and under a directory the go tool ignores so it never joins
	// ./... . A testdata directory is both.
	dir := filepath.Join("testdata", "build", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Join("testdata", "build")) })

	if err := os.WriteFile(filepath.Join(dir, "plugin.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	out := filepath.Join(dir, "plugin.so")
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", out, "./"+filepath.ToSlash(dir))
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	if combined, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(combined), "not supported") {
			t.Skipf("go build -buildmode=plugin: %s", combined)
		}
		t.Fatalf("go build -buildmode=plugin: %v\n%s", err, combined)
	}
	return out
}

func newTestRuntime() *runner.Runtime {
	return runner.New(&strings.Builder{}, runner.Options{})
}

// TestPluginBindsThroughItsOwnInterface is the headline case: a plugin that
// names none of phpscript's types still receives the runtime and registers on
// it.
func TestPluginBindsThroughItsOwnInterface(t *testing.T) {
	so := buildPlugin(t, "host", hostPlugin)

	p, err := plugin.Open("", so)
	if err != nil {
		if errors.Is(err, plugin.ErrUnsupported) {
			t.Skipf("host cannot load plugins: %v", err)
		}
		t.Fatalf("Open: %v", err)
	}
	if p.Path() == "" || !filepath.IsAbs(p.Path()) {
		t.Fatalf("Path() = %q, want an absolute path", p.Path())
	}

	ctx := context.Background()
	rt := newTestRuntime()
	if err := p.Init(ctx, rt); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := p.Bind(ctx, rt); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if _, ok := rt.LookupConstructor("Greeter"); !ok {
		t.Fatal("Bind did not register Greeter")
	}

	// Init runs once per process however many times it is asked for, including
	// against a second runtime: it is process setup, not per-runtime setup.
	other := newTestRuntime()
	if err := p.Init(ctx, other); err != nil {
		t.Fatalf("Init (again): %v", err)
	}
	if err := p.Bind(ctx, other); err != nil {
		t.Fatalf("Bind (second runtime): %v", err)
	}
	assertInitCount(t, other, 1)

	// Bind runs per request, so running it again on the same runtime has to be
	// safe. It overwrites the same registrations.
	if err := p.Bind(ctx, rt); err != nil {
		t.Fatalf("Bind (again): %v", err)
	}
	assertInitCount(t, rt, 1)
}

// assertInitCount reads back the counter the plugin's Init incremented, through
// a constructor, so the assertion goes the whole way across the boundary.
func assertInitCount(t *testing.T, rt *runner.Runtime, want int64) {
	t.Helper()
	ctor, ok := rt.LookupConstructor("InitCount")
	if !ok {
		t.Fatal("InitCount is not registered")
	}
	fn, ok := ctor.(func() int64)
	if !ok {
		t.Fatalf("InitCount constructor is %T", ctor)
	}
	if got := fn(); got != want {
		t.Fatalf("Init ran %d times, want %d", got, want)
	}
}

// TestPluginSourceDoesNotImportPhpscript pins the decoupling claim itself. A
// plugin that named *runner.Runtime would have to be rebuilt whenever any
// phpscript package changed, which is the thing the Host interface avoids.
func TestPluginSourceDoesNotImportPhpscript(t *testing.T) {
	if strings.Contains(hostPlugin, "phpscript") {
		t.Fatal("the example plugin imports phpscript; it must declare its own Host interface instead")
	}
}

// TestOpenIsIdempotent pins that a path already open is not opened twice: a
// plugin cannot be unloaded, so loading has to be by identity.
func TestOpenIsIdempotent(t *testing.T) {
	so := buildPlugin(t, "idempotent", ctxOnlyPlugin)

	first, err := plugin.Open("", so)
	if err != nil {
		if errors.Is(err, plugin.ErrUnsupported) {
			t.Skipf("host cannot load plugins: %v", err)
		}
		t.Fatalf("Open: %v", err)
	}
	second, err := plugin.Open("", so)
	if err != nil {
		t.Fatalf("Open (again): %v", err)
	}
	if first != second {
		t.Fatal("Open returned a different Plugin for the same path")
	}
}

// TestBindWithoutHostParameter pins the shape a plugin uses when it needs no
// runtime.
func TestBindWithoutHostParameter(t *testing.T) {
	so := buildPlugin(t, "ctxonly", ctxOnlyPlugin)

	p, err := plugin.Open("", so)
	if err != nil {
		if errors.Is(err, plugin.ErrUnsupported) {
			t.Skipf("host cannot load plugins: %v", err)
		}
		t.Fatalf("Open: %v", err)
	}
	rt := newTestRuntime()
	if err := p.Bind(context.Background(), rt); err != nil {
		t.Fatalf("Bind: %v", err)
	}
}

// TestUnsatisfiableHostIsRefusedAtLoad pins that a bad host interface fails
// when the plugin is opened, naming the method, rather than on the first
// request.
func TestUnsatisfiableHostIsRefusedAtLoad(t *testing.T) {
	so := buildPlugin(t, "badhost", badHostPlugin)

	_, err := plugin.Open("", so)
	if errors.Is(err, plugin.ErrUnsupported) {
		t.Skipf("host cannot load plugins: %v", err)
	}
	if !errors.Is(err, plugin.ErrSymbolType) {
		t.Fatalf("Open error = %v, want ErrSymbolType", err)
	}
	if !strings.Contains(err.Error(), "NoSuchMethodOnRuntime") {
		t.Fatalf("Open error %q does not name the missing method", err)
	}
}

// TestMissingSymbolIsRefusedAtLoad pins that both entry points are required.
func TestMissingSymbolIsRefusedAtLoad(t *testing.T) {
	so := buildPlugin(t, "missingbind", missingBindPlugin)

	_, err := plugin.Open("", so)
	if errors.Is(err, plugin.ErrUnsupported) {
		t.Skipf("host cannot load plugins: %v", err)
	}
	if !errors.Is(err, plugin.ErrMissingSymbol) {
		t.Fatalf("Open error = %v, want ErrMissingSymbol", err)
	}
	if !strings.Contains(err.Error(), "Bind") {
		t.Fatalf("Open error %q does not name the missing symbol", err)
	}
}

// TestResolve covers the spellings a fixture is allowed to use.
func TestResolve(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "testdata", "example")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	so := filepath.Join(pluginDir, "plugin.so")
	if err := os.WriteFile(so, []byte("not really a plugin"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	base := filepath.Join(dir, "fixture.phpt")

	for _, name := range []string{
		"testdata/example",           // a directory holding plugin.so
		"testdata/example/plugin.so", // the file itself
		so,                           // absolute
	} {
		t.Run(name, func(t *testing.T) {
			got, err := plugin.Resolve(base, name)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", name, err)
			}
			if got != so {
				t.Fatalf("Resolve(%q) = %q, want %q", name, got, so)
			}
		})
	}
}

// TestResolveMissingNamesWhatItTried pins that a failure is diagnosable: the
// error lists every path considered.
func TestResolveMissingNamesWhatItTried(t *testing.T) {
	dir := t.TempDir()
	_, err := plugin.Resolve(filepath.Join(dir, "fixture.phpt"), "testdata/absent")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Resolve error = %v, want ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "tried ") {
		t.Fatalf("Resolve error %q does not list the paths tried", err)
	}
}

// TestLoadAllAndBindAllAreNoopsWhenEmpty pins that a fixture naming no plugins
// pays nothing, which is what keeps the harness change invisible to the other
// fixtures.
func TestLoadAllAndBindAllAreNoopsWhenEmpty(t *testing.T) {
	rt := newTestRuntime()
	plugins, err := plugin.LoadAll(context.Background(), rt, "", nil)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if plugins != nil {
		t.Fatalf("LoadAll returned %v, want nil", plugins)
	}
	if err := plugin.BindAll(context.Background(), rt, nil); err != nil {
		t.Fatalf("BindAll: %v", err)
	}
}
