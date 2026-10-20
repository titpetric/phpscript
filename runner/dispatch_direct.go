package runner

import (
	"io"
	nethttp "net/http"
	"net/url"
)

// This file is the typed bridge in front of reflect.Value.Call for the Go
// methods a handler touches per request: the response writer, the request,
// url.Values, http.Header and the JSON encoder. reflect.Value.Call allocates
// the receiver box, the argument frame and the result slice on every call,
// and together those are the largest allocation site a request-path profile
// reports. An interface assertion dispatches the same method with none of the
// three.
//
// Each case is one method name with one signature, because a Go interface
// encodes both. The assertion runs per call and the runtime caches the itab
// lookup, so the per-call cost is a type check. A receiver or an
// argument shape a case does not cover answers handled=false, which sends the
// call down the planned reflect path with its coercion and its errors intact;
// nothing here may answer differently than that path would.

// directGoMethod returns the typed call for a resolved method, nil when no
// shape matches. base is the receiver the method resolved on, so a cache
// entry only carries a direct call its own receiver type answers.
func directGoMethod(base any, goName string) func(any, []any) (any, bool, error) {
	switch goName {
	case "Header":
		if _, ok := base.(interface{ Header() nethttp.Header }); ok {
			return func(recv any, args []any) (any, bool, error) {
				if len(args) > 0 {
					return nil, true, &ArgumentCountError{Want: 0, Got: len(args)}
				}
				return recv.(interface{ Header() nethttp.Header }).Header(), true, nil
			}
		}
	case "Write":
		if _, ok := base.(io.Writer); ok {
			return func(recv any, args []any) (any, bool, error) {
				if len(args) > 1 {
					return nil, true, &ArgumentCountError{Want: 1, Got: len(args)}
				}
				// A string takes io.StringWriter where the writer has one, so
				// $w->write($s) pays no clone; the []byte conversion is what
				// the reflect path's coercion makes anyway. Any other
				// argument keeps the reflect path's coercion and errors.
				switch v := argAt(args, 0).(type) {
				case string:
					if sw, ok := recv.(io.StringWriter); ok {
						n, err := sw.WriteString(v)
						return n, true, err
					}
					n, err := recv.(io.Writer).Write([]byte(v))
					return n, true, err
				case []byte:
					n, err := recv.(io.Writer).Write(v)
					return n, true, err
				}
				return nil, false, nil
			}
		}
	case "WriteHeader":
		if _, ok := base.(interface{ WriteHeader(int) }); ok {
			return func(recv any, args []any) (any, bool, error) {
				if len(args) > 1 {
					return nil, true, &ArgumentCountError{Want: 1, Got: len(args)}
				}
				if status, ok := argAt(args, 0).(int64); ok {
					recv.(interface{ WriteHeader(int) }).WriteHeader(int(status))
					return nil, true, nil
				}
				return nil, false, nil
			}
		}
	case "PathValue":
		if _, ok := base.(interface{ PathValue(string) string }); ok {
			return directStringString(func(recv any, name string) string {
				return recv.(interface{ PathValue(string) string }).PathValue(name)
			})
		}
	case "FormValue":
		if _, ok := base.(interface{ FormValue(string) string }); ok {
			return directStringString(func(recv any, name string) string {
				return recv.(interface{ FormValue(string) string }).FormValue(name)
			})
		}
	case "Get":
		if _, ok := base.(interface{ Get(string) string }); ok {
			return directStringString(func(recv any, name string) string {
				return recv.(interface{ Get(string) string }).Get(name)
			})
		}
	case "UserAgent":
		if _, ok := base.(interface{ UserAgent() string }); ok {
			return func(recv any, args []any) (any, bool, error) {
				if len(args) > 0 {
					return nil, true, &ArgumentCountError{Want: 0, Got: len(args)}
				}
				return recv.(interface{ UserAgent() string }).UserAgent(), true, nil
			}
		}
	case "Query":
		if _, ok := base.(interface{ Query() url.Values }); ok {
			return func(recv any, args []any) (any, bool, error) {
				if len(args) > 0 {
					return nil, true, &ArgumentCountError{Want: 0, Got: len(args)}
				}
				return recv.(interface{ Query() url.Values }).Query(), true, nil
			}
		}
	case "Set":
		if _, ok := base.(interface{ Set(string, string) }); ok {
			return func(recv any, args []any) (any, bool, error) {
				if len(args) > 2 {
					return nil, true, &ArgumentCountError{Want: 2, Got: len(args)}
				}
				recv.(interface{ Set(string, string) }).Set(phpString(argAt(args, 0)), phpString(argAt(args, 1)))
				return nil, true, nil
			}
		}
	case "Add":
		if _, ok := base.(interface{ Add(string, string) }); ok {
			return func(recv any, args []any) (any, bool, error) {
				if len(args) > 2 {
					return nil, true, &ArgumentCountError{Want: 2, Got: len(args)}
				}
				recv.(interface{ Add(string, string) }).Add(phpString(argAt(args, 0)), phpString(argAt(args, 1)))
				return nil, true, nil
			}
		}
	case "Encode":
		if _, ok := base.(interface{ Encode(any) error }); ok {
			return func(recv any, args []any) (any, bool, error) {
				if len(args) > 1 {
					return nil, true, &ArgumentCountError{Want: 1, Got: len(args)}
				}
				return nil, true, recv.(interface{ Encode(any) error }).Encode(argAt(args, 0))
			}
		}
	}
	return nil
}

// directStringString wraps the one-string-in, string-out shape three of the
// cases share. The argument coerces through phpString, matching the reflect
// path's coercion of a string parameter, nil included.
func directStringString(call func(any, string) string) func(any, []any) (any, bool, error) {
	return func(recv any, args []any) (any, bool, error) {
		if len(args) > 1 {
			return nil, true, &ArgumentCountError{Want: 1, Got: len(args)}
		}
		return call(recv, phpString(argAt(args, 0))), true, nil
	}
}
