package runner

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/telemetry"
)

// This file turns a path into a running program: parsing an entrypoint,
// resolving and evaluating an include, the autoload queue a missing class
// walks, and the path rules every spelling resolves through. The statement
// interpreter the loaded program runs on lives in runner.go.

// IncludeFunc resolves an include/require path to a parsed program. Wiring this
// from the host keeps the runner free of file-system and parser dependencies.
type IncludeFunc func(path string) (*model.Program, error)

// SetIncludeResolver installs the include/require resolver.
func (rt *Runtime) SetIncludeResolver(fn IncludeFunc) { rt.include = fn }

// RegisterInclude installs a host implementation of one include target, for a
// file the runtime reimplements in Go: composer's vendor/autoload.php.
func (rt *Runtime) RegisterInclude(path string, fn func() (any, error)) {
	if rt.includeHooks == nil {
		rt.includeHooks = map[string]func() (any, error){}
	}
	rt.includeHooks[cleanFSPath(path)] = fn
}

// Load parses PHP source, naming no file, into a program.
func (rt *Runtime) Load(src string) (*model.Program, error) {
	return rt.loadNamed("", src)
}

// loadNamed parses src as the contents of name, which is the path a script
// sees. An empty name is source with no file behind it.
func (rt *Runtime) loadNamed(name, src string) (*model.Program, error) {
	rt.UpdateStatus(telemetry.StateReading)
	program, err := parser.ParseFile(name, src)
	if err != nil {
		rt.UpdateStatus(telemetry.StateError)
	}
	return program, err
}

// LoadFile reads and parses a PHP file from the runtime source FS.
//
// Options.Precompile makes the include cache answer for entrypoints as well as
// for includes: the runtime parses the file once for the life of the process,
// and each request reads back the bytecode and the compiled expressions keyed by
// that AST and builds neither again. Without it every load reads and parses the
// file again, as a CLI run and an unconfigured host do.
func (rt *Runtime) LoadFile(path string) (*model.Program, error) {
	rt.UpdateFilename(path)
	rt.UpdateStatus(telemetry.StateReading)
	if rt.opts.RootFS == nil {
		rt.UpdateStatus(telemetry.StateError)
		return nil, fmt.Errorf("load %q: no source FS configured", path)
	}
	resolved := rt.resolveFSPath(path)
	if !rt.opts.Precompile {
		return rt.loadResolved(resolved)
	}
	if prog, ok := rt.includeCache.Get(resolved); ok {
		return prog, nil
	}
	prog, err := rt.loadResolved(resolved)
	if err != nil {
		return nil, err
	}
	rt.includeCache.Set(resolved, prog)
	return prog, nil
}

// loadResolved reads and parses a path the caller has already put through
// resolveFSPath. Resolution happens once per include, above the cache lookup,
// and the cache keys on the file and not on the spelling: after a chdir the
// same "x.php" names a different file, and a cache keyed on the spelling would
// answer with the previous one.
func (rt *Runtime) loadResolved(cleanPath string) (*model.Program, error) {
	b, err := fs.ReadFile(rt.opts.RootFS, cleanPath)
	if err != nil {
		rt.UpdateStatus(telemetry.StateError)
		return nil, fmt.Errorf("load %q: %w", cleanPath, err)
	}
	prog, err := rt.loadNamed(rootPath(cleanPath), string(b))
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", cleanPath, err)
	}
	return prog, nil
}

// runPrelude includes Options.Include ahead of this request's entrypoint and
// the entrypoint and everything it includes see what it declared.
//
// runPrelude skips a file that is not there: the option names what to load
// when the application carries it and one configuration covers a tree where
// the bootstrap file it names does not exist yet.
func (rt *Runtime) runPrelude() error {
	if rt.preludeDone || rt.opts.Include == "" || rt.opts.RootFS == nil {
		return nil
	}
	rt.preludeDone = true
	name := strings.TrimPrefix(path.Clean(filepath.ToSlash(rt.opts.Include)), "/")
	if _, err := fs.Stat(rt.opts.RootFS, name); err != nil {
		return nil
	}
	_, err := rt.IncludeFile(name)
	return err
}

func (rt *Runtime) evalInclude(n *model.Include, scope *Scope) (any, error) {
	path, err := rt.Eval(n.Path, scope)
	if err != nil {
		return nil, err
	}
	return rt.includeFile(phpString(path), n.Once, scope)
}

