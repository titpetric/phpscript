package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/mapmap"
	"github.com/titpetric/phpscript/stdlib/shared"
)

// PHP's UPLOAD_ERR_* codes, as they appear in a $_FILES entry. The list names
// only the ones this runtime can produce; 2, 3 and 8 come from a per-form
// limit and from extensions phpscript does not have.
const (
	UploadErrOK        = 0
	UploadErrIniSize   = 1
	UploadErrNoFile    = 4
	UploadErrNoTmpDir  = 6
	UploadErrCantWrite = 7
)

// maxMultipartMemory bounds how much of a multipart body net/http keeps in
// memory before spilling parts to disk. It matches the net/http default.
const maxMultipartMemory = 32 << 20

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

	// ServerSource is what $_SERVER reads through when the source can derive a
	// name without a map. A request answers most of them itself, so
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
	// Session\Manager looks a cookie up in, and they cannot express what the
	// decoder needs: `a[]=1&a[]=2` is one repeated name, so a map keeps only
	// the second value.
	//
	// Nil for a Context built from no request; Register falls back
	// to the flat map, so Register still reads a c.Get a host filled itself.
	GetVars    *model.Array
	PostVars   *model.Array
	CookieVars *model.Array

	// Files holds the file parts of a multipart body keyed by form field name,
	// in the order sent. A field carries more than one file when the
	// form repeats it, which HTML spells as a "name[]" field.
	Files map[string][]*UploadedFile

	// response collects headers set by the PHP header() function. The field is
	// an http.Header (a map, hence reference-shared across copies of Context) so
	// a host handler can flush it onto the response after execution.
	response http.Header
	status   *int
	// errors holds what went wrong with the request itself, before any script
	// ran: a body refused for its size. A pointer for the same reason status
	// is, so a copy of a Context reports what another copy recorded.
	errors *[]error

	// rawBody holds the request body bytes php://input answers with. A pointer
	// for the same reason status is, so a copy of a Context reads
	// what another copy buffered.
	rawBody *[]byte

	// decode holds the request and what decoding has pulled out of it so far. A
	// Context built by a host has none: there is nothing to decode, and the
	// fields above are what it holds. A pointer for the same reason status is,
	// so a decode through one copy shows in every other.
	decode *decoder
}

// decoder defers reading a request until a script reads what it holds.
//
// A page reads one superglobal or none, and each of them costs a parse: the
// query through ParseStr, the cookie header pair by pair, the body through
// ParseForm and again for the bracket names. Each parse runs at most once, on
// the first read that needs it, and writes into the Context fields the eager
// version filled.
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
	// reads no query allocates no query map, so they are not in
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

// newContext allocates what every Context has. The maps a request decodes into
// are not among them: the decode that fills each one allocates it, so a page
// reading no query allocates no query map. NewContext adds them because a host
// writes into the fields directly.
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

// routePatternKey carries the route path as its author declared it, which
// RoutePattern puts on the request context.
type routePatternKey struct{}

// WithRoutePattern records the @route path a request matched, unrewritten.
func WithRoutePattern(ctx context.Context, pattern string) context.Context {
	return context.WithValue(ctx, routePatternKey{}, pattern)
}

// routePattern returns the declared pattern, falling back to the one the
// router matched. The fallback covers a host that registers routes itself.
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

