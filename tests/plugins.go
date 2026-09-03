package tests

// This file is the fixture harness's half of the Go plugin support in
// runner/plugin: the `plugins:` metadata key, resolving what it names, and
// building the .so when it is missing or stale.
//
// A plugin has to be built by the toolchain that opens it, so the harness
// builds it rather than trusting an artifact in the tree. That is what makes
// the toolchain match a guarantee instead of a convention; set
// PHPSCRIPT_PLUGIN_BUILD=0 for a host that ships prebuilt plugins.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/plugin"
)

// Plugins names the Go plugins a fixture loads, relative to the fixture's own
// directory. It accepts the two spellings a fixture is likely to use, a
// whitespace-separated string and a list:
//
//	plugins: ../../testdata/plugins/http/plugin.so
//	plugins: [../../testdata/plugins/basic/plugin.so]
//
// A name without a .so suffix names a directory holding plugin.so.
type Plugins []string

// UnmarshalYAML accepts either spelling. goccy calls this for the `plugins`
// key, so no change to ParseFixture is needed.
func (p *Plugins) UnmarshalYAML(unmarshal func(any) error) error {
	var list []string
	if err := unmarshal(&list); err == nil {
		*p = list
		return nil
	}
	var value string
	if err := unmarshal(&value); err != nil {
		return fmt.Errorf("plugins: want a string or a list of strings: %w", err)
	}
	*p = strings.Fields(value)
	return nil
}

// builds serialises the build of one plugin directory, so the two fixture
// runners do not build the same plugin twice and concurrent fixtures naming the
// same plugin wait rather than race on the output file.
var builds sync.Map // string -> *sync.Once

// buildErrs holds the outcome of each build, so a second waiter sees the same
// error rather than a missing file.
var buildErrs sync.Map // string -> error

// loadPlugins resolves the fixture's plugins, builds any that are missing or
// stale, and runs Init on each. It returns nil for a fixture that names none,
// which is every fixture but a handful, so the cost is a length check.
func (f *Fixture) loadPlugins(ctx context.Context, rt *runner.Runtime) ([]*plugin.Plugin, error) {
	if len(f.Plugins) == 0 {
		return nil, nil
	}
	for _, name := range f.Plugins {
		if err := buildPlugin(f.Path, name); err != nil {
			return nil, err
		}
	}
	return plugin.LoadAll(ctx, rt, f.Path, f.Plugins)
}

// buildPlugin compiles the plugin named by name when its .so is missing or
// older than a .go file beside it. A name that resolves to a directory with no
// Go sources is left alone: it is a prebuilt artifact the host supplied.
func buildPlugin(base, name string) error {
	if os.Getenv("PHPSCRIPT_PLUGIN_BUILD") == "0" {
		return nil
	}

	dir, out, ok := pluginSource(base, name)
	if !ok {
		return nil
	}

	once, _ := builds.LoadOrStore(out, &sync.Once{})
	once.(*sync.Once).Do(func() {
		buildErrs.Store(out, runPluginBuild(dir, out))
	})
	if err, ok := buildErrs.Load(out); ok && err != nil {
		return err.(error)
	}
	return nil
}

// pluginSource locates the source directory for a plugin name and the .so it
// builds to, reporting whether there is a directory of Go sources to build. It
// deliberately does not use plugin.Resolve, which requires the .so to exist.
func pluginSource(base, name string) (dir, out string, ok bool) {
	trimmed := strings.TrimSuffix(filepath.Clean(name), ".so")
	if strings.HasSuffix(name, ".so") {
		trimmed = filepath.Dir(filepath.Clean(name))
	}

	baseDir := "."
	if base != "" {
		baseDir = filepath.Dir(base)
	}
	for _, candidate := range []string{
		filepath.Join(baseDir, trimmed),
		trimmed,
	} {
		sources, err := filepath.Glob(filepath.Join(candidate, "*.go"))
		if err != nil || len(sources) == 0 {
			continue
		}
		return candidate, filepath.Join(candidate, "plugin.so"), true
	}
	return "", "", false
}

// runPluginBuild builds dir as a plugin, unless the existing .so is newer than
// every source beside it.
//
// The build passes neither -trimpath nor -ldflags, because neither `go test`
// nor `go install .` does: a plugin has to match the binary that opens it, and
// the -trimpath release binary is built without cgo and never opens one.
func runPluginBuild(dir, out string) error {
	stale, err := pluginIsStale(dir, out)
	if err != nil {
		return err
	}
	if !stale {
		return nil
	}

	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", filepath.Base(out), ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	if combined, buildErr := cmd.CombinedOutput(); buildErr != nil {
		return fmt.Errorf("build plugin %s: %w\n%s", dir, buildErr, combined)
	}
	return nil
}

// pluginIsStale reports whether out has to be rebuilt from the sources in dir.
func pluginIsStale(dir, out string) (bool, error) {
	info, err := os.Stat(out)
	if err != nil {
		return true, nil
	}
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return false, err
	}
	for _, source := range sources {
		sourceInfo, err := os.Stat(source)
		if err != nil {
			return false, err
		}
		if sourceInfo.ModTime().After(info.ModTime()) {
			return true, nil
		}
	}
	return false, nil
}
