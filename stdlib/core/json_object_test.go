package core

import (
	"encoding/json"
	"testing"
)

// TestJSONObjectKeepsInsertionOrder covers the type encoding/json reaches on
// the indent path: the entries come out in the order add put them in, and the
// keys and values are the bytes json.Marshal writes for them, HTML escapes
// included.
func TestJSONObjectKeepsInsertionOrder(t *testing.T) {
	t.Parallel()
	object := newJSONObject(3)
	object.add("id", int64(42))
	object.add("html <key>", "a&b")
	object.add("nested", []any{"x"})
	got, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	// The expected key and value come from json.Marshal and the HTML escapes
	// are the library's and no literal in this file.
	key, _ := json.Marshal("html <key>")
	value, _ := json.Marshal("a&b")
	want := `{"id":42,` + string(key) + `:` + string(value) + `,"nested":["x"]}`
	if string(got) != want {
		t.Errorf("object = %s, want %s", got, want)
	}
}
