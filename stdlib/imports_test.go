package stdlib_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestRegisterInstallsEveryImportedArea covers the wiring the blank imports
// do. Each binding package contributes its installer through
// runner.RegisterBinding in its init() and importing this package is what puts
// the area in the binary and Register is what puts it on a runtime. A package
// dropped from the import list would leave its names undefined with nothing
// else failing and this test names one per area.
func TestRegisterInstallsEveryImportedArea(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	for _, test := range []struct {
		area string
		name string
	}{
		{area: "compat", name: "ob_start"},
		{area: "core", name: "strlen"},
		{area: "crypto", name: "password_hash"},
		{area: "files", name: "fopen"},
		{area: "gd", name: "imagecolorallocate"},
		{area: "info", name: "phpinfo"},
		{area: "internals", name: "memory_get_usage"},
		{area: "pexec", name: "exec"},
		{area: "regexp", name: "preg_match"},
		{area: "span", name: "start_span"},
		{area: "time", name: "DateTime::now"},
	} {
		t.Run(test.area, func(t *testing.T) {
			if _, ok := rt.LookupFunc(test.name); !ok {
				t.Errorf("%s() is undefined, so stdlib/%s reached no runtime", test.name, test.area)
			}
		})
	}

	for _, test := range []struct {
		area string
		name string
	}{
		{area: "auth", name: "Auth\\Storage\\PasswordFile"},
		{area: "database", name: "Database"},
		{area: "http", name: "HTTP\\Client"},
		{area: "mail", name: "Mail"},
		{area: "regexp", name: "Regexp\\Compile"},
		{area: "session", name: "Session\\Manager"},
		{area: "time", name: "Time"},
	} {
		t.Run(test.area+" constructor", func(t *testing.T) {
			if _, ok := rt.LookupConstructor(test.name); !ok {
				t.Errorf("new %s() is undefined, so stdlib/%s reached no runtime", test.name, test.area)
			}
		})
	}
}

// TestRegisterInstallsTheThrowableClasses covers the classes a PHP library
// throws. They are not a binding area: registerExceptions runs on every mount,
// whatever the profile narrowed, because a script that cannot construct
// InvalidArgumentException fails on the throw and never on the condition.
func TestRegisterInstallsTheThrowableClasses(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Mount(rt, stdlib.Secure)

	for _, class := range []string{
		"Exception",
		"RuntimeException",
		"InvalidArgumentException",
		"JsonException",
		"Error",
		"TypeError",
		"DivisionByZeroError",
	} {
		if _, ok := rt.LookupConstructor(class); !ok {
			t.Errorf("new %s() is undefined under the secure profile", class)
		}
	}
}
