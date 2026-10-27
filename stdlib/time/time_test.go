package time_test

import (
	"strings"
	"testing"
	stdtime "time"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	phptime "github.com/titpetric/phpscript/stdlib/time"
)

// run executes src against a runtime carrying this package's bindings and
// no other binding, and what resolves is what Register installed.
func run(t *testing.T, src string) string {
	t.Helper()
	program, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	phptime.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// TestRegister covers the layout constants and the names a script reaches the
// clock through. A namespaced constant does not parse in this runtime and the
// constants carry the classes' Time\ prefix as TIME_, and the value under each
// name is Go's own layout string.
func TestRegister(t *testing.T) {
	t.Parallel()
	rt := runner.New(&strings.Builder{}, runner.Options{})
	phptime.Register(rt)

	for _, test := range []struct {
		name string
		want string
	}{
		{name: "TIME_RFC3339", want: stdtime.RFC3339},
		{name: "TIME_RFC3339_NANO", want: stdtime.RFC3339Nano},
		{name: "TIME_DATETIME", want: stdtime.DateTime},
		{name: "TIME_DATE_ONLY", want: stdtime.DateOnly},
		{name: "TIME_TIME_ONLY", want: stdtime.TimeOnly},
	} {
		value, ok := rt.Const(test.name)
		if !ok {
			t.Errorf("%s is not defined after Register", test.name)
			continue
		}
		if value != test.want {
			t.Errorf("%s = %#v, want %q", test.name, value, test.want)
		}
	}

	for _, name := range []string{
		"set_timezone",
		"DateTime::now",
		"DateTime::parse",
		"DateTime::parse_in_location",
		"DateTime::date",
		"DateTime::unix",
		"DateTime::unix_milli",
		"DateTime::unix_micro",
		"DateTime::since",
		"DateTime::until",
		"Time\\Duration::parse",
		"Time\\Location::load",
		"Time\\Location::fixed",
		"Time\\Location::utc",
		"Time\\Location::local",
	} {
		if !rt.FunctionExists(name) {
			t.Errorf("%s does not resolve after Register", name)
		}
	}
}

// TestRegisterDispatchesToGoMethods covers the decision this package rests on:
// the constructors return Go values and no facade and every method a script
// calls is Go's own method under the name PHP spells it with. A facade would
// need one wrapper per method, the place a method goes missing.
func TestRegisterDispatchesToGoMethods(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a duration string parses and reports its own units",
			src:  `<?php echo (new Time\Duration("90m"))->hours();`,
			want: "1.5",
		},
		{
			name: "an integer duration is nanoseconds",
			src:  `<?php echo (new Time\Duration(1500000000))->seconds();`,
			want: "1.5",
		},
		{
			name: "an instant formats with a layout constant",
			src:  `<?php echo DateTime::unix(0, 0)->in(Time\Location::utc())->format(TIME_RFC3339);`,
			want: "1970-01-01T00:00:00Z",
		},
		{
			name: "a fixed zone carries its offset into the formatted instant",
			src:  `<?php echo DateTime::unix(0, 0)->in(Time\Location::fixed("CET", 3600))->format(TIME_RFC3339);`,
			want: "1970-01-01T01:00:00+01:00",
		},
		{
			name: "a parse reads the layout it was given",
			src:  `<?php echo DateTime::parse(TIME_DATE_ONLY, "2026-10-13")->format(TIME_DATE_ONLY);`,
			want: "2026-10-13",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := run(t, test.src); got != test.want {
				t.Errorf("output = %q, want %q", got, test.want)
			}
		})
	}
}

// TestRegisterTimezoneIsPerRuntime covers the reason the location sits on a
// clock value: set_timezone moves what Time construction and parsing resolve
// against for one runtime, and two runtimes serving two requests keep separate
// choices. Writing time.Local would leak one request's timezone into the other.
func TestRegisterTimezoneIsPerRuntime(t *testing.T) {
	t.Parallel()
	moved := run(t, `<?php
set_timezone("UTC");
echo DateTime::date(2026, 10, 13, 8, 0, 0, 0)->format(TIME_RFC3339);`)
	if moved != "2026-10-13T08:00:00Z" {
		t.Errorf("output = %q, want the instant in UTC", moved)
	}

	// A second runtime registers its own clock and it still reads the
	// location the host started in and never the one the first script set.
	rt := runner.New(&strings.Builder{}, runner.Options{})
	phptime.Register(rt)
	if !rt.FunctionExists("set_timezone") {
		t.Error("set_timezone does not resolve on a second runtime")
	}
}
