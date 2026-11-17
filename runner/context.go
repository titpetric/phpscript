package runner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner/mapmap"
	"github.com/titpetric/phpscript/stdlib/shared"
)

// Context holds HTTP request data exposed to PHP as superglobals, header
// functions and staged response headers.
type Context struct {
	Get     map[string]string
	Post    map[string]string
	Path    map[string]string
	Cookie  map[string]string
	Server  map[string]string
	Env     map[string]string
	Headers map[string]string
	Argv    []string

	// ServerSource is what $_SERVER reads through when the source can derive a
	// name without a map. A request answers most of them itself and
	// serving one fills Server with the few it cannot and leaves the rest for
	// the first read that reaches them to derive.
	//
	// Nil reads Server alone, as a run with no request behind it
	// has. Server still answers the keys a host sets either way: they are the
	// source's extras, and an extra wins over a derived name.
	ServerSource mapmap.Source

	// GetVars, PostVars and CookieVars are the same fields decoded through
	// PHP's bracket syntax, and seed $_GET, $_POST and $_COOKIE.
	//
	// The flat maps above are not the same information. They hold one string
	// per literal field name, the shape a Go host reads and what
	// Session\Manager looks a cookie up in. They cannot express what the
	// decoder needs: `a[]=1&a[]=2` is one repeated name and a map keeps only
	// the second value.
	//
	// Nil for a Context built from no request; Register falls back
	// to the flat map and Register still reads a c.Get a host filled itself.
	GetVars    *model.Array
	PostVars   *model.Array
	CookieVars *model.Array

	// Files holds the file parts of a multipart body keyed by form field name,
	// in the order sent. A field holds more than one file when the
	// form repeats it, which HTML spells as a "name[]" field.
	Files map[string][]*UploadedFile

	// response collects headers set by the PHP header() function. The field is
	// an http.Header (a map, hence reference-shared across copies of Context) so
	// a host handler can flush it onto the response after execution.
	response http.Header
	status   *int
	// errors holds what went wrong with the request itself, before any script
	// ran: a body refused for its size. A pointer for the same reason status
	// is and a copy of a Context reports what another copy recorded.
	errors *[]error

	// rawBody holds the request body bytes php://input answers with. A pointer
	// for the same reason status is and a copy of a Context reads
	// what another copy buffered.
	rawBody *[]byte

	// decode holds the request and what decoding has pulled out of it so far. A
	// Context built by a host leaves it nil: no bytes wait to decode, and the
	// fields above are what it holds. A pointer for the same reason status is,
	// and a decode through one copy shows in every other.
	decode *decoder
}

// decoder defers reading a request until a script reads what it holds.
type decoder struct {
	request *http.Request
	options Options

	query   sync.Once
	body    sync.Once
	cookies sync.Once

	// The decoded forms, held here and not on the Context because a
	// Context is a value: a method that assigned to its field would be
	// assigning to a copy. The exported fields stay what a host sets, and
	// win over these.
	getVars    *model.Array
	postVars   *model.Array
	cookieVars *model.Array

	// The flat forms, allocated by the decode that fills them. A request that
	// reads no query allocates no query map and they are not in
	// NewContext with the rest.
	get     map[string]string
	post    map[string]string
	cookie  map[string]string
	headers map[string]string
	files   map[string][]*UploadedFile

	headerOnce sync.Once
}

type requestContextKey struct{}

// NewContext returns an allocated empty Context value, the shape a host
// assembling a request by hand writes into: a CLI run, a scheduled job, a test.
func NewContext() Context {
	c := newContext()
	c.Get = map[string]string{}
	c.Post = map[string]string{}
	c.Cookie = map[string]string{}
	c.Headers = map[string]string{}
	c.Files = map[string][]*UploadedFile{}
	return c
}

// newContext allocates the fields every Context has, without the decode maps.
func newContext() Context {
	status := 0
	var errs []error
	return Context{
		errors:   &errs,
		rawBody:  new([]byte),
		Path:     map[string]string{},
		Server:   map[string]string{},
		Env:      map[string]string{},
		Argv:     []string{},
		response: http.Header{},
		status:   &status,
	}
}

// routePatternKey names the context value holding the route path as declared.
type routePatternKey struct{}

