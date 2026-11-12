package http

import (
	nethttp "net/http"

	"github.com/titpetric/phpscript/runner"
	pshttp "github.com/titpetric/phpscript/stdlib/ps/http"
)

// Register installs the net/http area: the client names, the server types and
// Aborted.
func Register(rt *runner.Runtime) {
	Set.Register(rt)
	// The three file-serving names resolve through stdlib/ps/http, which clamps
	// a path to the source filesystem. net/http's own ServeFile reads the host
	// filesystem, and no binding of this runtime does.
	pshttp.RegisterGoNames(rt, Namespace)
	// Runtime\Go\Net\HTTP\ServeMux routes requests to PHP callables with net/http's patterns.
	rt.RegisterConstructor(Namespace+`\ServeMux`, func() *pshttp.Mux { return pshttp.NewMux(rt) })
	// Runtime\Go\Net\HTTP\Mux is ServeMux under the name phpscript spells it.
	rt.RegisterConstructor(Namespace+`\Mux`, func() *pshttp.Mux { return pshttp.NewMux(rt) })
	// Runtime\Go\Net\HTTP\Server listens on $addr and answers through $handler, with $workers and a queue depth.
	rt.RegisterConstructor(Namespace+`\Server`, func(addr string, handler any, workers ...int64) (*pshttp.Server, error) {
		return pshttp.NewServer(rt, addr, handler, workers...)
	})
	// Runtime\Go\Net\HTTP\Aborted reports whether the client of $r has gone.
	rt.RegisterFunc(Namespace+`\Aborted`, aborted)
	// Runtime\Go\Net\HTTP\IgnoreUserAbort keeps the handler running when the client leaves.
	rt.RegisterFunc(Namespace+`\IgnoreUserAbort`, func() { rt.SetIgnoreUserAbort(true) })
}

// aborted reports whether r has no client left to answer.
func aborted(r *nethttp.Request) bool {
	return r != nil && r.Context().Err() != nil
}
