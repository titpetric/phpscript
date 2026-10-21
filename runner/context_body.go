package runner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/shared"
)

// This file decodes a request body: the one read that cannot run twice, so
// $_POST, $_FILES and php://input all resolve through parseBody. The query
// and cookie decodes, which the request answers at any time, stay in
// context.go with the Context itself.

// maxMultipartMemory bounds how much of a multipart body net/http keeps in
// memory before spilling parts to disk. It matches the net/http default.
const maxMultipartMemory = 32 << 20

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
