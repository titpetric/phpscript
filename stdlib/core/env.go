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
	// putenv sets an environment variable from a "NAME=value" string, or
	// unsets a bare "NAME"; it always returns true. The change lasts for the
	// running script: the next request starts on the environment the host
	// configured.
	rt.RegisterFunc("putenv", func(name string, values ...string) bool {
		if len(values) > 0 {
			rt.PutEnv(name, values[0])
			return true
		}
		if key, value, ok := strings.Cut(name, "="); ok {
			rt.PutEnv(key, value)
		} else {
			rt.UnsetEnv(name)
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