// markIncluded records a file as loaded without running it, for a caller that
// installed its declarations another way. include_once and require_once answer
// off this list.
func (rt *Runtime) markIncluded(path string) {
	name := rootPath(rt.resolveFSPath(path))
	if slices.Contains(rt.included, name) {
		return
	}
	rt.included = append(rt.included, name)
	rt.UpdateIncludedFiles(len(rt.included))
}

// includeFile evaluates one include. once is the *_once form, which
// includeFile answers here and not at the call site so that both engines
// dedupe on the same thing: the resolved file, never the spelling the script
// used. After a chdir two directories can spell one name, and a scan over
// spellings would skip the second file as though it had already run.
func (rt *Runtime) includeFile(path string, once bool, scope *Scope) (any, error) {
	path = rt.includeFallback(path, scope)
	resolved := rt.resolveFSPath(path)
	if once && slices.Contains(rt.included, rootPath(resolved)) {
		return true, nil
	}

	kind := telemetry.KindInternal
	if strings.EqualFold(filepath.Ext(path), ".tpl") {
		kind = telemetry.KindTemplate
	}
	defer rt.trace(scope, "include "+path, kind)()

	if hook, ok := rt.includeHooks[resolved]; ok {
		rt.included = append(rt.included, rootPath(resolved))
		rt.UpdateIncludedFiles(len(rt.included))
		return hook()
	}

	prog, filename, err := rt.resolveInclude(path)
	if err != nil {
		return nil, fmt.Errorf("error including %s: %w", path, err)
	}
	rt.addSourceSpans(prog)
	if rt.coverage != nil {
		rt.coverage.Register(filename, prog)
	}
	rt.included = append(rt.included, rootPath(filename))
	rt.UpdateIncludedFiles(len(rt.included))
	restoreFile := rt.pushScopeFile(scope, rootPath(filename))
	defer restoreFile.restore(scope)
	if err := rt.hoist(prog, rootPath(filename)); err != nil {
		return nil, err
	}
	deferMark := len(scope.deferred)
	value, fl, runErr := rt.exec(prog.Stmts, scope)
	err = combineErrors(runErr, rt.runDeferred(scope, deferMark))
	if err != nil {
		return nil, err
	}
	if fl == flowReturn {
		return value, nil
	}
	// PHP include/require constructs answer 1 when the included file
	// reaches its end without an explicit return.
	return int64(1), nil
}

func (rt *Runtime) autoload(class string, scope *Scope) error {
	for _, loader := range rt.autoloaders {
		// Every PHP callable spelling reaches here: composer registers
		// array($this, "loadClass"), its bootstrap registers the
		// "Class::method" string form, and a plain function name or closure is
		// just as valid.
		callable, ok := rt.callableWithScope(scope, loader)
		if !ok {
			return fmt.Errorf("autoload callback %v is not callable", loader)
		}
		if _, err := callable(class); err != nil {
			return err
		}
		if rt.hasClass(class) {
			return nil
		}
	}
	return nil
}

// UnregisterAutoloader removes a callback from the SPL autoload queue, matching
// it the way PHP does: by the function, object and method it names, and not
// by identity, since each `array($this, "loadClass")` is a fresh array.
func (rt *Runtime) UnregisterAutoloader(callback any) bool {
	want := autoloaderKey(callback)
	for i, loader := range rt.autoloaders {
		if autoloaderKey(loader) != want {
			continue
		}
		rt.autoloaders = append(rt.autoloaders[:i], rt.autoloaders[i+1:]...)
		return true
	}
	return false
}

// autoloaderKey renders a registered callback as a comparable identity.
func autoloaderKey(callback any) string {
	switch value := callback.(type) {
	case string:
		return "fn:" + value
	case *model.Array:
		if value.Len() != 2 {
			break
		}
		target, _ := value.Get(int64(0))
		method, _ := value.Get(int64(1))
		if object, ok := target.(*model.Object); ok {
			return fmt.Sprintf("method:%p:%v", object, method)
		}
		return fmt.Sprintf("static:%v:%v", target, method)
	}
	// A closure or a host func has no PHP-visible name and its code pointer is
	// the only identity available.
	if rv := reflect.ValueOf(callback); rv.IsValid() {
		switch rv.Kind() {
		case reflect.Func, reflect.Pointer, reflect.Map, reflect.Slice, reflect.UnsafePointer:
			return fmt.Sprintf("value:%v", rv.Pointer())
		}
	}
	return fmt.Sprintf("value:%v", callback)
}

