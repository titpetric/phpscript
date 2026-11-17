package runner

// GoRuntime reports whether the runtime installs Runtime\Go.
func (rt *Runtime) GoRuntime() bool { return rt.opts.GoRuntime() }
