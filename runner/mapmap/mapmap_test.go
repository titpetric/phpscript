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
	// which is what makes one source safe to share between requests.
	if got := mapmap.New(src).Read("REMOTE_ADDR"); got != "10.0.0.1" {
		t.Fatalf("a second reader saw the edit: %v", got)
	}
}

func TestOverwriteDoesNotGrowTheLength(t *testing.T) {
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
	// The pooled map is reused, so a release followed by a write must not
	// resurrect what the previous holder wrote.
	next := mapmap.New(mapmap.MapSource{})
	next.Write("C", "4")
	if next.Has("B") {
		t.Fatal("a pooled map came back with its old keys")
	}
}

func TestNilIsEmpty(t *testing.T) {
	var m *mapmap.MapMap
	if m.Read("A") != nil || m.Has("A") || m.Len() != 0 {
		t.Fatal("a nil MapMap is not empty")
	}
	m.Range(func(any, any) bool {
		t.Fatal("a nil MapMap visited an entry")
		return false
	})
}

func TestRequestSourceDerivesTheCGINames(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/a/b?q=1", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	src := &mapmap.RequestSource{
		Request: req,
		Extra:   mapmap.ValueSource{"REQUEST_TIME": "1700000000"},
	}
	m := mapmap.New(src)

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

	// Range visits each name once, and Len agrees with it.
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
			t.Errorf("Range gave an empty value for %s", key)
		}
	}
}

func TestExtraOverridesTheRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	m := mapmap.New(&mapmap.RequestSource{
		Request: req,
		Extra:   mapmap.ValueSource{"REQUEST_METHOD": "CLI"},
	})
	if got := m.Read("REQUEST_METHOD"); got != "CLI" {
		t.Fatalf("Read = %v, want the extra value CLI", got)
	}
	seen := 0
	m.Range(func(key, _ any) bool {
		if key == "REQUEST_METHOD" {
			seen++
		}
		return true
	})
	if seen != 1 {
		t.Fatalf("REQUEST_METHOD visited %d times, want 1", seen)
	}
}
