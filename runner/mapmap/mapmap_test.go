package mapmap_test

import (
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/titpetric/phpscript/runner/mapmap"
)

func collect(m *mapmap.MapMap) map[string]any {
	out := map[string]any{}
	m.Range(func(key, value any) bool {
		out[key.(string)] = value
		return true
	})
	return out
}

func TestReadsThroughToTheSource(t *testing.T) {
	t.Parallel()
	m := mapmap.New(mapmap.MapSource{"REQUEST_METHOD": "GET"})
	if got := m.Read("REQUEST_METHOD"); got != "GET" {
		t.Fatalf("Read = %v, want GET", got)
	}
	if got := m.Read("MISSING"); got != nil {
		t.Fatalf("Read of a missing key = %v, want nil", got)
	}
	if !m.Has("REQUEST_METHOD") || m.Has("MISSING") {
		t.Fatal("Has disagrees with Read")
	}
	if got := m.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1", got)
	}
}

func TestWriteLeavesTheSourceAlone(t *testing.T) {
	t.Parallel()
	src := mapmap.MapSource{"REMOTE_ADDR": "10.0.0.1"}
	m := mapmap.New(src)
	m.Write("REMOTE_ADDR", "127.0.0.1")

	if got := m.Read("REMOTE_ADDR"); got != "127.0.0.1" {
		t.Fatalf("Read after Write = %v, want 127.0.0.1", got)
	}
	if src["REMOTE_ADDR"] != "10.0.0.1" {
		t.Fatalf("the source was written: %v", src["REMOTE_ADDR"])
	}
	// A second MapMap over the same source does not see the first one's edit,
	// so one source is safe to share between requests.
	if got := mapmap.New(src).Read("REMOTE_ADDR"); got != "10.0.0.1" {
		t.Fatalf("a second reader saw the edit: %v", got)
	}
}

func TestOverwriteDoesNotGrowTheLength(t *testing.T) {
	t.Parallel()
	m := mapmap.New(mapmap.MapSource{"A": "1", "B": "2"})
	m.Write("A", "9")
	if got := m.Len(); got != 2 {
		t.Fatalf("Len after overwriting = %d, want 2", got)
	}
	m.Write("C", "3")
	if got := m.Len(); got != 3 {
		t.Fatalf("Len after a new key = %d, want 3", got)
	}
	got := collect(m)
	want := map[string]any{"A": "9", "B": "2", "C": "3"}
	if len(got) != len(want) {
		t.Fatalf("Range visited %v, want %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("Range gave %v for %s, want %v", got[key], key, value)
		}
	}
}

func TestDeleteHidesASourceKey(t *testing.T) {
	t.Parallel()
	m := mapmap.New(mapmap.MapSource{"A": "1", "B": "2"})
	m.Delete("A")
	if m.Has("A") || m.Read("A") != nil {
		t.Fatal("the deleted key is still readable")
	}
	if got := m.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1", got)
	}
	if _, ok := collect(m)["A"]; ok {
		t.Fatal("Range visited the deleted key")
	}
	// Writing it back brings it back, at the value written.
	m.Write("A", "3")
	if got := m.Read("A"); got != "3" {
		t.Fatalf("Read after rewriting = %v, want 3", got)
	}
	if got := m.Len(); got != 2 {
		t.Fatalf("Len after rewriting = %d, want 2", got)
	}
}

func TestRangeStopsWhenAskedTo(t *testing.T) {
	t.Parallel()
	m := mapmap.New(mapmap.MapSource{"A": "1", "B": "2", "C": "3"})
	m.Write("D", "4")
	seen := 0
	m.Range(func(any, any) bool {
		seen++
		return false
	})
	if seen != 1 {
		t.Fatalf("Range visited %d entries after being stopped, want 1", seen)
	}
}

func TestReleaseDropsTheEdits(t *testing.T) {
	t.Parallel()
	m := mapmap.New(mapmap.MapSource{"A": "1"})
	m.Write("A", "2")
	m.Write("B", "3")
	m.Release()
	if got := m.Read("A"); got != "1" {
		t.Fatalf("Read after Release = %v, want the source value 1", got)
	}
	if m.Has("B") {
		t.Fatal("a written key survived Release")
	}
	// Writing after a release starts from nothing, and not from what the
	// previous request left behind.
	m.Write("C", "4")
	if m.Has("B") {
		t.Fatal("a released edit came back")
	}
	if got := m.Read("C"); got != "4" {
		t.Fatalf("Read after writing past a release = %v, want 4", got)
	}
}