// FromRequestOptions builds a Context from an HTTP request. Path values come
// out of the matched ServeMux pattern via r.PathValue.
//
// A decode records query, form and cookie fields twice: flat for a Go host,
// and decoded for the superglobals. Both keep the last value when a name
// repeats, as PHP's parser does.
//
// upload_max_filesize and post_max_size limit what the body may carry;
// max_input_vars and max_input_nesting_level bound what the decode builds
// from it.
func FromRequestOptions(r *http.Request, opts Options) Context {
	c := newContext()
	c.decode = &decoder{request: r, options: opts}

	// The query, the body and the cookie header are not read here. The first
	// read that needs one decodes it, so a page that names no superglobal
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
	// the extras, so a read goes to both and copies neither.
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

// postInto and filesInto answer the map parseBody fills, allocating it on the
// way. A host that set the field keeps it; otherwise the decoder owns it.
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

// files answers what decoding has filled in so far, forcing nothing. Cleanup
// and the memory accounting read it: neither should parse a body to do its job.
func (c Context) files() map[string][]*UploadedFile {
	if c.Files != nil || c.decode == nil {
		return c.Files
	}
	return c.decode.files
}

// HeaderMap answers the request headers by canonical name, building the map on
// the first call. A request carries a dozen and most pages read none.
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

// getVars, postVars and cookieVars answer the decoded form of each input,
// decoding it on the first call. A host that set the exported field reads that
// back, and nothing decodes.
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

// decodeQuery fills $_GET, reading the query string on the first call.
//
// RawQuery and not r.URL.Query(): a url.Values is a map, so field order is
// already gone, and the bracket decoder needs it.
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

// decodeCookies fills $_COOKIE on the first call. r.Cookies() keeps the order
// sent, and Go does not percent-decode a cookie the way PHP does, so that
// happens here.
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

// decodeForm fills $_POST, $_FILES and php://input on the first call.
//
// Nothing can read a body twice, so every path needing any part of it comes
// through here.
func (c Context) decodeForm() {
	if c.decode == nil {
		return
	}
	c.decode.body.Do(func() {
		c.parseBody(c.decode.request, c.decode.options)
	})
}

// pathValues fills c.Path from the route parameters the pattern declares,
// which $_REQUEST merges over the request fields.
//
// The names come from the declared path and not the matched one, so a
// parameter reads back under its declared name whichever router serves it.
// Both routers publish the values through r.PathValue: ServeMux natively, chi
// by calling SetPathValue for every URL param it captured.
//
// A {name...} parameter is the exception. chi has no such syntax, so
// RenderRoutePath spells it as chi's * wildcard, which chi publishes under "*".
func (c Context) pathValues(r *http.Request) {
	params, err := model.ParseRoutePath(routePattern(r))
	if err != nil {
		// The linter reports a path the grammar refuses and registration skips
		// it, so reaching here means a host registered it directly.
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

// serverVars fills $_SERVER with the part of PHP's server array that the
// request itself answers for. Every value is a string, as $_SERVER
// holds, with the two exceptions serverArray types back.
//
// The keys PHP fills from the SAPI and from resolving a URL to a file on disk
// (DOCUMENT_ROOT, SCRIPT_NAME, SCRIPT_FILENAME, PHP_SELF, PATH_INFO,
// SERVER_NAME, SERVER_PORT, SERVER_SOFTWARE) are not here: none of them follow
// from an *http.Request. A real PHP answers SERVER_NAME and SERVER_PORT from
// the socket it accepted on and not from the Host header, so a request whose
// Host names something else still reports the listening address; the host that
// owns the listener is the one that can say. Set them on Context.Server after
// building the context.
func (c Context) serverVars(r *http.Request) {
	// REQUEST_METHOD, REQUEST_URI, QUERY_STRING, HTTP_HOST and SERVER_PROTOCOL
	// are not written here. The request answers all five, and mapmap derives
	// them on the read that asks, so a script that never names one costs
	// nothing for it. What follows is what the request cannot answer alone.

	// PHP splits the peer address in two, the address in REMOTE_ADDR and the
	// port in REMOTE_PORT, where Go keeps both in RemoteAddr as "address:port".
	// An IPv6 address arrives bracketed and leaves without, as PHP writes it.
	// A Go host is free to put anything in the field, so a value that does not
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
	// HTTPS is unset on a plain request, never "off", so an
	// isset($_SERVER["HTTPS"]) test works in PHP.
	c.Server["REQUEST_SCHEME"] = "http"
	if r.TLS != nil {
		c.Server["REQUEST_SCHEME"] = "https"
		c.Server["HTTPS"] = "on"
	}

	// CONTENT_TYPE and CONTENT_LENGTH mirror the two headers, and appear only
	// when the request sent them. PHP sets CONTENT_TYPE for any request
	// carrying the header, a bodyless GET included.
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		c.Server["CONTENT_TYPE"] = contentType
	}
	if length, ok := contentLength(r); ok {
		c.Server["CONTENT_LENGTH"] = length
	}

	// REQUEST_TIME_FLOAT is when the request started, to the microsecond, and
	// REQUEST_TIME the whole second of it. One clock read fills both, here and
	// not per key, so the two cannot disagree about which second the request
	// began in.
	start := time.Now()
	c.Server["REQUEST_TIME"] = strconv.FormatInt(start.Unix(), 10)
	c.Server["REQUEST_TIME_FLOAT"] = strconv.FormatFloat(float64(start.UnixMicro())/1e6, 'f', 6, 64)
}

// contentLength renders the CONTENT_LENGTH value of a request and reports
// whether it has one. PHP takes the key from the Content-Length header, so a
// chunked body, which announces no length, has no CONTENT_LENGTH, while a body
// of zero bytes that announced itself has "0".
//
// Go parses the header into Request.ContentLength (-1 when unknown) and leaves
// the header in place, so the header says whether the client sent one. A
// request built in process, as httptest.NewRequest does, has the field without
// the header; a positive length there is a body all the same.
func contentLength(r *http.Request) (string, bool) {
	if r.ContentLength < 0 {
		return "", false
	}
	if _, sent := r.Header["Content-Length"]; !sent && r.ContentLength == 0 {
		return "", false
	}
	return strconv.FormatInt(r.ContentLength, 10), true
}

// isMultipart reports whether the request body is a multipart form, the one
// body shape ParseForm does not decode by itself.
func isMultipart(r *http.Request) bool {
	return mediaType(r) == "multipart/form-data"
}

// isURLEncoded reports whether the body is a urlencoded form. Only the two
// form types populate $_POST: a JSON or XML body is php://input and nothing
// else, as in PHP.
func isURLEncoded(r *http.Request) bool {
	return mediaType(r) == "application/x-www-form-urlencoded"
}

func mediaType(r *http.Request) string {
	parsed, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return ""
	}
	return parsed
}

// parseBody decodes the request body into $_POST and $_FILES. ParseForm only
// decodes an urlencoded body; a multipart one leaves it with an empty PostForm,
// so a form with a file input arrives empty without it. Only the content
// type says which of the two a request carries. Both calls are idempotent and a
// no-op for bodyless requests, so neither needs a method check.
func (c *Context) parseBody(r *http.Request, opts Options) {
	capBody(r, opts.PostMaxSize)

	// Buffer the raw body before any parser consumes it: these bytes are what
	// php://input answers with, for any method and any content type, which is
	// how a JSON API or a webhook receiver reads its payload. A multipart
	// body is not buffered: PHP documents php://input as
	// unavailable there, and multipart is the one body shape that carries
	// whole files.
	if !isMultipart(r) && r.Body != nil {
		buf, readErr := io.ReadAll(r.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(readErr, &tooLarge) {
			c.recordError(fmt.Errorf("POST body exceeds the post_max_size limit of %d bytes", tooLarge.Limit))
			return
		}
		if readErr != nil {
			c.recordError(fmt.Errorf("reading request body: %w", readErr))
			return
		}
		*c.rawBody = buf
		r.Body = io.NopCloser(bytes.NewReader(buf))
	}

	var err error
	if isMultipart(r) {
		if err = r.ParseMultipartForm(maxMultipartMemory); err == nil {
			c.collectUploads(r, opts.UploadMaxFilesize)
		}
	} else {
		err = r.ParseForm()
	}

	// A body that did not announce its length is only found to be too large
	// once the read passes the cap. This drops everything decoded before that
	// point, so it ends the same way an announced one does.
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		c.recordError(fmt.Errorf("POST body exceeds the post_max_size limit of %d bytes", tooLarge.Limit))
		return
	}

	for k, v := range r.PostForm {
		if len(v) > 0 {
			c.postInto()[k] = v[len(v)-1]
		}
	}
	if c.decode != nil {
		c.decode.postVars = c.decodeBody(r, opts)
		return
	}
	c.PostVars = c.decodeBody(r, opts)
}

// decodeBody builds $_POST, reading the brackets in the field names. Only the
// two form content types produce one; anything else is php://input and leaves
// $_POST empty.
//
// A urlencoded body decodes from the bytes parseBody already buffered for
// php://input, so a repeated `a[]` is two fields and not one map entry.
//
// A multipart body has no such string: net/http has parsed it into a map, so
// order between two field names has gone and decodeBody sorts the names to
// make what remains deterministic. Nesting survives. What this cannot
// reproduce is a form interleaving two top-level names and expecting the later
// to win, which no browser produces.
func (c Context) decodeBody(r *http.Request, opts Options) *model.Array {
	limits := opts.inputLimits()
	if isURLEncoded(r) {
		return shared.ParseStr(string(*c.rawBody), limits)
	}
	if !isMultipart(r) || r.MultipartForm == nil {
		return model.NewArray()
	}
	names := make([]string, 0, len(r.MultipartForm.Value))
	for name := range r.MultipartForm.Value {
		names = append(names, name)
	}
	sort.Strings(names)
	var pairs []shared.Pair
	for _, name := range names {
		for _, value := range r.MultipartForm.Value[name] {
			pairs = append(pairs, shared.Pair{Name: name, Value: value})
		}
	}
	return shared.ParsePairs(limits, pairs)
}

// capBody limits how much of a body of unannounced length a parser can read,
// so a chunked request cannot get past post_max_size by withholding its length.
func capBody(r *http.Request, limit Size) {
	if limit <= 0 || r.Body == nil || r.ContentLength >= 0 {
		return
	}
	r.Body = http.MaxBytesReader(nil, r.Body, limit.Bytes())
}

// collectUploads copies every file part of a parsed multipart body to a
// temporary file and records it on the context. PHP exposes a path on disk and
// no stream, so the copy happens up front; Cleanup removes
// whatever the script did not move away.
func (c Context) collectUploads(r *http.Request, maxFileSize Size) {
	if r.MultipartForm == nil {
		return
	}
	for field, headers := range r.MultipartForm.File {
		for _, header := range headers {
			upload := saveUpload(header, maxFileSize)
			if upload.Error == UploadErrIniSize {
				c.recordError(fmt.Errorf("uploaded file %q of %d bytes exceeds the upload_max_filesize limit of %d bytes",
					upload.Name, header.Size, maxFileSize.Bytes()))
			}
			files := c.filesInto()
			files[field] = append(files[field], upload)
		}
	}
	// net/http spills the parts it could not hold in memory into temporary
	// files of its own. saveUpload has copied every one of them by now, so the
	// originals go before the handler has a chance to forget about them.
	_ = r.MultipartForm.RemoveAll()
}

// saveUpload writes one file part to its own temporary file. It reports a
// failure the way PHP does, as an error code on the entry, because a script
// reads $_FILES[...]["error"] and catches nothing.
func saveUpload(header *multipart.FileHeader, maxFileSize Size) *UploadedFile {
	upload := &UploadedFile{
		Name:     uploadBaseName(header.Filename),
		FullPath: header.Filename,
		Type:     header.Header.Get("Content-Type"),
		Size:     header.Size,
		Error:    UploadErrOK,
	}
	// A part PHP refuses before it reads it keeps only the two names the
	// client sent: no type, no size, no temporary file.
	if upload.Name == "" {
		return upload.refuse(UploadErrNoFile)
	}
	if maxFileSize.Exceeds(header.Size) {
		return upload.refuse(UploadErrIniSize)
	}

	src, err := header.Open()
	if err != nil {
		return upload.refuse(UploadErrCantWrite)
	}
	defer src.Close()

	dst, err := os.CreateTemp("", "phpscript-upload-*")
	if err != nil {
		return upload.refuse(UploadErrNoTmpDir)
	}
	size, err := io.Copy(dst, src)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst.Name())
		return upload.refuse(UploadErrCantWrite)
	}

	upload.TmpName = dst.Name()
	upload.Size = size
	return upload
}

