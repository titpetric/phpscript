package core_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestCoreAreasRegisterThemselves covers the wiring the package documents:
// every file contributes an area from its `init()`; adding, moving or dropping
// an area needs no edit to another file. Importing the package installs the
// areas, as a program imports a database/sql driver.
//
// Each file names its area with one name. A file whose `init()` stopped
// running, or whose registration moved, takes its name with it, failing here.
func TestCoreAreasRegisterThemselves(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	for file, name := range map[string]string{
		"arrays.go":          "array_merge",
		"arrays_sets.go":     "array_fill",
		"arrays_sort.go":     "ksort",
		"closure.go":         "Closure::fromCallable",
		"defer.go":           "defer",
		"encoding.go":        "base64_encode",
		"env.go":             "getenv",
		"fnmatch.go":         "fnmatch",
		"json.go":            "json_encode",
		"lang.go":            "gettype",
		"limits.go":          "set_time_limit",
		"log.go":             "error_log",
		"math.go":            "number_format",
		"mbstring.go":        "mb_strlen",
		"output.go":          "var_dump",
		"platform.go":        "phpversion",
		"reflect.go":         "get_class",
		"shutdown.go":        "register_shutdown_function",
		"sleep.go":           "usleep",
		"sprintf.go":         "sprintf",
		"strings.go":         "str_replace",
		"strings_compare.go": "strcmp",
		"tokenizer.go":       "token_name",
		"yaml.go":            "yaml_encode",
	} {
		if !rt.FunctionExists(name) {
			t.Errorf("%s() is not registered, so the init() in stdlib/ps/core/%s contributed nothing", name, file)
		}
	}

	// shared_memory.go registers a class and no function.
	if ok, err := rt.ClassExists("SharedMemory", false); err != nil || !ok {
		t.Errorf("SharedMemory is not declared, so the init() in stdlib/ps/core/shared_memory.go contributed nothing")
	}
}
