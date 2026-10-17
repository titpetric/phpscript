package core

import (
	"encoding/json"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestAppendJSONValueMatchesEncodingJSON holds the scalar shapes the appender
// writes itself to encoding/json's own bytes: a value it writes has to come out
// byte for byte as json.Marshal writes it, escapes, HTML characters and
// non-ASCII included, or the two spellings of json_encode disagree.
func TestAppendJSONValueMatchesEncodingJSON(t *testing.T) {
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
		"unicode žščř    😀",
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
}

// phpValues answers the value shapes a script hands the encoder: the
// collections, nested inside each other, and the keys that decide between a
// JSON array and a JSON object.
func phpValues() []any {
	list := model.NewArray()
	list.Set(int64(0), "a")
	list.Set(int64(1), int64(2))

	sparse := model.NewArray()
	sparse.Set(int64(1), "one")
	sparse.Set(int64(5), "five")

	keyed := model.NewArray()
	keyed.Set("id", int64(7))
	keyed.Set("name", "html <b>&</b>")
	keyed.Set("tags", list)
	keyed.Set("missing", nil)

	intKeyed := model.NewArray()
	intKeyed.Set(int64(3), "three")
	intKeyed.Set("mixed", true)

	nested := model.NewArray()
	nested.Set("outer", keyed)
	nested.Set("list", []any{list, sparse})

	object := model.NewObject(&model.Class{Name: "Point"})
	object.SetProp("x", 1.5)
	object.SetProp("y", int64(-2))

	return []any{
		model.NewArray(),
		list,
		sparse,
		keyed,
		intKeyed,
		nested,
		object,
		(*model.Array)(nil),
		(*model.Object)(nil),
		[]any{keyed, list},
	}
}

// TestAppendJSONValueMatchesTheConvertedTree holds the collection appender to
// the bytes the jsonEncodeValue tree encodes to.
//
// The appender is what the encoder and json_encode() write through, and the
// tree is what encoding/json walks on the indent path, so the two spellings of
// one document have to agree character for character: the list-or-object
// decision, the order of the keys, an int key's spelling and the escapes.
func TestAppendJSONValueMatchesTheConvertedTree(t *testing.T) {
	t.Parallel()
	for _, value := range phpValues() {
		got, err := appendJSONValue(nil, value)
		if err != nil {
			t.Fatalf("appendJSONValue(%T): %v", value, err)
		}
		want, err := json.Marshal(jsonEncodeValue(value))
		if err != nil {
			t.Fatalf("json.Marshal(jsonEncodeValue(%T)): %v", value, err)
		}
		if string(got) != string(want) {
			t.Errorf("appendJSONValue(%T) = %s, want %s", value, got, want)
		}
	}
}

// rowList builds the shape a JSON API answers with: a list of keyed rows.
func rowList(rows int) *model.Array {
	list := model.NewArraySize(rows)
	for i := range rows {
		row := model.NewArray()
		row.Set("id", int64(i))
		row.Set("name", "a name")
		row.Set("active", true)
		list.Append(row)
	}
	return list
}

// BenchmarkAppendJSONValue measures the walk over a list of keyed rows, which
// is the response shape an API answers with and the one where a per-collection
// cost in the walk is paid once per row and not once per document.
func BenchmarkAppendJSONValue(b *testing.B) {
	list := rowList(64)
	buf := make([]byte, 0, 8192)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var err error
		if buf, err = appendJSONValue(buf[:0], list); err != nil {
			b.Fatal(err)
		}
	}
}