// uploadBaseName strips the directory part a client may have sent along with
// the file name. The separator is the client's, not this host's, so it cuts on
// both; PHP keeps the base name only.
func uploadBaseName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// Cleanup removes the temporary files created for the uploaded parts of a
// request. A host handler defers it for the lifetime of one request; a file the
// script moved with move_uploaded_file is already gone and Cleanup skips it.
func (c Context) Cleanup() {
	for _, files := range c.files() {
		for _, file := range files {
			if file.TmpName != "" {
				_ = os.Remove(file.TmpName)
			}
		}
	}
}

// IsUpload reports whether path is the temporary file of a part of this
// request. It backs is_uploaded_file() and move_uploaded_file(), which in PHP
// refuse any path the request did not produce. A path the script has already
// moved away drops out of that set, so the copy has to still be there.
func (c Context) IsUpload(path string) bool {
	c.decodeForm()
	for _, files := range c.files() {
		for _, file := range files {
			if file.TmpName != "" && file.TmpName == path {
				_, err := os.Stat(path)
				return err == nil
			}
		}
	}
	return false
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

// memoryFootprint estimates the bytes this Context holds on the host side of
// the request boundary: the superglobal source maps, headers, arguments and
// upload metadata. The superglobal arrays Register seeds into the runtime are
// separate copies the walk counts on its own.
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
	// runtime serves one request at a time, so the moment the next registers
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
	// to catch: it happened before the script started, so there is nothing for
	// a try/catch to wrap. See Errors.
	for _, err := range c.Errors() {
		rt.RecordError(err)
	}
}

