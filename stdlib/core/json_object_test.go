package core

import (
	"encoding/json"
	"testing"
)

// TestJSONObjectMatchesEncodingJSON holds the fast paths in MarshalJSON to
// encoding/json's own bytes: a key or a value the appender writes itself has
// to come out byte for byte as json.Marshal writes it, escapes, HTML
// characters and non-ASCII included, or the two spellings of json_encode
// disagree.
func TestJSONObjectMatchesEncodingJSON(t *testing.T) {
	t.Parallel()
	values := []any{
		nil,
		true,
		false,
		int64(0),
		int64(-42),
		int64(9007199254740993),
		3.14,
		1e21,
		"",
		"plain",
		"with space / slash",
		`quote " and backslash \`,
		"html <b>&</b>",
		"control \n\t\r\x00",
		"unicode žščř    😀",
		"invalid \xff utf8",
		[]any{int64(1), "two", nil},
		map[string]any{"k": "v"},
	}
	for _, value := range values {
		buf, err := appendJSONValue(nil, value)
		if err != nil {
			t.Fatalf("appendJSONValue(%#v): %v", value, err)
		}
		want, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal(%#v): %v", value, err)
		}
		if string(buf) != string(want) {
			t.Errorf("appendJSONValue(%#v) = %s, want %s", value, buf, want)
		}
	}

	object := newJSONObject(3)
	object.add("id", int64(42))
	object.add("html <key>", "a&b")
	object.add("nested", []any{"x"})
	got, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	// The expected key and value come from json.Marshal, so the HTML escapes
	// are the library's and no literal in this file.
	key, _ := json.Marshal("html <key>")
	value, _ := json.Marshal("a&b")
	want := `{"id":42,` + string(key) + `:` + string(value) + `,"nested":["x"]}`
	if string(got) != want {
		t.Errorf("object = %s, want %s", got, want)
	}
}
