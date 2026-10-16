package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes json_encode and json_decode to stdlib.Register.
func init() {
	runner.RegisterBinding(registerJSON)
}

func registerJSON(rt *runner.Runtime) {
	// json_encode returns the JSON encoding of $value. $flags is accepted and
	// ignored because the encoding is not configurable, a forward slash is
	// written as itself, and an encoding failure raises an error where php
	// returns false.
	rt.RegisterFunc("json_encode", phpJSONEncode)
	// json_decode parses the JSON in $text. $associative must be true or
	// omitted because decoding into objects is not implemented, $depth and
	// $flags are accepted and ignored, and invalid input raises an error where
	// php returns null.
	rt.RegisterFunc("json_decode", phpJSONDecode)

	// The streaming pair. json_encode() and json_decode() work on a whole
	// string, which means holding the whole document; these work on a stream,
	// as a request body and a response are.
	// `new JSON\Decoder(fopen("php://input", "r"))` reads a POST body without a
	// string of it existing first, and an encoder over php://output writes the
	// response as it is built. Both take the io.Reader or io.Writer they wrap,
	// so any handle fopen() returns works, and so do STDIN and STDOUT.
	rt.RegisterConstructor("JSON\\Encoder", NewJSONEncoder)
	rt.RegisterConstructor("JSON\\Decoder", NewJSONDecoder)
}

// phpJSONEncode encodes value. $flags is accepted and ignored, the way
// json_decode accepts $depth and $flags, so a port carrying
// JSON_UNESCAPED_SLASHES runs: no JSON_* constant is defined, the argument
// arrives as null, and there is nothing for it to select. The encoding is not
// configurable by design; see docs/design.md, "JSON".
func phpJSONEncode(value any, flags ...any) (any, error) {
	buf, err := appendJSONValue(make([]byte, 0, 64), value)
	if err != nil {
		return nil, err
	}
	return string(buf), nil
}