// SPLAutoload implements PHP's default autoloader: lowercase the qualified
// class name and search each include_path entry for class.php.
func (rt *Runtime) SPLAutoload(class string) error {
	class = strings.ToLower(strings.TrimPrefix(class, "\\"))
	class = strings.ReplaceAll(class, "\\", "/")
	for _, dir := range filepath.SplitList(rt.includePath) {
		if dir == "" {
			dir = "."
		}
		for _, ext := range []string{".php"} {
			filename := path.Join(filepath.ToSlash(dir), class+ext)
			cleanPath := rt.resolveFSPath(filename)
			if rt.opts.RootFS == nil {
				continue
			}
			if _, err := fs.Stat(rt.opts.RootFS, cleanPath); err != nil {
				continue
			}
			_, err := rt.includeFile(filename, false, rt.newScope())
			return err
		}
	}
	return nil
}

// includeFallback is PHP's last stop for a relative include: when the working
// directory holds no such path, it retries in the directory of the calling
// script. A path written as ./ or ../ names the working directory only, and
// one written from the root has nothing to fall back to.
func (rt *Runtime) includeFallback(name string, scope *Scope) string {
	slash := filepath.ToSlash(name)
	if rt.include != nil || rt.opts.RootFS == nil || slash == "" ||
		strings.HasPrefix(slash, "/") || strings.HasPrefix(slash, "./") || strings.HasPrefix(slash, "../") {
		return name
	}
	if _, err := fs.Stat(rt.opts.RootFS, rt.resolveFSPath(name)); err == nil {
		return name
	}
	dir, ok := scope.Get("__DIR__")
	if !ok {
		return name
	}
	candidate := rootPath(path.Join(fmt.Sprint(dir), slash))
	if _, err := fs.Stat(rt.opts.RootFS, rt.resolveFSPath(candidate)); err != nil {
		return name
	}
	return candidate
}

// resolveInclude turns an include path into a parsed program, preferring an
// explicit IncludeFunc resolver and otherwise reading from the configured fs.FS.
func (rt *Runtime) resolveInclude(path string) (*model.Program, string, error) {
	if rt.include != nil {
		prog, err := rt.include(path)
		return prog, cleanFSPath(path), err
	}
	if rt.opts.RootFS != nil {
		cleanPath := rt.resolveFSPath(path)
		if prog, ok := rt.includeCache.Get(cleanPath); ok {
			return prog, cleanPath, nil
		}

		prog, err := rt.loadResolved(cleanPath)
		if err != nil {
			return nil, "", fmt.Errorf("include %q: %w", path, err)
		}
		rt.includeCache.Set(cleanPath, prog)
		return prog, cleanPath, nil
	}
	return nil, "", fmt.Errorf("include %q: no resolver or source FS configured", path)
}

// cleanFSPath normalises an include path for fs.FS, which requires slash-rooted,
// unrooted, dot-free paths.
func cleanFSPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	return path.Clean(p)
}

// clampFSPath cleans p against the root: the clean collapses a ".." that would
// climb above the root and "a/../../etc/passwd" names etc/passwd inside it.
// Cleaning happens against "/" first, which does the collapsing; path.Clean on
// its own keeps a leading "..".
func clampFSPath(p string) string {
	clean := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(p)), "/")
	if clean == "" {
		return "."
	}
	return clean
}

// rootPath spells an fs.FS path the way a script reads it. The source
// filesystem sits at "/" and a name below it starts from there, and that is
// what __FILE__, __DIR__ and getcwd() answer with.
//
// The point of the leading slash is that resolveFSPath leaves it alone: a path
// written from the root names one file whatever the working directory is, the
// way an absolute path does in PHP. Without it `include __DIR__ . "/x.php"`
// would be re-resolved against the working directory and name a different file
// after every chdir, and composer's autoloader depends on that.
func rootPath(p string) string {
	clean := clampFSPath(p)
	if clean == "." {
		return "/"
	}
	return "/" + clean
}

// resolveFSPath maps a path a script supplied onto the source filesystem,
// through the working directory chdir() moved.
//
// A path written from the root passes through as it stands; anything else is
// relative to the working directory. clampFSPath cleans both against the root,
// and neither can climb out of it.
func (rt *Runtime) resolveFSPath(p string) string {
	slash := filepath.ToSlash(p)
	if strings.HasPrefix(slash, "/") {
		return clampFSPath(slash)
	}
	if rt.opts.WorkDir == "" || rt.opts.WorkDir == "." {
		return clampFSPath(slash)
	}
	// The join happens before the clamp and never after and a ".." climbs out
	// of the working directory the way it would on a real filesystem and stops
	// at the root, not at the directory the script happens to be in.
	return clampFSPath(rt.opts.WorkDir + "/" + slash)
}
