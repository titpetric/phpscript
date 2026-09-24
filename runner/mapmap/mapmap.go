// Package mapmap holds a read-only map with a request-scoped layer of edits
// over it.
//
// A superglobal is read far more often than it is written, and what it reads is
// the request, which does not change while the request is served. Rendering it
// into a *model.Array copies every entry into a second representation on the
// way in, whether or not the script names the variable. A MapMap answers from
// the request instead, and holds only what a script wrote.
//
// The edits are request-scoped, so their map comes from a pool and goes back
// cleared rather than freed.
package mapmap

import (
	"net/http"
	"sync"
)

// Source is the read-only half of a MapMap: the request, or a map built for a
// run that has no request behind it.
//
// A source is read while the request is served and is never written, so one may
// be shared by every MapMap over it.
type Source interface {
	// Get answers the value of key and whether the source holds it.
	Get(key string) (any, bool)

	// Len counts the keys Range would visit.
	Len() int

	// Range visits every key in the source until fn answers false.
	Range(fn func(key string, value any) bool) bool
}

// MapMap reads through a Source and writes to a layer of its own.
//
// Read answers the layer first, so a script that assigns to a key sees what it
// assigned; the source is left as it was and stays shared. Nothing is copied
// out of the source until something asks for the whole thing.
type MapMap struct {
	src Source

	// edits holds what a script wrote, and dropped the keys it unset. Both are
	// nil until the first write, because most requests never make one.
	edits   map[string]any
	dropped map[string]struct{}
}

// New returns a MapMap reading through src. A nil source reads as empty.
func New(src Source) *MapMap {
	return &MapMap{src: src}
}

// Read answers the value of key, or nil when nothing holds it.
func (m *MapMap) Read(key string) any {
	if m == nil {
		return nil
	}
	if v, ok := m.edits[key]; ok {
		return v
	}
	if _, gone := m.dropped[key]; gone {
		return nil
	}
	if m.src != nil {
		if v, ok := m.src.Get(key); ok {
			return v
		}
	}
	return nil
}

// Has reports whether anything holds key, which is what isset() asks.
func (m *MapMap) Has(key string) bool {
	if m == nil {
		return false
	}
	if _, ok := m.edits[key]; ok {
		return true
	}
	if _, gone := m.dropped[key]; gone {
		return false
	}
	if m.src == nil {
		return false
	}
	_, ok := m.src.Get(key)
	return ok
}

// Write records a value for key, leaving the source as it was.
func (m *MapMap) Write(key string, value any) {
	if m.edits == nil {
		m.edits = editsPool.Get().(map[string]any)
	}
	m.edits[key] = value
	delete(m.dropped, key)
}

// Delete forgets key, including one the source holds.
func (m *MapMap) Delete(key string) {
	delete(m.edits, key)
	if m.src == nil {
		return
	}
	if _, ok := m.src.Get(key); !ok {
		return
	}
	if m.dropped == nil {
		m.dropped = map[string]struct{}{}
	}
	m.dropped[key] = struct{}{}
}

// Len counts what Range would visit, which is count().
func (m *MapMap) Len() int {
	if m == nil {
		return 0
	}
	n := len(m.edits)
	if m.src == nil {
		return n
	}
	m.src.Range(func(key string, _ any) bool {
		if _, written := m.edits[key]; written {
			return true
		}
		if _, gone := m.dropped[key]; gone {
			return true
		}
		n++
		return true
	})
	return n
}

// Range visits the source in its own order, then the keys only the layer holds.
//
// A key the layer overwrote is visited in the source's position with the
// written value, so assigning to an existing key does not move it.
func (m *MapMap) Range(fn func(key, value any) bool) {
	if m == nil {
		return
	}
	seen := 0
	if m.src != nil {
		ok := m.src.Range(func(key string, value any) bool {
			if _, gone := m.dropped[key]; gone {
				return true
			}
			if written, ok := m.edits[key]; ok {
				seen++
				return fn(key, written)
			}
			return fn(key, value)
		})
		if !ok {
			return
		}
	}
	if len(m.edits) == seen {
		return
	}
	for key, value := range m.edits {
		if m.src != nil {
			if _, ok := m.src.Get(key); ok {
				continue
			}
		}
		if !fn(key, value) {
			return
		}
	}
}

// Release returns the layer to the pool. The MapMap reads as the source alone
// afterwards, so a caller releases when the request it belongs to is over.
func (m *MapMap) Release() {
	if m == nil || m.edits == nil {
		return
	}
	clear(m.edits)
	editsPool.Put(m.edits)
	m.edits = nil
	m.dropped = nil
}

// editsPool holds the written-key maps between requests. A map returned to it
// is cleared rather than dropped, so the next request writes into buckets that
// are already there.
var editsPool = sync.Pool{New: func() any { return map[string]any{} }}

// MapSource reads a plain map. It is the source for a run with no request
// behind it, which is every CLI invocation.
type MapSource map[string]string

