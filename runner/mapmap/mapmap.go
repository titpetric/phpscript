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

	// edits holds what a script wrote, in write order, and is nil until the
	// first write because most requests never make one.
	//
	// A slice rather than a map: a request writes a key or two, and a linear
	// scan over that beats hashing, with one allocation for the backing array
	// instead of one for the map and its bucket. Nothing here grows to the
	// size where the scan would lose.
	edits []edit
}

// edit is one write, or one removal when gone is set. A removal is recorded
// rather than applied, because the source it hides is read-only.
type edit struct {
	key   string
	value any
	gone  bool
}

// find answers the position of key in the edits, or -1.
func (m *MapMap) find(key string) int {
	for i := range m.edits {
		if m.edits[i].key == key {
			return i
		}
	}
	return -1
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
	if i := m.find(key); i >= 0 {
		if m.edits[i].gone {
			return nil
		}
		return m.edits[i].value
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
	if i := m.find(key); i >= 0 {
		return !m.edits[i].gone
	}
	if m.src == nil {
		return false
	}
	_, ok := m.src.Get(key)
	return ok
}

// Write records a value for key, leaving the source as it was.
func (m *MapMap) Write(key string, value any) {
	if i := m.find(key); i >= 0 {
		m.edits[i].value, m.edits[i].gone = value, false
		return
	}
	m.edits = append(m.edits, edit{key: key, value: value})
}

// Delete forgets key, including one the source holds.
func (m *MapMap) Delete(key string) {
	i := m.find(key)
	inSource := false
	if m.src != nil {
		_, inSource = m.src.Get(key)
	}
	// A key the source does not hold leaves no trace: forgetting the write is
	// the whole removal. One it does hold is marked, because the source cannot
	// be written to.
	if i >= 0 && !inSource {
		m.edits = append(m.edits[:i], m.edits[i+1:]...)
		return
	}
	if !inSource {
		return
	}
	if i >= 0 {
		m.edits[i].value, m.edits[i].gone = nil, true
		return
	}
	m.edits = append(m.edits, edit{key: key, gone: true})
}

// Len counts what Range would visit, which is count().
func (m *MapMap) Len() int {
	if m == nil {
		return 0
	}
	n := 0
	for i := range m.edits {
		if !m.edits[i].gone {
			n++
		}
	}
	if m.src == nil {
		return n
	}
	m.src.Range(func(key string, _ any) bool {
		if m.find(key) < 0 {
			n++
		}
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
	if m.src != nil {
		ok := m.src.Range(func(key string, value any) bool {
			if i := m.find(key); i >= 0 {
				if m.edits[i].gone {
					return true
				}
				return fn(key, m.edits[i].value)
			}
			return fn(key, value)
		})
		if !ok {
			return
		}
	}
	for i := range m.edits {
		if m.edits[i].gone {
			continue
		}
		if m.src != nil {
			if _, ok := m.src.Get(m.edits[i].key); ok {
				continue
			}
		}
		if !fn(m.edits[i].key, m.edits[i].value) {
			return
		}
	}
}

// Release returns the layer to the pool. The MapMap reads as the source alone
// afterwards, so a caller releases when the request it belongs to is over.
func (m *MapMap) Release() {
	if m == nil {
		return
	}
	m.edits = nil
}

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

// Lazy defers building a source until something reads it, which is what lets a
// superglobal cost nothing until a script names it.
//
// build runs at most once, on the first Get, Len or Range. A request that
// reaches no read never parses, and one that reads twice parses once.
func Lazy(build func() Source) Source { return &lazySource{build: build} }

type lazySource struct {
	once  sync.Once
	build func() Source
	src   Source
}

func (s *lazySource) resolve() Source {
	s.once.Do(func() {
		if s.build != nil {
			s.src = s.build()
		}
		s.build = nil
	})
	return s.src
}

// Get builds the source if it has not been built, then reads it.
func (s *lazySource) Get(key string) (any, bool) {
	src := s.resolve()
	if src == nil {
		return nil, false
	}
	return src.Get(key)
}

// Len builds the source if it has not been built, then counts it.
func (s *lazySource) Len() int {
	src := s.resolve()
	if src == nil {
		return 0
	}
	return src.Len()
}

// Range builds the source if it has not been built, then walks it.
func (s *lazySource) Range(fn func(key string, value any) bool) bool {
	src := s.resolve()
	if src == nil {
		return true
	}
	return src.Range(fn)
}
