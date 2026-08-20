package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/titpetric/phpscript/model"
)

// PHP's UPLOAD_ERR_* codes, as they appear in a $_FILES entry. Only the ones
// this runtime can produce are named; 2, 3 and 8 come from a per-form limit and
// from extensions that phpscript has no equivalent of.
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

// UploadedFile is one file part of a multipart request body, in the shape a
// PHP $_FILES entry exposes. TmpName is the absolute path of the server-side
// copy, which lives until Cleanup runs or the script moves it away.
type UploadedFile struct {
	Name string // client-supplied file name, without any directory part
	// FullPath is the file name as the client sent it, directory part and all,
	// which a directory upload uses to say where in the tree a file sat. PHP
	// 8.1 added it; like Name, it is the client's word and not a path on this
	// host.
	FullPath string
	Type     string // client-supplied content type
	TmpName  string // path of the temporary copy, empty when Error is set
	Size     int64
	Error    int // an UPLOAD_ERR_* code, UploadErrOK when the part was stored
}

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

	// Files holds the file parts of a multipart body keyed by form field name,
	// in the order they were sent. A field carries more than one file when the
	// form repeats it, which HTML spells as a "name[]" field.
	Files map[string][]*UploadedFile

	// response collects headers set by the PHP header() function. It is an
	// http.Header (a map, hence reference-shared across copies of Context) so a
	// host handler can flush it onto the response after execution.
	response http.Header
	status   *int
	// errors holds what went wrong with the request itself, before any script
	// ran: a body refused for its size. It is a pointer for the same reason
	// status is, so a copy of a Context reports what another copy recorded.
	errors *[]error
}

type requestContextKey struct{}

// NewContext returns an allocated empty Context value.
func NewContext() Context {
	status := 0
	var errs []error
	return Context{
		errors:   &errs,
		Get:      map[string]string{},
		Post:     map[string]string{},
		Path:     map[string]string{},
		Cookie:   map[string]string{},
		Server:   map[string]string{},
		Env:      map[string]string{},
		Headers:  map[string]string{},
		Argv:     []string{},
		Files:    map[string][]*UploadedFile{},
		response: http.Header{},
		status:   &status,
	}
}

