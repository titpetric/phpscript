package core

import (
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the putenv/getenv pair to stdlib.Register.
func init() {
	runner.RegisterBinding(registerEnvironment)
}

func registerEnvironment(rt *runner.Runtime) {
	// putenv sets an environment variable from a "NAME=value" string, or unsets a bare "NAME"; it always returns true.
	rt.RegisterFunc("putenv", func(name string, values ...string) bool {
		if len(values) > 0 {
			rt.Env[name] = values[0]
			return true
		}
		if key, value, ok := strings.Cut(name, "="); ok {
			rt.Env[key] = value
		} else {
			delete(rt.Env, name)
		}
		return true
	})
	// getenv returns the value of the environment variable $name, or false when it is not set.
	rt.RegisterFunc("getenv", func(name string) any {
		value, ok := rt.Env[name]
		if !ok {
			return false
		}
		return value
	})
}