func TestNilIsEmpty(t *testing.T) {
	t.Parallel()
	var m *mapmap.MapMap
	if m.Read("A") != nil || m.Has("A") || m.Len() != 0 {
		t.Fatal("a nil MapMap is not empty")
	}
	m.Range(func(any, any) bool {
		t.Fatal("a nil MapMap visited an entry")
		return false
	})
}

// TestRequestSourceReadsThroughAMapMap covers the layer over a request: a
// MapMap reads every derived name through to the source, and Len agrees with
// what Range visited, so count() and foreach() over a superglobal report the
// same set.
func TestRequestSourceReadsThroughAMapMap(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("POST", "http://example.com/a/b?q=1", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	m := mapmap.New(&mapmap.RequestSource{
		Request: req,
		Extra:   mapmap.ValueSource{"REQUEST_TIME": "1700000000"},
	})

	for key, want := range map[string]any{
		"REQUEST_METHOD":       "POST",
		"REQUEST_URI":          "/a/b?q=1",
		"QUERY_STRING":         "q=1",
		"HTTP_HOST":            "example.com",
		"HTTP_ACCEPT_ENCODING": "gzip",
		"REQUEST_TIME":         "1700000000",
	} {
		if got := m.Read(key); got != want {
			t.Errorf("Read(%q) = %v, want %v", key, got, want)
		}
	}
	if m.Has("HTTP_NOT_SENT") {
		t.Error("a header that was not sent reads as present")
	}

	got := collect(m)
	if len(got) != m.Len() {
		t.Fatalf("Len = %d but Range visited %d", m.Len(), len(got))
	}
	keys := make([]string, 0, len(got))
	for key := range got {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if got[key] == "" {
			t.Errorf("Range visited %s with an empty value", key)
		}
	}
}

// rangeAll walks src twice: once to the end, and once cut short at the first
// entry. It returns what the complete walk visited, and checks the bool Range
// answers, so a MapMap over the source tells a finished walk from a
// stopped one.
func rangeAll(t *testing.T, src mapmap.Source) map[string]any {
	t.Helper()

	seen := map[string]any{}
	if !src.Range(func(key string, value any) bool {
		seen[key] = value
		return true
	}) {
		t.Error("Range() = false after a complete walk, want true")
	}

	visited := 0
	stopped := src.Range(func(string, any) bool {
		visited++
		return false
	})
	if src.Len() > 0 && (stopped || visited != 1) {
		t.Errorf("a stopped Range visited %d entries and answered %t, want 1 and false", visited, stopped)
	}
	return seen
}

// TestSource covers the contract the read-only half of a MapMap must meet.
// Each of the three implementations answers Get, Len and Range, and a MapMap
// built over any of them reads through to it, so one source type stands in for
// another and the layer over it stays put.
func TestSource(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest("GET", "http://example.com/", nil)

	for _, test := range []struct {
		name string
		src  mapmap.Source
		key  string
		want any
	}{
		{name: "MapSource", src: mapmap.MapSource{"A": "1"}, key: "A", want: "1"},
		{name: "ValueSource", src: mapmap.ValueSource{"A": int64(1)}, key: "A", want: int64(1)},
		{
			name: "RequestSource",
			src:  &mapmap.RequestSource{Request: request},
			key:  "REQUEST_METHOD",
			want: "GET",
		},
		{name: "Lazy", src: mapmap.Lazy(func() mapmap.Source { return mapmap.MapSource{"A": "1"} }), key: "A", want: "1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := test.src.Get(test.key); !ok || got != test.want {
				t.Errorf("Get(%q) = %v, %t, want %v, true", test.key, got, ok, test.want)
			}
			if test.src.Len() == 0 {
				t.Error("Len() = 0 on a source holding an entry")
			}
			if got := mapmap.New(test.src).Read(test.key); got != test.want {
				t.Errorf("a MapMap over it read %v, want %v", got, test.want)
			}
		})
	}
}

