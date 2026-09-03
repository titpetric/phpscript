// Package plugin loads compiled Go extensions and binds them to a Runtime.
//
// A plugin is a package main built with -buildmode=plugin that exports two
// functions:
//
//	func Init(ctx context.Context) error         // once per process
//	func Bind(ctx context.Context, h Host) error // once per request
//
// Init is for setup a plugin does once, and gets no runtime: the same plugin
// serves every runtime in the process. Bind installs the plugin's symbols on
// the runtime serving the current request, and runs again for the next one.
//
// Host is declared by the plugin, not imported from here. Go compares
// interface types structurally, so a plugin that writes
//
//	type Host interface {
//		RegisterConstructor(name string, ctor any)
//		Output() io.Writer
//	}
//
// receives a *runner.Runtime without naming it, and therefore without linking
// phpscript. That is what lets a plugin survive a phpscript rebuild. The
// constraint that remains is the Go toolchain: a plugin shares runtime,
// context and sync with its host, so both must be built by the same Go
// version, and any third-party package they both link must be the same
// version. Importing as little as possible is what makes a plugin portable.
//
// Bind should register constructors rather than functions. RegisterConstructor
// writes a map entry; RegisterFunc bumps the runtime's function generation,
// which invalidates its expression type environment, its compile configuration
// and every pooled evaluation environment. A per-request Bind that calls
// RegisterFunc undoes the runtime's caching on every request.
//
// A plugin cannot be unloaded, so loading is process-wide and permanent: Open
// returns the same Plugin for a path already open, and Init runs at most once
// for it.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// ErrUnsupported reports a host that cannot load plugins at all. Go plugins
// need cgo and a dynamically linked host; the released phpscript binary is
// built with CGO_ENABLED=0 so that it stays a single static file, and takes
// this path. A caller treats it the way it treats a missing interpreter: the
// work is skipped, not failed.
var ErrUnsupported = errors.New("plugin: unsupported build")

// ErrMissingSymbol reports a plugin that does not export Init or Bind.
var ErrMissingSymbol = errors.New("plugin: missing symbol")

// ErrSymbolType reports an exported Init or Bind whose signature the loader
// cannot call.
var ErrSymbolType = errors.New("plugin: wrong symbol type")

// ErrRegisteredBinding reports a plugin that contributed a process-global
// binding from its init(), which would install it into every runtime built
// afterwards rather than into the one Bind is given.
var ErrRegisteredBinding = errors.New("plugin: registered a process-global binding")

// contextType and errorType are the reflect types the symbol shapes are
// checked against.
var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
	runtimeType = reflect.TypeOf((*runner.Runtime)(nil))
)

// Plugin is one loaded extension.
type Plugin struct {
	path string
	init func(context.Context, *runner.Runtime) error
	bind func(context.Context, *runner.Runtime) error

	once    sync.Once
	initErr error
}

// Path returns the absolute path the plugin was loaded from.
func (p *Plugin) Path() string { return p.path }

// registry holds the plugins loaded by this process, keyed by resolved
// absolute path. A plugin cannot be unloaded, so an entry is never removed.
var registry struct {
	sync.Mutex
	byPath map[string]*Plugin
}

// Open loads the plugin named by name, resolved relative to base (see Resolve).
// Opening a path already open returns the plugin already loaded, so a plugin's
// own init() runs at most once per process.
func Open(base, name string) (*Plugin, error) {
	path, err := Resolve(base, name)
	if err != nil {
		return nil, err
	}

	registry.Lock()
	defer registry.Unlock()
	if loaded, ok := registry.byPath[path]; ok {
		return loaded, nil
	}

	// A plugin's init() runs inside openPlugin. One that calls
	// runner.RegisterBinding installs itself into every runtime constructed
	// afterwards, process-wide, which is the opposite of a per-request Bind and
	// would be invisible at the call site. Count the registry across the load
	// and refuse the plugin rather than let it act at a distance.
	before := len(runner.Bindings())
	opened, err := openPlugin(path)
	if err != nil {
		return nil, err
	}
	if len(runner.Bindings()) != before {
		return nil, fmt.Errorf("plugin %s: %w: register from Bind instead", path, ErrRegisteredBinding)
	}

	initFn, err := lookupEntry(opened, path, "Init")
	if err != nil {
		return nil, err
	}
	bindFn, err := lookupEntry(opened, path, "Bind")
	if err != nil {
		return nil, err
	}

	p := &Plugin{path: path, init: initFn, bind: bindFn}
	if registry.byPath == nil {
		registry.byPath = make(map[string]*Plugin)
	}
	registry.byPath[path] = p
	return p, nil
}

// Init runs the plugin's one-time setup. It runs at most once per process per
// plugin, and a failed Init is returned by every later call rather than
// retried, because a plugin that failed to set itself up cannot be re-loaded.
func (p *Plugin) Init(ctx context.Context, rt *runner.Runtime) error {
	p.once.Do(func() {
		p.initErr = p.call(ctx, rt, p.init, "Init")
	})
	return p.initErr
}

// Bind installs the plugin's symbols on rt. It runs on every request, so it
// must be cheap and it must be safe to run again: a re-bind overwrites the same
// registrations. rt must be a runtime the caller owns exclusively, because
// registration writes maps the runtime does not lock.
func (p *Plugin) Bind(ctx context.Context, rt *runner.Runtime) error {
	return p.call(ctx, rt, p.bind, "Bind")
}

// call invokes one of the plugin's entry points, converting a panic in plugin
// code into the error a panicking binding produces, so a host handles third
// party code the one way rather than two.
func (p *Plugin) call(ctx context.Context, rt *runner.Runtime, fn func(context.Context, *runner.Runtime) error, name string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &runner.HostPanicError{Callable: p.path + "." + name, Value: recovered}
		}
	}()
	return fn(ctx, rt)
}

