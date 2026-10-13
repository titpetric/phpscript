package core_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/core"
)

func runPHP(t *testing.T, src string) string {
	t.Helper()
	program, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// TestNoJSONFlagConstants is the executable form of a design decision: this
// runtime has one JSON encoding and no flags to vary it. If this test fails,
// read docs/design.md before changing it.
//
// The flags exist in PHP because its encoder makes choices a caller then has
// to undo. Escaping a forward slash is the clearest: PHP writes "one\/two",
// which is legal JSON that no other language emits, and JSON_UNESCAPED_SLASHES
// exists to turn it off. Go's encoder does not escape it, so there is nothing
// to turn off and the flag has no work to do here.
//
// The rest are presentation. A consumer needing indented JSON runs it
// through jq or its own pretty printer; a producer that indents its output is
// deciding for a reader it cannot see.
func TestNoJSONFlagConstants(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	for name := range rt.DefinedConstants() {
		if strings.HasPrefix(name, "JSON_") {
			t.Fatalf("%s is registered: json_encode has one encoding and no flags. See docs/design.md.", name)
		}
	}
}

// $flags is accepted and ignored, as json_decode accepts $depth and $flags, so
// a literal one changes nothing.
func TestJSONEncodeIgnoresFlags(t *testing.T) {
	tests := []string{
		`json_encode(array("a" => 1))`,
		`json_encode(array("a" => 1), 128)`,
		`json_encode(array("a" => 1), 0)`,
	}
	for _, call := range tests {
		t.Run(call, func(t *testing.T) {
			if got := runPHP(t, "<?php echo "+call+";"); got != `{"a":1}` {
				t.Fatalf("got %q, want %q", got, `{"a":1}`)
			}
		})
	}
}

// A JSON_* name is not defined, so it raises before json_encode is called.
// phpscript lint reports it first: the argument was never going to do
// anything.
func TestJSONFlagConstantThrows(t *testing.T) {
	got := runPHP(t, `<?php try { echo json_encode(array(1), JSON_PRETTY_PRINT); } catch (Exception $e) { echo $e->getMessage(); }`)
	if want := `Undefined constant "JSON_PRETTY_PRINT"`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A forward slash is written as itself, which is Go's behaviour and legal
// JSON. php escapes it unless JSON_UNESCAPED_SLASHES is passed.
func TestJSONEncodeDoesNotEscapeSlashes(t *testing.T) {
	got := runPHP(t, `<?php echo json_encode(array("path" => "a/b"));`)
	if want := `{"path":"a\/b"}`; got == want {
		t.Fatalf("got php's escaped form %q; this runtime writes the slash as itself", got)
	}
	if want := `{"path":"a/b"}`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestJSONEncoder covers the one difference between this encoder and Go's: the
// value handed to it is a PHP value, so a *model.Array converts the way
// json_encode() converts one. A stream is required, because fopen() answers
// false for a file it could not open and false is no stream.
func TestJSONEncoder(t *testing.T) {
	var out bytes.Buffer
	enc, err := core.NewJSONEncoder(&out)
	if err != nil {
		t.Fatalf("NewJSONEncoder: %v", err)
	}

	list := model.NewArray()
	list.Append(int64(1))
	list.Append("two")
	if err := enc.Encode(list); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got, want := out.String(), "[1,\"two\"]\n"; got != want {
		t.Errorf("a list array encoded to %q, want %q", got, want)
	}

	if _, err := core.NewJSONEncoder(nil); err == nil {
		t.Error("NewJSONEncoder(nil) built an encoder over no stream")
	}
}

// TestJSONEncoder_Encode covers the key order a PHP array keeps through the
// stream. encoding/json sorts a Go map's keys; an array is written in the order
// the script set them, so a row encoded for a client reads back the way it was
// assembled. Every value ends with a newline, separating one from the next.
func TestJSONEncoder_Encode(t *testing.T) {
	var out bytes.Buffer
	enc, err := core.NewJSONEncoder(&out)
	if err != nil {
		t.Fatalf("NewJSONEncoder: %v", err)
	}

	row := model.NewArray()
	row.Set("zebra", int64(1))
	row.Set("apple", int64(2))
	if err := enc.Encode(row); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if err := enc.Encode("tail"); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := "{\"zebra\":1,\"apple\":2}\n\"tail\"\n"
	if got := out.String(); got != want {
		t.Errorf("the stream reads %q, want %q", got, want)
	}
}

// TestJSONEncoder_SetIndent covers json.Encoder.SetIndent reaching a script:
// each value is written across several lines, $indent per level under $prefix.
// Two empty strings go back to one line per value.
func TestJSONEncoder_SetIndent(t *testing.T) {
	var out bytes.Buffer
	enc, err := core.NewJSONEncoder(&out)
	if err != nil {
		t.Fatalf("NewJSONEncoder: %v", err)
	}

	row := model.NewArray()
	row.Set("a", int64(1))

	enc.SetIndent("", "  ")
	if err := enc.Encode(row); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got, want := out.String(), "{\n  \"a\": 1\n}\n"; got != want {
		t.Errorf("the indented form is %q, want %q", got, want)
	}

	out.Reset()
	enc.SetIndent("", "")
	if err := enc.Encode(row); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got, want := out.String(), "{\"a\":1}\n"; got != want {
		t.Errorf("two empty strings left %q, want %q", got, want)
	}
}

// TestJSONDecoder covers UseNumber being on, as it is in json_decode(): a whole
// number decodes to an int and a fractional one to a float, so the 7 in a
// document does not come back as 7.0.
func TestJSONDecoder(t *testing.T) {
	dec, err := core.NewJSONDecoder(strings.NewReader(`[7, 7.5, "x", true, null]`))
	if err != nil {
		t.Fatalf("NewJSONDecoder: %v", err)
	}

	value, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("a JSON array decoded to %T, want []any", value)
	}
	want := []any{int64(7), 7.5, "x", true, nil}
	if len(items) != len(want) {
		t.Fatalf("decoded %d items, want %d", len(items), len(want))
	}
	for i, value := range want {
		if items[i] != value {
			t.Errorf("item %d is %#v, want %#v", i, items[i], value)
		}
	}

	if _, err := core.NewJSONDecoder(nil); err == nil {
		t.Error("NewJSONDecoder(nil) built a decoder over no stream")
	}
}

// TestJSONDecoder_Decode covers the shape an object decodes into: a
// *model.Array carrying the document's own key order. json.Decoder.Decode would
// build a Go map and lose it.
func TestJSONDecoder_Decode(t *testing.T) {
	dec, err := core.NewJSONDecoder(strings.NewReader(`{"zebra":1,"apple":{"deep":2}}`))
	if err != nil {
		t.Fatalf("NewJSONDecoder: %v", err)
	}

	value, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	array, ok := value.(*model.Array)
	if !ok {
		t.Fatalf("a JSON object decoded to %T, want *model.Array", value)
	}
	if got, want := array.Keys(), []any{"zebra", "apple"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the keys decoded as %v, want %v", got, want)
	}

	nested, _ := array.Get("apple")
	inner, ok := nested.(*model.Array)
	if !ok {
		t.Fatalf("the nested object decoded to %T, want *model.Array", nested)
	}
	if got, _ := inner.Get("deep"); got != int64(2) {
		t.Errorf("the nested value is %#v, want int64(2)", got)
	}
}

// TestJSONDecoder_More covers what ends a decode loop. A null is a value JSON
// carries, so a loop testing what Decode answered would stop one value early;
// More reads the stream and not the value.
func TestJSONDecoder_More(t *testing.T) {
	dec, err := core.NewJSONDecoder(strings.NewReader("null\n{\"a\":1}\n"))
	if err != nil {
		t.Fatalf("NewJSONDecoder: %v", err)
	}

	var decoded int
	for dec.More() {
		if _, err := dec.Decode(); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		decoded++
	}
	if decoded != 2 {
		t.Errorf("the loop read %d values, want 2", decoded)
	}
	if dec.More() {
		t.Error("More() stayed true at the end of the stream")
	}
}