// pathVarRE matches Go 1.22+ ServeMux wildcard segments, e.g. {id} or {rest...}.
var pathVarRE = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)(\.\.\.)?\}`)

// FromRequest builds a Context from an HTTP request with no size limits on the
// body. A host that has runtime options, which is every host that reads a
// configuration file, uses FromRequestOptions instead.
func FromRequest(r *http.Request) Context {
	return FromRequestOptions(r, Options{})
}

// FromRequestOptions builds a Context from an HTTP request. Query and form
// values are flattened to one value per key (PHP's scalar superglobal shape);
// path values are pulled out of the matched ServeMux pattern via r.PathValue.
//
// A key sent more than once keeps the last value, which is what PHP's own
// parser does: each repetition assigns over the one before it.
//
// The upload_max_filesize and post_max_size options limit what the body may
// carry; see enforcement in the form-body section below.
func FromRequestOptions(r *http.Request, opts Options) Context {
	c := NewContext()

	// Query string ($_GET).
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			c.Get[k] = v[len(v)-1]
		}
	}

	// Form body ($_POST) and file uploads ($_FILES). A body over post_max_size
	// is not parsed at all: PHP leaves both superglobals empty rather than
	// giving a script half a form, and says so in its log.
	if opts.PostMaxSize.Exceeds(r.ContentLength) {
		c.recordError(fmt.Errorf("POST Content-Length of %d bytes exceeds the post_max_size limit of %d bytes",
			r.ContentLength, opts.PostMaxSize.Bytes()))
	} else {
		c.parseBody(r, opts)
	}

	// Cookies ($_COOKIE).
	for _, ck := range r.Cookies() {
		c.Cookie[ck.Name] = ck.Value
	}

	// Server variables ($_SERVER).
	c.serverVars(r)

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

// serverVars fills $_SERVER with the part of PHP's server array that the
// request itself answers for. Every value is a string, which is what $_SERVER
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
	c.Server["REQUEST_METHOD"] = r.Method
	c.Server["REQUEST_URI"] = r.URL.RequestURI()
	c.Server["QUERY_STRING"] = r.URL.RawQuery
	c.Server["HTTP_HOST"] = r.Host
	c.Server["SERVER_PROTOCOL"] = r.Proto

	// PHP splits the peer address in two, the address in REMOTE_ADDR and the
	// port in REMOTE_PORT, where Go keeps both in RemoteAddr as "address:port".
	// An IPv6 address arrives bracketed and leaves without, as PHP writes it.
	// A Go host is free to put anything in the field, so one that does not
	// parse is passed through whole and leaves REMOTE_PORT unset.
	if addr, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		c.Server["REMOTE_ADDR"] = addr
		c.Server["REMOTE_PORT"] = port
	} else if r.RemoteAddr != "" {
		c.Server["REMOTE_ADDR"] = r.RemoteAddr
	}

	// The scheme is the one this process is serving, r.TLS, and not the one
	// X-Forwarded-Proto claims: that header is the client's to send, and a
	// script deciding on it whether it is talking over TLS would be deciding on
	// what the client said. A host behind a proxy that terminates TLS is the
	// one that knows the proxy is trusted, and sets the two keys itself.
	// HTTPS is unset on a plain request rather than "off", which is why an
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
	// REQUEST_TIME the whole second of it. Both are read here rather than per
	// key so the two cannot disagree about which second the request began in.
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
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	return mediaType == "multipart/form-data"
}

// parseBody decodes the request body into $_POST and $_FILES. ParseForm only
// decodes an urlencoded body; a multipart one leaves it with an empty PostForm,
// which is why a form with a file input used to arrive empty. Only the content
// type says which of the two a request carries. Both calls are idempotent and a
// no-op for bodyless requests, so neither needs a method check.
func (c Context) parseBody(r *http.Request, opts Options) {
	capBody(r, opts.PostMaxSize)

	var err error
	if isMultipart(r) {
		if err = r.ParseMultipartForm(maxMultipartMemory); err == nil {
			c.collectUploads(r, opts.UploadMaxFilesize)
		}
	} else {
		err = r.ParseForm()
	}

	// A body that did not announce its length is only found to be too large
	// once it has been read past the cap. Whatever was decoded before that
	// point is dropped, so this ends the same way an announced one does.
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		c.recordError(fmt.Errorf("POST body exceeds the post_max_size limit of %d bytes", tooLarge.Limit))
		return
	}

	for k, v := range r.PostForm {
		if len(v) > 0 {
			c.Post[k] = v[len(v)-1]
		}
	}
}

// capBody limits how much of a body of unannounced length is read, so a
// chunked request cannot get past post_max_size by not saying how large it is.
func capBody(r *http.Request, limit Size) {
	if limit <= 0 || r.Body == nil || r.ContentLength >= 0 {
		return
	}
	r.Body = http.MaxBytesReader(nil, r.Body, limit.Bytes())
}

// collectUploads copies every file part of a parsed multipart body to a
// temporary file and records it on the context. PHP hands a script a path on
// disk rather than a stream, so the copy happens up front; Cleanup removes
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
			c.Files[field] = append(c.Files[field], upload)
		}
	}
	// net/http spills the parts it could not hold in memory into temporary
	// files of its own. Every one of them has been copied by now, so the
	// originals go before the handler has a chance to forget about them.
	_ = r.MultipartForm.RemoveAll()
}

// saveUpload writes one file part to its own temporary file. A failure is
// reported the way PHP reports it, as an error code on the entry, because a
// script reads $_FILES[...]["error"] rather than catching anything.
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

// refuse marks an upload as not stored, in the shape PHP gives an entry it
// refused: the error code, and nothing that describes content there is none of.
func (u *UploadedFile) refuse(code int) *UploadedFile {
	u.Type = ""
	u.TmpName = ""
	u.Size = 0
	u.Error = code
	return u
}

// uploadBaseName strips the directory part a client may have sent along with
// the file name. The separator is the client's, not this host's, so both are
// cut; PHP keeps the base name only.
func uploadBaseName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// Cleanup removes the temporary files created for the uploaded parts of a
// request. A host handler defers it for the lifetime of one request; a file the
// script moved with move_uploaded_file is already gone and is skipped.
func (c Context) Cleanup() {
	for _, files := range c.Files {
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
// moved away is no longer one of them, so the copy has to still be there.
func (c Context) IsUpload(path string) bool {
	for _, files := range c.Files {
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

// Errors returns what the request got wrong before any script ran, which today
// is a body or a file part refused for its size. A script has no way to catch
// these: they happened outside it, and the only sign of them it gets is the
// empty superglobal or the UPLOAD_ERR_INI_SIZE entry they left behind. Register
// reports each one to the runtime through Runtime.RecordError, so a Go host
// sees them on the request trace or through Runtime.OnError.
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
	for _, m := range []map[string]string{c.Get, c.Post, c.Path, c.Cookie, c.Server, c.Env, c.Headers} {
		total += DeepSize(m, visited)
	}
	total += DeepSize(c.Argv, visited)
	total += DeepSize(map[string][]string(c.response), visited)
	for name, files := range c.Files {
		total += 16 + int64(len(name)) + 24
		for _, file := range files {
			total += DeepSize(file, visited)
		}
	}
	return total
}

// Register installs the request-aware PHP functions onto rt and seeds the
// request superglobals. After this, transpiled PHP can call getallheaders() /
// header() and read $_GET, $_POST, $_PATH, all backed by this Context.
func (c Context) Register(rt *Runtime) {
	// The request has crossed into the runtime: fold its host-side size into
	// the memory baseline for the rest of this request's lifetime.
	rt.AccountRequest(c)
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
	rt.SetGlobal("_SERVER", c.serverArray())
	rt.SetGlobal("_ENV", mapToArray(c.Env))
	rt.SetGlobal("_PATH", mapToArray(c.Path))
	rt.SetGlobal("_FILES", c.filesArray())

	argvArr := model.NewArray()
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
func (c Context) filesArray() *model.Array {
	if len(c.Files) == 0 {
		return model.NewArray()
	}
	fields := make([]string, 0, len(c.Files))
	for field := range c.Files {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	arr := model.NewArraySize(len(fields))
	for _, field := range fields {
		files := c.Files[field]
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
// per file rather than a single value.
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
// type back here, so a script comparing either with === sees what PHP gives it.
func (c Context) serverArray() *model.Array {
	arr := mapToArray(c.Server)
	if seconds, err := strconv.ParseInt(c.Server["REQUEST_TIME"], 10, 64); err == nil {
		arr.Set("REQUEST_TIME", seconds)
	}
	if exact, err := strconv.ParseFloat(c.Server["REQUEST_TIME_FLOAT"], 64); err == nil {
		arr.Set("REQUEST_TIME_FLOAT", exact)
	}
	return arr
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