// LoadAll opens each name relative to base and runs Init on each, in order.
func LoadAll(ctx context.Context, rt *runner.Runtime, base string, names []string) ([]*Plugin, error) {
	if len(names) == 0 {
		return nil, nil
	}
	plugins := make([]*Plugin, 0, len(names))
	for _, name := range names {
		p, err := Open(base, name)
		if err != nil {
			return nil, err
		}
		if err := p.Init(ctx, rt); err != nil {
			return nil, fmt.Errorf("plugin %s: init: %w", p.path, err)
		}
		plugins = append(plugins, p)
	}
	return plugins, nil
}

// BindAll runs Bind on each plugin in order. Order is significant: a later
// plugin overrides an earlier one under the same name, the way a plugin
// overrides the standard library.
func BindAll(ctx context.Context, rt *runner.Runtime, plugins []*Plugin) error {
	for _, p := range plugins {
		if err := p.Bind(ctx, rt); err != nil {
			return fmt.Errorf("plugin %s: bind: %w", p.path, err)
		}
	}
	return nil
}

// lookupEntry resolves one exported entry point and adapts it to the uniform
// shape the loader calls. The shape is resolved once, here, so a plugin with an
// unusable signature fails at load with a message naming the signature, rather
// than on the first request with a type assertion failure.
func lookupEntry(p symbols, path, name string) (func(context.Context, *runner.Runtime) error, error) {
	symbol, err := p.Lookup(name)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w: %s", path, ErrMissingSymbol, name)
	}
	adapted, err := adaptEntry(symbol)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %s: %w", path, name, err)
	}
	return adapted, nil
}

// adaptEntry wraps a plugin entry point in the loader's calling convention.
//
// Four shapes are accepted. The two-parameter ones take the host as an
// interface the plugin declared itself; reflect checks that *runner.Runtime
// implements it, which is a structural check and therefore does not require the
// plugin to have imported anything of ours. The one-parameter ones are for a
// plugin that needs no host, which Init usually does not.
func adaptEntry(symbol any) (func(context.Context, *runner.Runtime) error, error) {
	// The exact-match cases avoid reflection on the call path entirely. An
	// unnamed interface type is identical across packages, so a plugin
	// declaring its Host inline lands here rather than in the reflect path.
	switch fn := symbol.(type) {
	case func(context.Context) error:
		return func(ctx context.Context, _ *runner.Runtime) error { return fn(ctx) }, nil
	case func(context.Context):
		return func(ctx context.Context, _ *runner.Runtime) error { fn(ctx); return nil }, nil
	}

	typ := reflect.TypeOf(symbol)
	if typ == nil || typ.Kind() != reflect.Func {
		return nil, fmt.Errorf("%w: %T is not a function", ErrSymbolType, symbol)
	}
	if err := checkEntrySignature(typ); err != nil {
		return nil, err
	}

	value := reflect.ValueOf(symbol)
	returnsError := typ.NumOut() == 1
	hostArg := typ.NumIn() == 2

	return func(ctx context.Context, rt *runner.Runtime) error {
		in := make([]reflect.Value, 0, 2)
		in = append(in, reflect.ValueOf(ctx))
		if hostArg {
			in = append(in, reflect.ValueOf(rt))
		}
		out := value.Call(in)
		if !returnsError {
			return nil
		}
		err, _ := out[0].Interface().(error)
		return err
	}, nil
}

// checkEntrySignature reports whether typ is a shape the loader can call, and
// says what is wrong when it is not. The host parameter has to be an interface
// *runner.Runtime satisfies: a plugin that asks for a method the runtime does
// not have is a mistake worth naming at load.
func checkEntrySignature(typ reflect.Type) error {
	if typ.IsVariadic() || typ.NumIn() < 1 || typ.NumIn() > 2 {
		return fmt.Errorf("%w: %s, want func(context.Context[, Host]) [error]", ErrSymbolType, typ)
	}
	if typ.In(0) != contextType {
		return fmt.Errorf("%w: %s takes %s first, want context.Context", ErrSymbolType, typ, typ.In(0))
	}
	if typ.NumOut() > 1 || (typ.NumOut() == 1 && typ.Out(0) != errorType) {
		return fmt.Errorf("%w: %s, want no result or error", ErrSymbolType, typ)
	}
	if typ.NumIn() == 2 {
		host := typ.In(1)
		if host.Kind() != reflect.Interface {
			return fmt.Errorf("%w: host parameter is %s, want an interface", ErrSymbolType, host)
		}
		if !runtimeType.Implements(host) {
			return fmt.Errorf("%w: the runtime does not implement %s (%s)", ErrSymbolType, host, missingMethods(host))
		}
	}
	return nil
}

// missingMethods lists the methods of host that *runner.Runtime does not have,
// so an unsatisfiable host interface reports which method to drop rather than
// only that the interface did not match.
func missingMethods(host reflect.Type) string {
	var missing []string
	for i := range host.NumMethod() {
		want := host.Method(i)
		got, ok := runtimeType.MethodByName(want.Name)
		if !ok {
			missing = append(missing, "missing "+want.Name+want.Type.String()[4:])
			continue
		}
		// A method value on a concrete type carries the receiver as its first
		// parameter, which the interface method type does not.
		if got.Type.NumIn()-1 != want.Type.NumIn() || got.Type.NumOut() != want.Type.NumOut() {
			missing = append(missing, want.Name+" has a different signature")
		}
	}
	if len(missing) == 0 {
		return "method set differs"
	}
	return strings.Join(missing, ", ")
}
