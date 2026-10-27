package runner

import (
	"context"
	"sort"
	"strconv"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/mapmap"
)

// This file binds a request onto a runtime as PHP's superglobals: Register
// seeds the lazy views, autoGlobal builds each one on the first read that
// names it, and the renderers below spell what the views hold. context.go and
// context_body.go decode what they read.

// Register seeds the request superglobals and puts the request itself on the
// runtime context. After this, PHP reads $_GET, $_POST and
// $_REQUEST from this Context, and the request-aware functions find it.
//
// stdlib/http registers the functions themselves - getallheaders(), header(),
// http_response_code(), setcookie() - and reaches this Context through
// RequestContext at call time. That registration runs before this does,
// because a host installs the standard library first and seeds the request
// second; the indirection is what makes that order work either way.
func (c Context) Register(rt *Runtime) {
	// The request has crossed into the runtime: fold its host-side size into
	// the memory baseline for the rest of this request's lifetime.
	rt.AccountRequest(c)
	// Make request cookies and staged response headers available to bindings
	// whose methods receive the runtime lifecycle context.
	rt.SetContext(context.WithValue(rt.Context(), requestContextKey{}, c))

	// Superglobals bind on their first read. Every superglobal is a view over
	// what the request holds, and a view decodes what it needs on the first
	// read; binding them the same way means a page that names none of them
	// builds none of the mapmap layers either, which is php's own
	// auto_globals_jit. Runtime.resolveAuto is the read that binds.
	//
	// The layers of the request this one replaces go back to the pool. A
	// runtime serves one request at a time and the moment the next registers
	// is the moment the last one lets go of them.
	releaseMapMaps(rt, superglobalNames...)
	for _, name := range superglobalNames {
		rt.auto.Set(name, nil)
	}
	rt.lazyGlobals = c.autoGlobal

	argvArr := rt.scratchArray("argv")
	for i, arg := range c.Argv {
		argvArr.Set(int64(i), arg)
	}
	rt.SetGlobal("argv", argvArr)
	rt.SetGlobal("argc", int64(len(c.Argv)))

	// What the request got wrong is the runtime's to report, not the script's
	// to catch: it happened before the script started and a try/catch has no
	// error to wrap. See Errors.
	for _, err := range c.Errors() {
		rt.RecordError(err)
	}
}

// releaseMapMaps returns the edit layers of the named globals, if they are
// views and if anything wrote to them.
func releaseMapMaps(rt *Runtime, names ...string) {
	for _, name := range names {
		if previous, ok := rt.auto.Lookup(name); ok {
			if m, ok := previous.(*mapmap.MapMap); ok {
				m.Release()
			}
		}
	}
}

// superglobalNames are the request variables Register seeds, which are the
// views whose layers go back to the pool when the next request arrives.
var superglobalNames = []string{"_GET", "_POST", "_COOKIE", "_SERVER", "_ENV", "_REQUEST", "_FILES"}

// autoGlobal builds the view for one superglobal, on the first read that
// names it. The names are the closed set AutoGlobals claims; _SESSION is
// Session\Manager's and no request input and it builds nothing here.
func (c Context) autoGlobal(name string) any {
	switch name {
	case "_GET":
		return c.lazyInput(c.getVars, c.GetMap)
	case "_POST":
		return c.lazyInput(c.postVars, c.PostMap)
	case "_COOKIE":
		return c.lazyInput(c.cookieVars, c.CookieMap)
	case "_SERVER":
		return c.ServerMap()
	case "_ENV":
		return mapmap.New(mapmap.MapSource(c.Env))
	case "_REQUEST":
		return c.lazyArray(func() *model.Array {
			return c.requestArrayInto(model.NewArray())
		})
	case "_FILES":
		return c.lazyArray(func() *model.Array {
			c.decodeForm()
			return c.filesArrayInto(model.NewArray())
		})
	}
	return nil
}

// lazyArray renders a superglobal that build assembles and nothing decodes, as
// $_REQUEST is from the four inputs it merges and $_FILES from the body.
func (c Context) lazyArray(build func() *model.Array) *mapmap.MapMap {
	if c.decode == nil {
		return mapmap.New(arraySource{build()})
	}
	return mapmap.New(mapmap.Lazy(func() mapmap.Source { return arraySource{build()} }))
}

// lazyInput renders one decoded input as a view whose source is the decode
// itself and naming the variable is what parses it.
//
// decoded answers the bracket-decoded array once it exists, and flat the map a
// host set. A Context with nothing to decode reads flat directly, which is
// every CLI run.
func (c Context) lazyInput(decoded func() *model.Array, flat func() map[string]string) *mapmap.MapMap {
	if c.decode == nil {
		return mapmap.New(arraySource{superglobal(decoded(), flat())})
	}
	return mapmap.New(mapmap.Lazy(func() mapmap.Source {
		return arraySource{superglobal(decoded(), flat())}
	}))
}

