package tests_test

// The plugin fixtures rest on a claim: a plugin declares the interface it needs
// and therefore links none of phpscript, so it keeps loading across a phpscript
// rebuild. A plugin that quietly acquired an import would still pass every
// fixture, right up until the day someone changed an unrelated package and
// plugin.Open started refusing it. These tests are what make the claim
// falsifiable.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/tests"
)

// pluginDirs are the plugin sources shipped with the fixtures. They live under
// testdata so the go tool never folds them into ./..., and outside fixtures/ so
// a built .so never reaches the embedded fixture tree.
var pluginDirs = []string{
	"testdata/plugins/basic",
	"testdata/plugins/http",
}

// TestPluginsLinkNoPhpscriptPackage is the decoupling check. The only
// phpscript path a plugin is allowed to report is its own, which it has
// because the sources live inside this module.
func TestPluginsLinkNoPhpscriptPackage(t *testing.T) {
	for _, dir := range pluginDirs {
		t.Run(dir, func(t *testing.T) {
			out, err := exec.Command("go", "list", "-deps", "./"+dir).Output()
			if err != nil {
				t.Fatalf("go list -deps ./%s: %v", dir, err)
			}
			self := "github.com/titpetric/phpscript/tests/" + dir

			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if !strings.Contains(line, "titpetric/phpscript") || line == self {
					continue
				}
				t.Errorf("plugin %s imports %s; it must declare the interface it needs "+
					"instead, so it survives a phpscript rebuild", dir, line)
			}
		})
	}
}

// TestPluginsDeclareBothEntryPoints pins the ABI at the source level, so a
// plugin missing one is a readable failure here rather than an ErrMissingSymbol
// from inside a fixture.
func TestPluginsDeclareBothEntryPoints(t *testing.T) {
	for _, dir := range pluginDirs {
		t.Run(dir, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(dir, "plugin.go"))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			for _, want := range []string{"func Init(ctx context.Context) error", "func Bind(ctx context.Context"} {
				if !strings.Contains(string(source), want) {
					t.Errorf("plugin %s does not declare %q", dir, want)
				}
			}
		})
	}
}

// TestPluginFixturesOptOutOfPHP pins that a fixture loading a plugin says so.
// ParseFixture enforces this, and this test is what proves the enforcement is
// reached rather than a branch nobody takes.
func TestPluginFixturesOptOutOfPHP(t *testing.T) {
	broken := []byte(`name: plugin fixture without the opt-out
description: A fixture that loads a plugin has to opt the php runner out.
plugins: ../../testdata/plugins/basic/plugin.so
---
<?php echo "x";
---
x
`)
	_, err := tests.ParseFixture(broken, "fixtures/broken.phpt")
	if err == nil {
		t.Fatal("ParseFixture accepted a plugin fixture that still runs against php")
	}
	if !strings.Contains(err.Error(), "php: false") {
		t.Errorf("error %q does not say how to fix it", err)
	}
}

// TestPluginsUnmarshalYAML pins both spellings of the metadata key.
func TestPluginsUnmarshalYAML(t *testing.T) {
	cases := map[string][]string{
		"plugins: testdata/basic/plugin.so":                            {"testdata/basic/plugin.so"},
		"plugins: testdata/basic/plugin.so testdata/http/plugin.so":    {"testdata/basic/plugin.so", "testdata/http/plugin.so"},
		"plugins: [testdata/basic/plugin.so, testdata/http/plugin.so]": {"testdata/basic/plugin.so", "testdata/http/plugin.so"},
	}
	for line, want := range cases {
		t.Run(line, func(t *testing.T) {
			source := []byte("name: n\ndescription: d\nrunner:\n  php: false\n" + line + "\n---\n<?php\n---\n")
			fx, err := tests.ParseFixture(source, "fixtures/x.phpt")
			if err != nil {
				t.Fatalf("ParseFixture: %v", err)
			}
			if len(fx.Plugins) != len(want) {
				t.Fatalf("Plugins = %v, want %v", fx.Plugins, want)
			}
			for i, name := range want {
				if fx.Plugins[i] != name {
					t.Fatalf("Plugins = %v, want %v", fx.Plugins, want)
				}
			}
		})
	}
}