// WithRoutePattern records the @route path a request matched, unrewritten.
func WithRoutePattern(ctx context.Context, pattern string) context.Context {
	return context.WithValue(ctx, routePatternKey{}, pattern)
}

// routePattern returns the declared pattern, falling back to the router's match.
func routePattern(r *http.Request) string {
	if pattern, ok := r.Context().Value(routePatternKey{}).(string); ok && pattern != "" {
		return pattern
	}
	// ServeMux prefixes the method; chi does not.
	if _, path, ok := strings.Cut(r.Pattern, " "); ok {
		return path
	}
	return r.Pattern
}

// FromRequest builds a Context with no size limit on the body.
func FromRequest(r *http.Request) Context {
	return FromRequestOptions(r, Options{})
}

// FromRequestOptions builds a Context from an HTTP request, path values out of
// the matched ServeMux pattern.
//
// A decode records query, form and cookie fields twice: flat for a Go host,
// decoded for the superglobals. Both keep the last value when a name repeats,
// as PHP's parser does.
func FromRequestOptions(r *http.Request, opts Options) Context {
	c := newContext()
	c.decode = &decoder{request: r, options: opts}

	// The query, the body and the cookie header are not read here. The first
	// read that needs one decodes it and a page that names no superglobal
	// parses nothing; see decoder.
	//
	// This check refuses a body over post_max_size here and not later, because
	// PHP says so in its log whether the script reads $_POST or not, and
	// comparing the announced length costs no read.
	if opts.PostMaxSize.Exceeds(r.ContentLength) {
		c.recordError(fmt.Errorf("POST Content-Length of %d bytes exceeds the post_max_size limit of %d bytes",
			r.ContentLength, opts.PostMaxSize.Bytes()))
		c.decode.body.Do(func() {})
	}

	// Server variables ($_SERVER). The request is the source and Server holds
	// the extras and a read goes to both and copies neither.
	c.serverVars(r)
	c.ServerSource = &mapmap.RequestSource{Request: r, Extra: mapmap.MapSource(c.Server)}

	// Path values from the matched route, merged into $_REQUEST.
	c.pathValues(r)

	// The headers are not copied into a map here either. HeaderMap builds one
	// on the first read, and mapmap.RequestSource derives the HTTP_ spelling
	// of each from the request.

	return c
}

// GetMap, PostMap, CookieMap and FileMap answer the flat form of each input,
// decoding the request on the first call.
//
// The fields behind them are what a host sets; these are what anything reading
// a request built from an *http.Request resolves through, because the field
// holds nothing until one of them runs.

// GetMap answers the query string, one value per name.
func (c Context) GetMap() map[string]string {
	c.decodeQuery()
	if c.Get != nil || c.decode == nil {
		return c.Get
	}
	return c.decode.get
}

// PostMap answers the form body, one value per name.
func (c Context) PostMap() map[string]string {
	c.decodeForm()
	if c.Post != nil || c.decode == nil {
		return c.Post
	}
	return c.decode.post
}

// CookieMap answers the request's cookies, one value per name. Session\Manager
// reads a session id through it.
func (c Context) CookieMap() map[string]string {
	c.decodeCookies()
	if c.Cookie != nil || c.decode == nil {
		return c.Cookie
	}
	return c.decode.cookie
}

// FileMap answers the file parts of a multipart body, keyed by field name, in
// the order sent.
func (c Context) FileMap() map[string][]*UploadedFile {
	c.decodeForm()
	if c.Files != nil || c.decode == nil {
		return c.Files
	}
	return c.decode.files
}

// postInto returns the map parseBody fills, keeping a field the host set.
func (c Context) postInto() map[string]string {
	if c.Post != nil || c.decode == nil {
		if c.Post == nil {
			c.Post = map[string]string{}
		}
		return c.Post
	}
	if c.decode.post == nil {
		c.decode.post = map[string]string{}
	}
	return c.decode.post
}

func (c Context) filesInto() map[string][]*UploadedFile {
	if c.Files != nil || c.decode == nil {
		if c.Files == nil {
			c.Files = map[string][]*UploadedFile{}
		}
		return c.Files
	}
	if c.decode.files == nil {
		c.decode.files = map[string][]*UploadedFile{}
	}
	return c.decode.files
}

