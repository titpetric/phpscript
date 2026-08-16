package runner

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// Context carries HTTP request data exposed to PHP as superglobals, header
// functions, and staged response headers.
type Context struct {
	Get     map[string]string
	Post    map[string]string
	Path    map[string]string
	Cookie  map[string]string
	Server  map[string]string
	Env     map[string]string
	Headers map[string]string
	Argv    []string

	// response collects headers set by the PHP header() function. It is an
	// http.Header (a map, hence reference-shared across copies of Context) so a
	// host handler can flush it onto the response after execution.
	response http.Header
	status   *int
}

type requestContextKey struct{}

// NewContext returns an allocated empty Context value.
func NewContext() Context {
	status := 0
	return Context{
		Get:      map[string]string{},
		Post:     map[string]string{},
		Path:     map[string]string{},
		Cookie:   map[string]string{},
		Server:   map[string]string{},
		Env:      map[string]string{},
		Headers:  map[string]string{},
		Argv:     []string{},
		response: http.Header{},
		status:   &status,
	}
}

// pathVarRE matches Go 1.22+ ServeMux wildcard segments, e.g. {id} or {rest...}.
var pathVarRE = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)(\.\.\.)?\}`)

// FromRequest builds a Context from an HTTP request. Query and form values are
// flattened to their first value (PHP's scalar superglobal shape); path values
// are pulled out of the matched ServeMux pattern via r.PathValue.
func FromRequest(r *http.Request) Context {
	c := NewContext()

	// Query string ($_GET).
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			c.Get[k] = v[0]
		}
	}

	// Form body ($_POST). ParseForm is idempotent and a no-op for bodyless
	// requests, so calling it unconditionally is safe.
	_ = r.ParseForm()
	for k, v := range r.PostForm {
		if len(v) > 0 {
			c.Post[k] = v[0]
		}
	}

	// Cookies ($_COOKIE).
	for _, ck := range r.Cookies() {
		c.Cookie[ck.Name] = ck.Value
	}

	// Server variables ($_SERVER).
	c.Server["REQUEST_METHOD"] = r.Method
	c.Server["REQUEST_URI"] = r.URL.RequestURI()
	c.Server["QUERY_STRING"] = r.URL.RawQuery
	c.Server["HTTP_HOST"] = r.Host
	c.Server["SERVER_PROTOCOL"] = r.Proto
	c.Server["REMOTE_ADDR"] = r.RemoteAddr

	// Path values from the matched route pattern, e.g. "GET /users/{id}".
	// The stdlib exposes individual values via r.PathValue but no enumeration,
	// so we recover the wildcard names from r.Pattern and look each one up.
	for _, m := range pathVarRE.FindAllStringSubmatch(r.Pattern, -1) {
		name := m[1]
		if val := r.PathValue(name); val != "" {
			c.Path[name] = val
		}
	}

	// Request headers (canonical name -> first value).
	for k, v := range r.Header {
		if len(v) > 0 {
			c.Headers[k] = v[0]
			key := "HTTP_" + strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
			c.Server[key] = v[0]
		}
	}

	return c
}

// Register installs the request-aware PHP functions onto rt and seeds the
// request superglobals. After this, transpiled PHP can call getallheaders() /
// header() and read $_GET, $_POST, $_PATH — all backed by this Context.
func (c Context) Register(rt *Runtime) {
	// Make request cookies and staged response headers available to bindings
	// whose methods receive the runtime lifecycle context.
	rt.SetContext(context.WithValue(rt.Context(), requestContextKey{}, c))

	// PHP header inspection.
	rt.RegisterFunc("getallheaders", c.GetAllHeaders)
	rt.RegisterFunc("get_all_headers", c.GetAllHeaders) // README spelling / alias
	rt.RegisterFunc("apache_request_headers", c.GetAllHeaders)

	// PHP response header emission.
	rt.RegisterFunc("header", c.Header)

	// Superglobals as ordinary PHP arrays.
	rt.SetGlobal("_GET", mapToArray(c.Get))
	rt.SetGlobal("_POST", mapToArray(c.Post))
	rt.SetGlobal("_COOKIE", mapToArray(c.Cookie))
	rt.SetGlobal("_SERVER", mapToArray(c.Server))
	rt.SetGlobal("_ENV", mapToArray(c.Env))
	rt.SetGlobal("_PATH", mapToArray(c.Path))

	argvArr := model.NewArray()
	for i, arg := range c.Argv {
		argvArr.Set(int64(i), arg)
	}
	rt.SetGlobal("argv", argvArr)
	rt.SetGlobal("argc", int64(len(c.Argv)))
}

// GetAllHeaders implements PHP getallheaders(): an associative array of the
// incoming request headers keyed by canonical header name.
func (c Context) GetAllHeaders() *model.Array {
	return mapToArray(c.Headers)
}

// Header implements PHP header($header[, $replace[, $code]]): it parses a
// "Name: value" line and stages it on the response header set. replace controls
// whether an existing header of the same name is overwritten (default true).
func (c Context) Header(header string, opts ...any) {
	name, value, ok := strings.Cut(header, ":")
	if !ok {
		// Status-line / valueless headers (e.g. "HTTP/1.0 404 Not Found") have
		// no name:value shape; ignore for the simple model.
		return
	}
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)

	replace := true
	if len(opts) > 0 {
		replace = phpTruthy(opts[0])
	}
	if replace {
		c.response.Set(name, value)
	} else {
		c.response.Add(name, value)
	}
	if strings.EqualFold(name, "Location") && c.ResponseStatus() == 0 {
		*c.status = http.StatusFound
	}
	if len(opts) > 1 {
		switch code := opts[1].(type) {
		case int:
			*c.status = code
		case int64:
			*c.status = int(code)
		}
	}
}

// ResponseHeaders returns the headers staged by the PHP header() function so a
// host handler can copy them onto the http.ResponseWriter after execution.
func (c Context) ResponseHeaders() http.Header { return c.response }

// RequestContext returns the request data registered on a runtime context.
// It is intended for request-aware Go bindings such as session management.
func RequestContext(ctx context.Context) (Context, bool) {
	c, ok := ctx.Value(requestContextKey{}).(Context)
	return c, ok
}

// AddResponseHeader appends a response header without replacing existing
// values. It is useful for headers such as Set-Cookie that may occur more than
// once in a response.
func (c Context) AddResponseHeader(name, value string) {
	c.response.Add(name, value)
}

// ResponseStatus returns the status staged by header(), or zero when the host
// should retain its default status. A Location header defaults to 302 like PHP.
func (c Context) ResponseStatus() int {
	if c.status == nil {
		return 0
	}
	return *c.status
}

// mapToArray converts a string map into a PHP associative array with stable,
// alphabetical key order (Go map iteration is random; PHP callers expect a
// deterministic shape).
func mapToArray(m map[string]string) *model.Array {
	if len(m) == 0 {
		return model.NewArray()
	}
	arr := model.NewArraySize(len(m))
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
