package annotations_test

import (
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/annotations"
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