// files returns what decoding has filled so far, forcing no parse.
func (c Context) files() map[string][]*UploadedFile {
	if c.Files != nil || c.decode == nil {
		return c.Files
	}
	return c.decode.files
}

// HeaderMap answers the request headers by canonical name, building the map on
// the first call. A request sends a dozen and most pages read none.
func (c Context) HeaderMap() map[string]string {
	if c.Headers != nil || c.decode == nil {
		return c.Headers
	}
	c.decode.headerOnce.Do(func() {
		r := c.decode.request
		c.decode.headers = make(map[string]string, len(r.Header))
		for name, values := range r.Header {
			if len(values) > 0 {
				c.decode.headers[name] = values[0]
			}
		}
	})
	return c.decode.headers
}

// getVars returns the decoded $_GET array, keeping the field a host set.
func (c Context) getVars() *model.Array {
	if c.GetVars != nil {
		return c.GetVars
	}
	c.decodeQuery()
	if c.decode == nil {
		return nil
	}
	return c.decode.getVars
}

func (c Context) postVars() *model.Array {
	if c.PostVars != nil {
		return c.PostVars
	}
	c.decodeForm()
	if c.decode == nil {
		return nil
	}
	return c.decode.postVars
}

func (c Context) cookieVars() *model.Array {
	if c.CookieVars != nil {
		return c.CookieVars
	}
	c.decodeCookies()
	if c.decode == nil {
		return nil
	}
	return c.decode.cookieVars
}

// decodeQuery fills $_GET on the first call, reading RawQuery to keep field order.
func (c Context) decodeQuery() {
	if c.decode == nil {
		return
	}
	c.decode.query.Do(func() {
		r := c.decode.request
		query := r.URL.Query()
		c.decode.get = make(map[string]string, len(query))
		for k, v := range query {
			if len(v) > 0 {
				c.decode.get[k] = v[len(v)-1]
			}
		}
		c.decode.getVars = shared.ParseStr(r.URL.RawQuery, c.decode.options.inputLimits())
	})
}

// decodeCookies fills $_COOKIE on the first call, percent-decoding as PHP does.
func (c Context) decodeCookies() {
	if c.decode == nil {
		return
	}
	c.decode.cookies.Do(func() {
		cookies := c.decode.request.Cookies()
		pairs := make([]shared.Pair, 0, len(cookies))
		c.decode.cookie = make(map[string]string, len(cookies))
		for _, ck := range cookies {
			c.decode.cookie[ck.Name] = ck.Value
			pairs = append(pairs, shared.Pair{
				Name:  shared.URLDecode(ck.Name),
				Value: shared.URLDecode(ck.Value),
			})
		}
		c.decode.cookieVars = shared.ParsePairs(c.decode.options.inputLimits(), pairs)
	})
}

// decodeForm fills $_POST, $_FILES and php://input from the one body read.
func (c Context) decodeForm() {
	if c.decode == nil {
		return
	}
	c.decode.body.Do(func() {
		c.parseBody(c.decode.request, c.decode.options)
	})
}

// pathValues fills c.Path from the pattern's declared names, reading a rest parameter under "*".
func (c Context) pathValues(r *http.Request) {
	params, err := modelutil.ParseRoutePath(routePattern(r))
	if err != nil {
		// The linter reports a path the grammar refuses and registration skips
		// it and reaching here means a host registered it directly.
		return
	}
	for _, param := range params {
		value := r.PathValue(param.Name)
		if value == "" && param.Rest {
			value = r.PathValue("*")
		}
		if value != "" {
			c.Path[param.Name] = value
		}
	}
}