func phpJSONDecode(text string, opts ...any) (any, error) {
	// $associative selects the shape objects decode into. The decoder only
	// builds arrays, so false is refused and never answered with the shape it
	// did not name; $depth and $flags are accepted and ignored. stdClass
	// exists and `(object)` builds one, so what is missing is the decode path,
	// not the class; see docs/design.md, "JSON".
	if len(opts) > 0 && opts[0] != nil && !phpval.Truthy(opts[0]) {
		return nil, errors.New("json_decode(): $associative must be true; decoding into objects is not implemented")
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	value, err := jsonDecodeStream(dec)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// jsonDecodeStream reads one value from the token stream.
//
// It reads tokens and decodes into no map, because a Go map has no
// order and the decoder would return the document's keys in a different
// order on every run. PHP preserves the document's own order, so a
// decoded object encoded again reads the way it arrived.
func jsonDecodeStream(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return jsonDecodeValue(token), nil
	}

	switch delim {
	case '{':
		out := model.NewArray()
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			value, err := jsonDecodeStream(dec)
			if err != nil {
				return nil, err
			}
			name, _ := key.(string)
			out.Set(name, value)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return out, nil
	case '[':
		out := []any{}
		for dec.More() {
			value, err := jsonDecodeStream(dec)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, fmt.Errorf("json_decode(): unexpected %v", delim)
}

// jsonEncodeValue rewrites the value model into something encoding/json
// understands. Native Go collections are already encodable, so they are only
// walked when they might contain an *model.Array or *model.Object; scalars and
// []string pass through untouched.
func jsonEncodeValue(v any) any {
	switch x := v.(type) {
	case nil, string, bool, int64, int, float64, []string:
		return v
	case *model.Array:
		if x == nil {
			return nil
		}
		if arrayIsList(x) {
			out := make([]any, 0, x.Len())
			x.Range(func(_, v any) bool {
				out = append(out, jsonEncodeValue(v))
				return true
			})
			return out
		}
		out := newJSONObject(x.Len())
		x.Range(func(k, v any) bool {
			out.add(phpval.String(k), jsonEncodeValue(v))
			return true
		})
		return out
	case *model.Object:
		if x == nil {
			return nil
		}
		out := newJSONObject(x.Len())
		x.Range(func(name string, v any) bool {
			out.add(name, jsonEncodeValue(v))
			return true
		})
		return out
	case model.Collection:
		// A view over request data, which is keyed by name and so encodes as
		// an object. Reading it is what decodes the request behind it.
		if x == nil {
			return nil
		}
		out := newJSONObject(x.Len())
		x.Range(func(k, v any) bool {
			out.add(phpval.String(k), jsonEncodeValue(v))
			return true
		})
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = jsonEncodeValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = jsonEncodeValue(item)
		}
		return out
	default:
		return v
	}
}

// arrayIsList reports whether an *model.Array's keys are the dense int64
// sequence 0..n-1, i.e. whether it is a PHP list.
func arrayIsList(a *model.Array) bool {
	expect := int64(0)
	isList := true
	a.Range(func(k, _ any) bool {
		if i, ok := k.(int64); !ok || i != expect {
			isList = false
			return false
		}
		expect++
		return true
	})
	return isList
}

func jsonDecodeValue(v any) any {
	switch x := v.(type) {
	case []any:
		// A JSON array is positional, so a []any loses nothing and reuses the
		// slice the decoder already allocated.
		for i, item := range x {
			x[i] = jsonDecodeValue(item)
		}
		return x
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return string(x)
	default:
		return v
	}
}

// JSONEncoder writes JSON values to a stream. It is Go's json.Encoder with the
// one difference PHP forces: the value it is given is a PHP value, so it is
// converted the way json_encode() converts one, and the two spellings answer
// the same JSON for the same array.
//
// A plain encoder appends the document into buf and writes it in one call. The
// json.Encoder underneath is built by SetIndent alone, because indenting is
// encoding/json's own pass over finished bytes and this writes none it could
// read; an encoder nothing indents never builds one.
type JSONEncoder struct {
	stream io.Writer
	buf    []byte
	enc    *json.Encoder
}

// jsonEncoderBuffer is how much of its buffer an encoder keeps between values.
// One response's worth is kept and reused; a large document's is released, so a
// handle held open for the length of a script does not hold that peak with it.
const jsonEncoderBuffer = 1 << 16

// NewJSONEncoder returns an encoder writing to $stream.
func NewJSONEncoder(stream io.Writer) (*JSONEncoder, error) {
	if stream == nil {
		return nil, errNilStream("JSON\\Encoder", "writer")
	}
	return &JSONEncoder{stream: stream}, nil
}

// encode writes $value to the stream as JSON, followed by a newline.
//
// The newline is Go's, not php's: an Encoder writes a stream of values, and the
// newline is what separates one from the next. A single value followed by a
// newline is still valid JSON to any reader.
func (e *JSONEncoder) Encode(value any) error {
	if e.enc != nil {
		return e.enc.Encode(jsonEncodeValue(value))
	}
	buf, err := appendJSONValue(e.buf[:0], value)
	if err != nil {
		return err
	}
	e.buf = append(buf, '\n')
	_, err = e.stream.Write(e.buf)
	if cap(e.buf) > jsonEncoderBuffer {
		e.buf = nil
	}
	return err
}

// SetIndent is json.Encoder.SetIndent: each value is written across several
// lines, $indent per level under $prefix, and two empty strings go back to one
// line per value.
func (e *JSONEncoder) SetIndent(prefix, indent string) {
	if e.enc == nil {
		e.enc = json.NewEncoder(e.stream)
		e.buf = nil
	}
	e.enc.SetIndent(prefix, indent)
}

// JSONDecoder reads JSON values from a stream, one call per value.
type JSONDecoder struct {
	dec *json.Decoder
}

// NewJSONDecoder returns a decoder reading from $stream.
//
// UseNumber is on, as it is in json_decode(): without it Go reads every number
// as a float, and the 7 in a document would come back as 7.0. With it a whole
// number is an int and only a fractional one is a float, as php
// answers and what a row written back out has to preserve.
func NewJSONDecoder(stream io.Reader) (*JSONDecoder, error) {
	if stream == nil {
		return nil, errNilStream("JSON\\Decoder", "reader")
	}
	dec := json.NewDecoder(stream)
	dec.UseNumber()
	return &JSONDecoder{dec: dec}, nil
}

// Decode reads the next value from the stream and returns it, or throws at the
// end of the stream - a null is a value JSON carries, so `while ($d->more())`
// is the loop and no test against what this returned.
//
// It decodes through json_decode()'s own walk and not json.Decoder.Decode,
// which would build a map and lose the key order the two spellings agree on.
func (d *JSONDecoder) Decode() (any, error) {
	return jsonDecodeStream(d.dec)
}

// More is json.Decoder.More: another value is waiting in the stream. It is what
// ends a decode loop.
func (d *JSONDecoder) More() bool {
	return d.dec.More()
}

// errNilStream phrases the one refusal both constructors make. A stream is not
// optional: fopen() answers false for a file it cannot open, and false is not a
// stream.
func errNilStream(class, kind string) error {
	return &jsonStreamError{class: class, kind: kind}
}

type jsonStreamError struct {
	class string
	kind  string
}

func (e *jsonStreamError) Error() string {
	return e.class + ": argument #1 ($stream) must be a " + e.kind + ", none given"
}