// arraySource reads a *model.Array as a mapmap source and a read reaches a
// decoded input where it lies, with no copy into a second shape.
type arraySource struct{ array *model.Array }

// Get answers the entry under key.
func (s arraySource) Get(key string) (any, bool) {
	if s.array == nil {
		return nil, false
	}
	return s.array.Get(normalizeKey(key))
}

// Len counts the entries.
func (s arraySource) Len() int {
	if s.array == nil {
		return 0
	}
	return s.array.Len()
}

// Range visits the entries in the order decoded.
func (s arraySource) Range(fn func(key string, value any) bool) bool {
	if s.array == nil {
		return true
	}
	done := true
	s.array.Range(func(key, val any) bool {
		done = fn(phpval.String(key), val)
		return done
	})
	return done
}

// ServerMap renders $_SERVER as a view over the request, with no array built.
//
// A read goes to the source, which is the request where there is one and a
// script that reads a key or two copies nothing and one that reads none costs
// no time at all. ServerMap seeds the two times because PHP answers them as
// numbers and every other name as a string.
func (c Context) ServerMap() *mapmap.MapMap {
	src := c.ServerSource
	if src == nil {
		src = mapmap.MapSource(c.Server)
	}
	m := mapmap.New(src)
	if seconds, err := strconv.ParseInt(c.Server["REQUEST_TIME"], 10, 64); err == nil {
		m.Write("REQUEST_TIME", seconds)
	}
	if exact, err := strconv.ParseFloat(c.Server["REQUEST_TIME_FLOAT"], 64); err == nil {
		m.Write("REQUEST_TIME_FLOAT", exact)
	}
	return m
}

// serverArrayInto renders $_SERVER. Context.Server holds every value as a
// string because that is what all but two of PHP's server keys are; the two
// that are not, REQUEST_TIME as an integer and REQUEST_TIME_FLOAT as a float,
// get their type back here and a script comparing either with === sees what
// PHP writes.
func (c Context) serverArrayInto(arr *model.Array) *model.Array {
	mapToArrayInto(arr, c.Server)
	if seconds, err := strconv.ParseInt(c.Server["REQUEST_TIME"], 10, 64); err == nil {
		arr.Set("REQUEST_TIME", seconds)
	}
	if exact, err := strconv.ParseFloat(c.Server["REQUEST_TIME_FLOAT"], 64); err == nil {
		arr.Set("REQUEST_TIME_FLOAT", exact)
	}
	return arr
}

// requestArrayInto renders $_REQUEST: the query, form and cookie fields merged in
// PHP's default request order, with the route's path values written over them
// last. The path values are the one deviation from PHP, whose $_REQUEST
// carries no route parameters: a path parameter is request input here, and
// it carries under PHP's name and nothing here spells a name PHP does not
// have. See the predefined-variables reference.
//
// Every entry is a copy. $_REQUEST is its own array in PHP and a write to it
// must not reach $_GET, $_POST or $_COOKIE, nor theirs reach it.
func (c Context) requestArrayInto(arr *model.Array) *model.Array {
	// Through the accessors, because $_REQUEST is the four merged and reading
	// it means a read of each: the fields hold nothing until a decode fills
	// them.
	for _, source := range []*model.Array{
		superglobal(c.getVars(), c.GetMap()),
		superglobal(c.postVars(), c.PostMap()),
		superglobal(c.cookieVars(), c.CookieMap()),
		mapToArray(c.Path),
	} {
		source.Range(func(key, val any) bool {
			arr.Set(key, model.CopyValue(val))
			return true
		})
	}
	return arr
}

// superglobal prefers the decoded record, and falls back to the flat map for a
// Context assembled by hand: a CLI run, a scheduled job, the fixture harness,
// or an embedder filling c.Get itself. No request arrives to decode.
func superglobal(vars *model.Array, flat map[string]string) *model.Array {
	if vars != nil {
		return vars
	}
	return mapToArray(flat)
}

// mapToArray converts a string map into a PHP associative array with stable,
// alphabetical key order (Go map iteration is random; PHP callers expect a
// deterministic shape).
func mapToArray(m map[string]string) *model.Array {
	if len(m) == 0 {
		return model.NewArray()
	}
	return mapToArrayInto(model.NewArraySize(len(m)), m)
}

// mapToArrayInto is mapToArray filling a caller-owned container, which a
// per-request superglobal reuses across requests.
func mapToArrayInto(arr *model.Array, m map[string]string) *model.Array {
	if len(m) == 0 {
		return arr
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		arr.Set(k, m[k])
	}
	return arr
}