// Get answers the value of key.
func (s MapSource) Get(key string) (any, bool) {
	v, ok := s[key]
	return v, ok
}

// Len counts the entries.
func (s MapSource) Len() int { return len(s) }

// Range visits every entry. Go map order is not insertion order, which is the
// order a script reading this sees.
func (s MapSource) Range(fn func(key string, value any) bool) bool {
	for key, value := range s {
		if !fn(key, value) {
			return false
		}
	}
	return true
}

// ValueSource reads a map whose values are already PHP values, which is what a
// superglobal carrying nested data needs: a query string decodes `a[]=1&a[]=2`
// to an array under one name, and no map of strings can hold that.
type ValueSource map[string]any

// Get answers the value of key.
func (s ValueSource) Get(key string) (any, bool) {
	v, ok := s[key]
	return v, ok
}

// Len counts the entries.
func (s ValueSource) Len() int { return len(s) }

// Range visits every entry.
func (s ValueSource) Range(fn func(key string, value any) bool) bool {
	for key, value := range s {
		if !fn(key, value) {
			return false
		}
	}
	return true
}

// RequestSource derives the CGI names from the request itself, so serving one
// builds no map.
//
// Extra carries the names the request cannot answer on its own, which are the
// ones a caller computed: the clock for REQUEST_TIME, and whatever the host
// decided about the scheme behind a proxy. It is a Source of its own so a
// caller passes the map it already has rather than copying it into another.
// It is read, never written, and wins over a derived name.
type RequestSource struct {
	Request *http.Request
	Extra   Source
}

// extra answers one name from Extra, tolerating a nil one.
func (s *RequestSource) extra(key string) (any, bool) {
	if s.Extra == nil {
		return nil, false
	}
	return s.Extra.Get(key)
}

// Get answers one CGI name.
func (s *RequestSource) Get(key string) (any, bool) {
	if s == nil || s.Request == nil {
		return "", false
	}
	if v, ok := s.extra(key); ok {
		return v, true
	}
	if name, ok := headerName(key); ok {
		if v := s.Request.Header.Get(name); v != "" {
			return v, true
		}
		return "", false
	}
	switch key {
	case "REQUEST_METHOD":
		return s.Request.Method, true
	case "REQUEST_URI":
		return s.Request.URL.RequestURI(), true
	case "QUERY_STRING":
		return s.Request.URL.RawQuery, true
	case "HTTP_HOST":
		return s.Request.Host, true
	case "SERVER_PROTOCOL":
		return s.Request.Proto, true
	}
	return "", false
}

// Len counts the names Range visits.
func (s *RequestSource) Len() int {
	n := 0
	s.Range(func(string, any) bool {
		n++
		return true
	})
	return n
}

// Range visits the derived names, then the extras that are not among them.
func (s *RequestSource) Range(fn func(key string, value any) bool) bool {
	if s == nil || s.Request == nil {
		return true
	}
	for _, key := range derived {
		if _, ok := s.extra(key); ok {
			continue
		}
		value, ok := s.Get(key)
		if !ok {
			continue
		}
		if !fn(key, value) {
			return false
		}
	}
	for name, values := range s.Request.Header {
		if len(values) == 0 {
			continue
		}
		key := cgiName(name)
		if _, ok := s.extra(key); ok {
			continue
		}
		if !fn(key, values[0]) {
			return false
		}
	}
	if s.Extra == nil {
		return true
	}
	return s.Extra.Range(fn)
}

// derived names the CGI keys Get answers off the request.
var derived = []string{
	"REQUEST_METHOD",
	"REQUEST_URI",
	"QUERY_STRING",
	"HTTP_HOST",
	"SERVER_PROTOCOL",
}

// headerName turns HTTP_ACCEPT_ENCODING back into Accept-Encoding, and reports
// whether the key named a header at all. HTTP_HOST is not one: the host is a
// field of the request rather than a header Go keeps.
func headerName(key string) (string, bool) {
	const prefix = "HTTP_"
	if key == "HTTP_HOST" || len(key) <= len(prefix) || key[:len(prefix)] != prefix {
		return "", false
	}
	name := []byte(key[len(prefix):])
	upper := true
	for i, c := range name {
		switch {
		case c == '_':
			name[i] = '-'
			upper = true
		case upper:
			upper = false
		case c >= 'A' && c <= 'Z':
			name[i] = c + ('a' - 'A')
		}
	}
	return string(name), true
}

// cgiName turns Accept-Encoding into HTTP_ACCEPT_ENCODING.
func cgiName(name string) string {
	out := make([]byte, 0, len("HTTP_")+len(name))
	out = append(out, "HTTP_"...)
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '-':
			out = append(out, '_')
		case c >= 'a' && c <= 'z':
			out = append(out, c-('a'-'A'))
		default:
			out = append(out, c)
		}
	}
	return string(out)
}
