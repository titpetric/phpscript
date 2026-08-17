package composer_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/composer"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// project is a composer install as it appears on disk: a root manifest, the
// generated vendor/composer/installed.json, an empty generated autoload.php,
// and one PSR-4 package.
func project() fstest.MapFS {
	return fstest.MapFS{
		"composer.json": &fstest.MapFile{Data: []byte(`{
			"autoload": {"psr-4": {"App\\": "src/"}, "files": ["src/helpers.php"]},
			"require": {"acme/greeter": "@dev"}
		}`)},
		"src/App.php":     &fstest.MapFile{Data: []byte("<?php\nnamespace App;\nclass App { function name() { return \"app\"; } }\n")},
		"src/helpers.php": &fstest.MapFile{Data: []byte("<?php\nfunction greet_prefix() { return \"hello, \"; }\n")},
		"vendor/autoload.php": &fstest.MapFile{Data: []byte(
			"<?php\n// composer's generated bootstrap; phpscript never parses this.\nreturn ComposerAutoloaderInit::getLoader();\n")},
		"vendor/composer/installed.json": &fstest.MapFile{Data: []byte(`{
			"packages": [
				{
					"name": "acme/greeter",
					"autoload": {"psr-4": {"Acme\\Greeter\\": "src/"}},
					"install-path": "../acme/greeter"
				}
			]
		}`)},
		"vendor/acme/greeter/src/Greeter.php": &fstest.MapFile{Data: []byte(
			"<?php\nnamespace Acme\\Greeter;\nclass Greeter { function hello($who) { return greet_prefix() . $who; } }\n")},
	}
}

func TestDiscover(t *testing.T) {
	p, ok, err := composer.Discover(project(), "src")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !ok {
		t.Fatal("Discover found no project; it should walk up from src/")
	}
	if p.Dir != "." {
		t.Errorf("Dir = %q, want %q", p.Dir, ".")
	}
	if p.AutoloadFile() != "vendor/autoload.php" {
		t.Errorf("AutoloadFile = %q, want vendor/autoload.php", p.AutoloadFile())
	}
	if got, want := strings.Join(p.Files, ","), "src/helpers.php"; got != want {
		t.Errorf("Files = %q, want %q", got, want)
	}
}

func TestDiscoverWithoutManifest(t *testing.T) {
	_, ok, err := composer.Discover(fstest.MapFS{"index.php": &fstest.MapFile{}}, ".")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if ok {
		t.Error("Discover reported a project for a tree with no composer.json")
	}
}

func TestResolve(t *testing.T) {
	p, _, err := composer.Discover(project(), ".")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	cases := map[string]string{
		"App\\App":                  "src/App.php",
		"\\App\\Nested\\Thing":      "src/Nested/Thing.php",
		"Acme\\Greeter\\Greeter":    "vendor/acme/greeter/src/Greeter.php",
		"Acme\\Greeter\\Sub\\Thing": "vendor/acme/greeter/src/Sub/Thing.php",
	}
	for class, want := range cases {
		got := p.Resolve(class)
		if len(got) == 0 {
			t.Errorf("Resolve(%q) = nothing, want %q", class, want)
			continue
		}
		if got[0] != want {
			t.Errorf("Resolve(%q) = %q, want %q", class, got[0], want)
		}
	}
	if got := p.Resolve("Unmapped\\Thing"); len(got) != 0 {
		t.Errorf("Resolve of an unmapped class = %v, want nothing", got)
	}
}

func TestResolvePSR0(t *testing.T) {
	fsys := fstest.MapFS{
		"composer.json": &fstest.MapFile{Data: []byte(`{"autoload": {"psr-0": {"Legacy\\": "lib"}}}`)},
	}
	p, _, err := composer.Discover(fsys, ".")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// PSR-0 keeps the full class name in the path and expands underscores.
	if got := p.Resolve("Legacy\\Deep\\Class_Name"); len(got) == 0 || got[0] != "lib/Legacy/Deep/Class/Name.php" {
		t.Errorf("Resolve = %v, want lib/Legacy/Deep/Class/Name.php", got)
	}
}

// TestRegisterAutoloadsOnInclude is the end-to-end contract: nothing is
// autoloadable until the script includes vendor/autoload.php, and after it does,
// vendor classes resolve without any further include.
func TestRegisterAutoloadsOnInclude(t *testing.T) {
	fsys := project()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{RootFS: fsys})
	stdlib.Register(rt)
	if err := composer.Register(rt, fsys, "."); err != nil {
		t.Fatalf("Register: %v", err)
	}

	prog, err := rt.Load(`<?php
include "vendor/autoload.php";
$g = new Acme\Greeter\Greeter();
echo $g->hello("world");
`)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := rt.Run(prog); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// "hello, " comes from autoload.files, "world" from the autoloaded class.
	if got := out.String(); got != "hello, world" {
		t.Errorf("output = %q, want %q", got, "hello, world")
	}
}

func TestRegisterIsInertWithoutInclude(t *testing.T) {
	fsys := project()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{RootFS: fsys})
	stdlib.Register(rt)
	if err := composer.Register(rt, fsys, "."); err != nil {
		t.Fatalf("Register: %v", err)
	}

	prog, err := rt.Load("<?php $g = new Acme\\Greeter\\Greeter();")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := rt.Run(prog); err == nil {
		t.Fatal("constructing a vendor class succeeded without including the autoloader")
	}
}

// TestRegisterWithoutGeneratedAutoloader covers a checkout where composer has
// not run: the include must fail the way PHP's would, not silently succeed.
func TestRegisterWithoutGeneratedAutoloader(t *testing.T) {
	fsys := fstest.MapFS{
		"composer.json": &fstest.MapFile{Data: []byte(`{"autoload": {"psr-4": {"App\\": "src/"}}}`)},
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{RootFS: fsys})
	stdlib.Register(rt)
	if err := composer.Register(rt, fsys, "."); err != nil {
		t.Fatalf("Register: %v", err)
	}

	prog, err := rt.Load(`<?php include "vendor/autoload.php";`)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := rt.Run(prog); err == nil {
		t.Fatal("include of a missing vendor/autoload.php succeeded")
	}
}

// TestScanVendorFallback covers a vendor tree without installed.json, which is
// what a hand-assembled or partially generated vendor directory looks like.
func TestScanVendorFallback(t *testing.T) {
	fsys := fstest.MapFS{
		"composer.json":                       &fstest.MapFile{Data: []byte(`{}`)},
		"vendor/autoload.php":                 &fstest.MapFile{Data: []byte("<?php")},
		"vendor/acme/greeter/composer.json":   &fstest.MapFile{Data: []byte(`{"name": "acme/greeter", "autoload": {"psr-4": {"Acme\\": "src/"}}}`)},
		"vendor/acme/greeter/src/Greeter.php": &fstest.MapFile{Data: []byte("<?php")},
	}
	p, _, err := composer.Discover(fsys, ".")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := p.Resolve("Acme\\Greeter"); len(got) == 0 || got[0] != "vendor/acme/greeter/src/Greeter.php" {
		t.Errorf("Resolve = %v, want vendor/acme/greeter/src/Greeter.php", got)
	}
}
