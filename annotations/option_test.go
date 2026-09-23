package annotations_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/runner"
)

var testModuleFileSystem = fstest.MapFS{}

func TestModuleNamesDefault(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"route", annotations.NewRoute(testModuleFileSystem).Name(), "phproute"},
		{"startup", annotations.NewStartup(testModuleFileSystem).Name(), "phpstartup"},
		{"schedule", annotations.NewScheduler(testModuleFileSystem).Name(), "phpschedule"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("Name() = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestModuleNamesWithSuffix(t *testing.T) {
	option := annotations.WithModuleSuffix("example.com")

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"route", annotations.NewRoute(testModuleFileSystem, option).Name(), "phproute:example.com"},
		{"startup", annotations.NewStartup(testModuleFileSystem, option).Name(), "phpstartup:example.com"},
		{"schedule", annotations.NewScheduler(testModuleFileSystem, option).Name(), "phpschedule:example.com"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("Name() = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestModuleSuffixEmpty(t *testing.T) {
	cases := []struct {
		name   string
		suffix string
	}{
		{"empty", ""},
		{"whitespace", "   \t "},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			option := annotations.WithModuleSuffix(test.suffix)
			if got := annotations.NewRoute(testModuleFileSystem, option).Name(); got != "phproute" {
				t.Fatalf("Name() = %q, want %q", got, "phproute")
			}
			if got := annotations.NewStartup(testModuleFileSystem, option).Name(); got != "phpstartup" {
				t.Fatalf("Name() = %q, want %q", got, "phpstartup")
			}
			if got := annotations.NewScheduler(testModuleFileSystem, option).Name(); got != "phpschedule" {
				t.Fatalf("Name() = %q, want %q", got, "phpschedule")
			}
		})
	}
}

// TestWithIncludeCache covers the cache a host that precompiled its tree hands
// over: the endpoint reads its entrypoint back out of it rather than parsing
// the file, so the program that ran is the one the cache holds.
func TestWithIncludeCache(t *testing.T) {
	cache := runner.NewIncludeCache()
	mux := newTestMux(t,
		annotations.WithIncludeCache(cache),
		annotations.WithRunnerOptions(runner.Options{Precompile: true}),
	)

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || response.Body.String() != "home" {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}

	if _, ok := cache.Get("index.php"); !ok {
		t.Fatal("the endpoint did not read through the supplied include cache")
	}
}