// serverVars fills the $_SERVER keys the request answers, leaving the SAPI keys to the host.
func (c Context) serverVars(r *http.Request) {
	// REQUEST_METHOD, REQUEST_URI, QUERY_STRING, HTTP_HOST and SERVER_PROTOCOL
	// are not written here. The request answers all five, and mapmap derives
	// them on the read that asks and a script that never names one costs
	// no time for it. What follows is what the request cannot answer alone.

	// PHP splits the peer address in two, the address in REMOTE_ADDR and the
	// port in REMOTE_PORT, where Go keeps both in RemoteAddr as "address:port".
	// An IPv6 address arrives bracketed and leaves without, as PHP writes it.
	// A Go host is free to put anything in the field and a value that does not
	// parse lands in REMOTE_ADDR whole and REMOTE_PORT stays unset.
	if addr, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		c.Server["REMOTE_ADDR"] = addr
		c.Server["REMOTE_PORT"] = port
	} else if r.RemoteAddr != "" {
		c.Server["REMOTE_ADDR"] = r.RemoteAddr
	}

	// The scheme is the one this process is serving, r.TLS, and not the one
	// X-Forwarded-Proto claims: that header is the client's to send, and a
	// script deciding from it whether it talks over TLS would be deciding on
	// what the client said. A host behind a proxy that terminates TLS is the
	// one that knows whether to trust the proxy, and sets the two keys itself.
	// HTTPS is unset on a plain request, never "off" and an
	// isset($_SERVER["HTTPS"]) test works in PHP.
	c.Server["REQUEST_SCHEME"] = "http"
	if r.TLS != nil {
		c.Server["REQUEST_SCHEME"] = "https"
		c.Server["HTTPS"] = "on"
	}

	// CONTENT_TYPE and CONTENT_LENGTH mirror the two headers, and appear only
	// when the request sent them. PHP sets CONTENT_TYPE for any request
	// with the header, a bodyless GET included.
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		c.Server["CONTENT_TYPE"] = contentType
	}
	if length, ok := contentLength(r); ok {
		c.Server["CONTENT_LENGTH"] = length
	}

	// REQUEST_TIME_FLOAT is when the request started, to the microsecond, and
	// REQUEST_TIME the whole second of it. One clock read fills both, here and
	// not per key and the two cannot disagree about which second the request
	// began in.
	start := time.Now()
	c.Server["REQUEST_TIME"] = strconv.FormatInt(start.Unix(), 10)
	c.Server["REQUEST_TIME_FLOAT"] = strconv.FormatFloat(float64(start.UnixMicro())/1e6, 'f', 6, 64)
}

// contentLength renders CONTENT_LENGTH and reports whether the request announced a length.
func contentLength(r *http.Request) (string, bool) {
	if r.ContentLength < 0 {
		return "", false
	}
	if _, sent := r.Header["Content-Length"]; !sent && r.ContentLength == 0 {
		return "", false
	}
	return strconv.FormatInt(r.ContentLength, 10), true
}

// recordError notes something the request itself got wrong. See Errors.
func (c Context) recordError(err error) {
	if c.errors == nil || err == nil {
		return
	}
	*c.errors = append(*c.errors, err)
}

// Errors returns what the request got wrong before any script ran, which no
// script can catch: a body or a file part refused for its size.
func (c Context) Errors() []error {
	if c.errors == nil {
		return nil
	}
	return *c.errors
}

// memoryFootprint estimates the host-side bytes this Context holds, excluding runtime copies.
func (c Context) memoryFootprint(visited visitedSet) int64 {
	total := int64(unsafe.Sizeof(c))
	// What decoding has filled in, not what it could: forcing a parse to
	// measure one would be the accounting doing the work it measures.
	for _, m := range c.decodedMaps() {
		total += DeepSize(m, visited)
	}
	total += DeepSize(c.Argv, visited)
	total += DeepSize(map[string][]string(c.response), visited)
	for name, files := range c.files() {
		total += 16 + int64(len(name)) + 24
		for _, file := range files {
			total += DeepSize(file, visited)
		}
	}
	return total
}

// decodedMaps lists the string maps this Context holds right now.
func (c Context) decodedMaps() []map[string]string {
	maps := []map[string]string{c.Path, c.Server, c.Env}
	if c.decode == nil {
		return append(maps, c.Get, c.Post, c.Cookie, c.Headers)
	}
	return append(maps, c.decode.get, c.decode.post, c.decode.cookie, c.decode.headers)
}

// GetAllHeaders is getallheaders(): the request headers, canonically keyed.
func (c Context) GetAllHeaders() *model.Array {
	return mapToArray(c.HeaderMap())
}

// RequestContext returns the request data registered on a runtime context.
// Request-aware Go bindings such as session management read through it.
func RequestContext(ctx context.Context) (Context, bool) {
	c, ok := ctx.Value(requestContextKey{}).(Context)
	return c, ok
}