// TestMapSource_Get covers the read a CLI run answers through: a plain map of
// strings, where a key the map does not hold answers absent and never answers
// an empty string, because isset() reads the second return.
func TestMapSource_Get(t *testing.T) {
	t.Parallel()
	src := mapmap.MapSource{"REQUEST_METHOD": "GET", "EMPTY": ""}

	if got, ok := src.Get("REQUEST_METHOD"); !ok || got != "GET" {
		t.Errorf("Get(REQUEST_METHOD) = %v, %t, want GET, true", got, ok)
	}
	// An entry holding the empty string counts as present, so the two returns
	// disagree.
	if got, ok := src.Get("EMPTY"); !ok || got != "" {
		t.Errorf("Get(EMPTY) = %v, %t, want the empty string, true", got, ok)
	}
	if got, ok := src.Get("MISSING"); ok {
		t.Errorf("Get(MISSING) = %v, true, want false", got)
	}
}

// TestMapSource_Len covers count() over a CLI source: one per entry, and zero
// for a map nobody filled.
func TestMapSource_Len(t *testing.T) {
	t.Parallel()
	if got := (mapmap.MapSource{"A": "1", "B": "2"}).Len(); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}
	if got := (mapmap.MapSource{}).Len(); got != 0 {
		t.Errorf("Len() of an empty source = %d, want 0", got)
	}
}

// TestMapSource_Range covers the walk, and the bool that cuts it short. Go
// map order is not insertion order, so the assertions take what the walk
// visited and never the sequence it visited them in.
func TestMapSource_Range(t *testing.T) {
	t.Parallel()
	src := mapmap.MapSource{"A": "1", "B": "2"}

	seen := rangeAll(t, src)
	if len(seen) != 2 || seen["A"] != "1" || seen["B"] != "2" {
		t.Errorf("Range visited %v, want A=1 and B=2", seen)
	}
}

// TestValueSource_Get covers the source a superglobal carrying nested data
// reads through. The values are already PHP values, so `a[]=1&a[]=2` decoded
// to an array under one name comes back as that array; a map of strings
// cannot hold one.
func TestValueSource_Get(t *testing.T) {
	t.Parallel()
	nested := []any{int64(1), int64(2)}
	src := mapmap.ValueSource{"a": nested, "q": "1"}

	got, ok := src.Get("a")
	if !ok {
		t.Fatal("Get(a) reported the key as absent")
	}
	values, isSlice := got.([]any)
	if !isSlice || len(values) != 2 || values[0] != int64(1) {
		t.Errorf("Get(a) = %#v, want the array it was built with", got)
	}
	if got, ok := src.Get("missing"); ok {
		t.Errorf("Get(missing) = %v, true, want false", got)
	}
}

// TestValueSource_Len covers count() over a source of PHP values: one per
// name, at any depth of the value under it.
func TestValueSource_Len(t *testing.T) {
	t.Parallel()
	src := mapmap.ValueSource{"a": []any{int64(1), int64(2)}, "q": "1"}
	if got := src.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}
	if got := (mapmap.ValueSource{}).Len(); got != 0 {
		t.Errorf("Len() of an empty source = %d, want 0", got)
	}
}

// TestValueSource_Range covers the walk over PHP values: every name once,
// carrying the value the source stores, which the walk never flattens to a
// string.
func TestValueSource_Range(t *testing.T) {
	t.Parallel()
	nested := []any{int64(1)}
	src := mapmap.ValueSource{"a": nested, "q": "1"}

	seen := rangeAll(t, src)
	if len(seen) != 2 || seen["q"] != "1" {
		t.Errorf("Range visited %v, want a and q", seen)
	}
	if values, ok := seen["a"].([]any); !ok || len(values) != 1 {
		t.Errorf("Range visited a as %#v, want the array itself", seen["a"])
	}
}