// GetAllHeaders is getallheaders(): the request headers, canonically keyed.
func (c Context) GetAllHeaders() *model.Array {
	return mapToArray(c.HeaderMap())
}

// Header implements PHP header($header[, $replace[, $code]]): it parses a
// "Name: value" line and stages it on the response header set. replace controls
// whether an existing header of the same name is overwritten (default true).
func (c Context) Header(header string, opts ...any) {
	name, value, ok := strings.Cut(header, ":")
	if !ok {
		// A status line, "HTTP/1.0 404 Not Found", is the other thing header()
		// takes. It has no name:value shape, and what it carries is the status.
		if code, ok := parseStatusLine(header); ok {
			*c.status = code
		}
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
	// $code is an int parameter, so PHP coerces whatever the call passes, and
	// zero is the value that means the call named no status.
	if len(opts) > 1 {
		if code := toInt(opts[1]); code != 0 {
			*c.status = int(code)
		}
	}
}

// parseStatusLine reads the status out of a "HTTP/1.0 404 Not Found" line, the
// spelling header() takes in place of a name and a value.
//
// Neither the protocol nor the reason phrase reaches the response. Go writes
// the protocol it actually serves and the reason phrase that belongs to the
// code, so all the line can contribute is the number in the middle.
func parseStatusLine(header string) (int, bool) {
	if !strings.HasPrefix(header, "HTTP/") {
		return 0, false
	}
	fields := strings.Fields(header)
	if len(fields) < 2 {
		return 0, false
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil || code < 100 || code > 599 {
		return 0, false
	}
	return code, true
}

// HTTPResponseCode implements PHP http_response_code([$response_code]).
//
// With no argument it reports the status this response will carry, and with
// one it stages that status and reports the one it replaced.
//
// PHP answers false while nothing has set a status, and true for the first one
// set, having no previous number to return. On
// a web SAPI neither happens: a request starts out answering 200. sapi is the
// SAPI name the runtime runs under, and only the command line, or a host that
// named no SAPI at all, starts without a status.
func (c Context) HTTPResponseCode(sapi string, opts ...any) any {
	previous := c.ResponseStatus()
	if previous == 0 && sapi != "" && sapi != "cli" {
		previous = http.StatusOK
	}

	// The parameter is an int, so PHP coerces whatever the call passes, and
	// $_SERVER holds every value as a string: a page passing REDIRECT_STATUS
	// straight back is passing "404" and means 404.
	code := 0
	if len(opts) > 0 {
		code = int(toInt(opts[0]))
	}
	// A zero, a null and a false are not a status. PHP treats the call as the
	// reporting one, which is all any of them amounts to here.
	if code == 0 {
		if previous == 0 {
			return false
		}
		return int64(previous)
	}

	*c.status = code
	if previous == 0 {
		return true
	}
	return int64(previous)
}

// ResponseHeaders returns the headers header() staged, for a host to copy.
func (c Context) ResponseHeaders() http.Header { return c.response }

// RawBody returns the request body bytes buffered for php://input, if any. A
// multipart body answers empty, as PHP's php://input does.
func (c Context) RawBody() []byte {
	if c.rawBody == nil {
		return nil
	}
	// php://input is the body, so this read decodes the body if nothing has yet.
	c.decodeForm()
	return *c.rawBody
}

// SetRawBody stores the bytes php://input answers with, for a host that
// builds its Context by hand, with no *http.Request behind it.
func (c Context) SetRawBody(body []byte) {
	if c.rawBody != nil {
		*c.rawBody = body
	}
}

// RequestContext returns the request data registered on a runtime context.
// Request-aware Go bindings such as session management read through it.
func RequestContext(ctx context.Context) (Context, bool) {
	c, ok := ctx.Value(requestContextKey{}).(Context)
	return c, ok
}

// AddResponseHeader appends a header, for a Set-Cookie and its like.
func (c Context) AddResponseHeader(name, value string) {
	c.response.Add(name, value)
}

// ResponseStatus returns the status staged by header(), or zero when the host
// should keep its default status. A Location header defaults to 302 like PHP.
func (c Context) ResponseStatus() int {
	if c.status == nil {
		return 0
	}
	return *c.status
}

// uploadKeys is the key set of a $_FILES entry, in the order PHP writes it.
var uploadKeys = [...]string{"name", "full_path", "type", "tmp_name", "error", "size"}

// uploadValue reads one key of a $_FILES entry off an upload, so the scalar and
// the parallel-array shapes cannot disagree about what a key holds.
func uploadValue(file *UploadedFile, key string) any {
	switch key {
	case "name":
		return file.Name
	case "full_path":
		return file.FullPath
	case "type":
		return file.Type
	case "tmp_name":
		return file.TmpName
	case "error":
		return int64(file.Error)
	case "size":
		return file.Size
	}
	return nil
}

// filesArray renders $_FILES. A field named "files[]" takes PHP's parallel
// array shape, $_FILES["files"]["name"][0]; any other field takes the scalar
// shape, $_FILES["file"]["name"], and keeps the last file sent under it, the
// way a repeated form value assigns over the one before it.
func (c Context) filesArrayInto(arr *model.Array) *model.Array {
	if len(c.files()) == 0 {
		return arr
	}
	fields := make([]string, 0, len(c.files()))
	for field := range c.files() {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		files := c.files()[field]
		if len(files) == 0 {
			continue
		}
		if name, isList := strings.CutSuffix(field, "[]"); isList {
			arr.Set(name, uploadListArray(files))
			continue
		}
		arr.Set(field, uploadArray(files[len(files)-1]))
	}
	return arr
}

// uploadArray renders one upload as PHP's keyed array.
func uploadArray(file *UploadedFile) *model.Array {
	arr := model.NewArraySize(len(uploadKeys))
	for _, key := range uploadKeys {
		arr.Set(key, uploadValue(file, key))
	}
	return arr
}

// uploadListArray renders a list field: the same keys, each holding one value
// per file in place of a single value.
func uploadListArray(files []*UploadedFile) *model.Array {
	arr := model.NewArraySize(len(uploadKeys))
	for _, key := range uploadKeys {
		column := model.NewArraySize(len(files))
		for i, file := range files {
			column.Set(int64(i), uploadValue(file, key))
		}
		arr.Set(key, column)
	}
	return arr
}

// serverArray renders $_SERVER. Context.Server holds every value as a string
// because that is what all but two of PHP's server keys are; the two that are
// not, REQUEST_TIME as an integer and REQUEST_TIME_FLOAT as a float, get their
// type back here, so a script comparing either with === sees what PHP writes.
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
// Session\Manager's and no request input, so it builds nothing here.
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
// itself, so naming the variable is what parses it.
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

// arraySource reads a *model.Array as a mapmap source, so a read reaches a
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
// A read goes to the source, which is the request where there is one, so a
// script that reads a key or two copies nothing and one that reads none costs
// nothing at all. ServerMap seeds the two times because PHP answers them as
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

// requestArray renders $_REQUEST: the query, form and cookie fields merged in
// PHP's default request order, with the route's path values written over them
// last. The path values are the one deviation from PHP, whose $_REQUEST
// carries no route parameters: a path parameter is request input here, and
// it carries under PHP's name, so nothing here spells a name PHP does not
// have. See the predefined-variables reference.
//
// Every entry is a copy. $_REQUEST is its own array in PHP, so a write to it
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