// TestRequestSource_Get covers the CGI names derived off the request, so
// serving one builds no map. A header arrives under its HTTP_ spelling, a
// header the client omits reads as absent, Extra wins over a derived name,
// and a source with no request behind it answers nothing.
func TestRequestSource_Get(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest("POST", "http://example.com/a/b?q=1", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	src := &mapmap.RequestSource{
		Request: request,
		Extra:   mapmap.ValueSource{"REQUEST_METHOD": "CLI"},
	}

	for key, want := range map[string]any{
		"REQUEST_METHOD":       "CLI",
		"REQUEST_URI":          "/a/b?q=1",
		"QUERY_STRING":         "q=1",
		"HTTP_HOST":            "example.com",
		"SERVER_PROTOCOL":      "HTTP/1.1",
		"HTTP_ACCEPT_ENCODING": "gzip",
	} {
		if got, ok := src.Get(key); !ok || got != want {
			t.Errorf("Get(%q) = %v, %t, want %v, true", key, got, ok, want)
		}
	}
	if _, ok := src.Get("HTTP_NOT_SENT"); ok {
		t.Error("Get of a header that was not sent reported it as present")
	}
	if _, ok := (&mapmap.RequestSource{}).Get("REQUEST_METHOD"); ok {
		t.Error("Get on a source with no request reported a name as present")
	}
}

// TestRequestSource_Len covers count() over a request: the derived names plus
// the headers plus the extras, each counted once. Len answers the number of
// names Range visits, so the two agree by construction and a name counted
// twice in one shows up in both.
func TestRequestSource_Len(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest("GET", "http://example.com/?q=1", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	src := &mapmap.RequestSource{
		Request: request,
		Extra:   mapmap.ValueSource{"REQUEST_TIME": "1700000000"},
	}

	// Five derived names, one header, one extra.
	if got := src.Len(); got != 7 {
		t.Errorf("Len() = %d, want 7", got)
	}
	if got := len(rangeAll(t, src)); got != src.Len() {
		t.Errorf("Range visited %d names and Len() = %d", got, src.Len())
	}
}

// TestRequestSource_Range covers the order the names arrive in and the
// de-duplication between the halves: the derived names first, then the
// headers, then the extras that are not among either. The walk passes over a
// name Extra holds where it would otherwise derive one, so that name arrives
// once, carrying the value Extra holds.
func TestRequestSource_Range(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest("GET", "http://example.com/?q=1", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	src := &mapmap.RequestSource{
		Request: request,
		Extra:   mapmap.ValueSource{"REQUEST_METHOD": "CLI"},
	}

	seen := rangeAll(t, src)
	if seen["REQUEST_METHOD"] != "CLI" {
		t.Errorf("Range visited REQUEST_METHOD as %v, want the Extra value CLI", seen["REQUEST_METHOD"])
	}
	if seen["QUERY_STRING"] != "q=1" || seen["HTTP_ACCEPT_ENCODING"] != "gzip" {
		t.Errorf("Range visited %v, want the derived names and the header", seen)
	}

	counted := 0
	src.Range(func(key string, _ any) bool {
		if key == "REQUEST_METHOD" {
			counted++
		}
		return true
	})
	if counted != 1 {
		t.Errorf("REQUEST_METHOD visited %d times, want 1", counted)
	}

	// A source with no request behind it visits nothing and reports a
	// finished walk, so a MapMap over it reads as empty.
	if !(&mapmap.RequestSource{}).Range(func(string, any) bool { return false }) {
		t.Error("Range on a source with no request answered false")
	}
}

// TestLazy covers the deferral a superglobal costs nothing until a script
// names it: build runs on the first Get, Len or Range, at most once, and
// never at all for a request that reads none of them.
func TestLazy(t *testing.T) {
	t.Parallel()
	built := 0
	src := mapmap.Lazy(func() mapmap.Source {
		built++
		return mapmap.MapSource{"A": "1"}
	})
	if built != 0 {
		t.Fatalf("Lazy built the source %d times before the first read", built)
	}

	if got, ok := src.Get("A"); !ok || got != "1" {
		t.Errorf("Get(A) = %v, %t, want 1, true", got, ok)
	}
	if got := src.Len(); got != 1 {
		t.Errorf("Len() = %d, want 1", got)
	}
	if got := len(rangeAll(t, src)); got != 1 {
		t.Errorf("Range visited %d entries, want 1", got)
	}
	if built != 1 {
		t.Errorf("Lazy built the source %d times across three reads, want 1", built)
	}

	// A nil build reads as an empty source, so a caller that has nothing to
	// defer passes nothing and the reads still answer.
	empty := mapmap.Lazy(nil)
	if _, ok := empty.Get("A"); ok || empty.Len() != 0 {
		t.Error("a Lazy source with no build function is not empty")
	}
}
