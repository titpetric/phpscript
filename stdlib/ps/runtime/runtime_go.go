package runtime

import (
	"fmt"
	"io"
	goos "os"
	"reflect"
	goruntime "runtime"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
)

// The Runtime\Go bindings are go's builtins, reached with `use Runtime\Go`.
// A script opts in by importing the namespace. They form a binding
// area no runtime installs by default: `make` and `len` are not php's. A
// program importing neither keeps the surface php defines. The import
// installs them where stdlib.Mount would install an area every runtime holds.
//
// Each binding reads php's values and answers go's semantics. A length is go's,
// over php's arrays as well as go's slices; a conversion applies go's width to
// a value php reduced to one of its scalars; a panic throws a
// RuntimeException, the class a php `catch` can filter on.

func init() {
	runner.RegisterNamespace(modelutil.GoNamespace, RegisterGo)
}

// RegisterGo installs the Runtime\Go bindings on a runtime that imported them.
func RegisterGo(rt *runner.Runtime) {
	// Runtime\Go\make allocates a go slice with a length and capacity: make([]string, 0, 10).
	rt.RegisterFunc(goFunc("make"), goMake)
	// Runtime\Go\append appends values to a php array or a go slice and returns it.
	rt.RegisterFunc(goFunc("append"), goAppend)
	// Runtime\Go\len returns the length of a string, a php array or a go collection.
	rt.RegisterFunc(goFunc("len"), goLen)
	// Runtime\Go\cap returns the entries a go slice holds before it grows.
	rt.RegisterFunc(goFunc("cap"), goCap)
	// Runtime\Go\copy copies elements into $dst and returns how many it copied.
	rt.RegisterFunc(goFunc("copy"), func(dst, src any) (int64, error) {
		if target, ok := dst.(*model.Array); ok {
			return goCopyArray(target, src)
		}
		target := reflect.ValueOf(dst)
		if target.Kind() != reflect.Slice {
			return 0, goFail(`Runtime\Go\copy: cannot copy into a %T`, dst)
		}
		return goCopySlice(target, src)
	})
	// Runtime\Go\delete removes a key from a php array or a go map.
	rt.RegisterFunc(goFunc("delete"), func(collection, key any) error {
		if target, ok := collection.(*model.Array); ok {
			target.Delete(phpval.Key(key))
			return nil
		}
		return goDeleteKey(reflect.ValueOf(collection), key)
	})
	// Runtime\Go\clear empties a php array or a go map, and zeroes a go slice.
	rt.RegisterFunc(goFunc("clear"), goClear)
	// Runtime\Go\min returns the smallest of its arguments.
	rt.RegisterFunc(goFunc("min"), func(args ...any) (any, error) { return goOrdered("min", args, -1) })
	// Runtime\Go\max returns the largest of its arguments.
	rt.RegisterFunc(goFunc("max"), func(args ...any) (any, error) { return goOrdered("max", args, 1) })
	// Runtime\Go\panic throws a RuntimeException holding $value.
	rt.RegisterFunc(goFunc("panic"), func(value any) error {
		return runner.NewRuntimeException("panic: "+phpval.String(value), 0)
	})
	// Runtime\Go\print writes its arguments to the output, as go's print does.
	rt.RegisterFunc(goFunc("print"), func(args ...any) error { return goPrint(rt, args, false) })
	// Runtime\Go\println writes its arguments spaced, with a newline, as go's println does.
	rt.RegisterFunc(goFunc("println"), func(args ...any) error { return goPrint(rt, args, true) })
	// Runtime\Go\getenv returns the value of environment variable $key, or null for a name the process does not hold.
	rt.RegisterFunc(goFunc("getenv"), func(key string) any {
		if value, ok := goos.LookupEnv(key); ok {
			return value
		}
		return nil
	})
	// Runtime\Go\numcpu returns the number of logical CPUs the process can use.
	rt.RegisterFunc(goFunc("numcpu"), func() int64 { return int64(goruntime.NumCPU()) })
	registerGoTypes(rt)
}

// registerGoTypes installs each builtin type's conversion as a function plus a constructor.
func registerGoTypes(rt *runner.Runtime) {
	for _, name := range modelutil.GoTypeNames() {
		// Runtime\Go\<type> converts a value to a go builtin type: int64(0), float64($n).
		convert := goConvert(name)
		rt.RegisterFunc(goFunc(name), convert)
		rt.RegisterConstructor(goFunc(name), convert)
	}
}

// goFunc is the name a Runtime\Go binding registers under.
func goFunc(name string) string { return modelutil.GoNamespace + `\` + name }

// goFail is the RuntimeException a binding throws, visible to a php `catch`.
func goFail(format string, args ...any) error {
	return runner.NewRuntimeException(fmt.Sprintf(format, args...), 0)
}

// goConvert returns a builtin type's conversion, a missing argument as the zero value.
func goConvert(name string) func(args ...any) (any, error) {
	return func(args ...any) (any, error) {
		if len(args) > 1 {
			return nil, goFail(`%s: takes one value and no more`, goFunc(name))
		}
		var value any
		if len(args) == 1 {
			value = args[0]
		}
		reduced, err := goReduce(name, value)
		if err != nil {
			return nil, err
		}
		folded, ok := modelutil.GoFold(name, reduced)
		if !ok {
			return nil, goFail(`%s: cannot convert a %T`, goFunc(name), value)
		}
		return folded, nil
	}
}

// goReduce reads a value through PHP's semantics before GoFold applies the type's width.
func goReduce(name string, value any) (any, error) {
	switch name {
	case "any":
		return value, nil
	case "bool":
		return phpval.Truthy(value), nil
	case "string":
		text, err := phpval.StringContext(value)
		return text, err
	case "float32", "float64":
		return phpval.Float(value), nil
	}
	return phpval.Int(value), nil
}

// goMake allocates a slice of the named type, length and capacity zero.
func goMake(spelling string, size ...any) (any, error) {
	elem, ok := goElem(spelling)
	if !ok {
		return nil, goFail(`Runtime\Go\make: cannot allocate a %s; make takes a slice type, []int64 or [][]string or []MyClass`, spelling)
	}
	if len(size) > 2 {
		return nil, goFail(`Runtime\Go\make: takes a type, a length and a capacity, and no more`)
	}
	length := 0
	if len(size) > 0 {
		length = int(phpval.Int(size[0]))
	}
	capacity := length
	if len(size) > 1 {
		capacity = int(phpval.Int(size[1]))
	}
	if length < 0 || capacity < length {
		return nil, goFail(`Runtime\Go\make: a length of %d and a capacity of %d make no %s`, length, capacity, spelling)
	}
	return reflect.MakeSlice(reflect.SliceOf(elem), length, capacity).Interface(), nil
}

// goElems holds the element types make allocates a slice of, keyed by name.
var goElems = map[string]reflect.Type{
	"any":     reflect.TypeFor[any](),
	"bool":    reflect.TypeFor[bool](),
	"string":  reflect.TypeFor[string](),
	"int":     reflect.TypeFor[int](),
	"int8":    reflect.TypeFor[int8](),
	"int16":   reflect.TypeFor[int16](),
	"int32":   reflect.TypeFor[int32](),
	"rune":    reflect.TypeFor[rune](),
	"int64":   reflect.TypeFor[int64](),
	"uint":    reflect.TypeFor[uint](),
	"uint8":   reflect.TypeFor[uint8](),
	"byte":    reflect.TypeFor[byte](),
	"uint16":  reflect.TypeFor[uint16](),
	"uint32":  reflect.TypeFor[uint32](),
	"uint64":  reflect.TypeFor[uint64](),
	"float32": reflect.TypeFor[float32](),
	"float64": reflect.TypeFor[float64](),
}

// goElem reads a type spelling into its element type, composing a nested slice.
func goElem(spelling string) (reflect.Type, bool) {
	rest, ok := strings.CutPrefix(spelling, "[]")
	if !ok || rest == "" {
		return nil, false
	}
	if nested, ok := goElem(rest); ok {
		return reflect.SliceOf(nested), true
	}
	if elem, known := goElems[rest]; known {
		return elem, true
	}
	if modelutil.GoType(rest) {
		return nil, false
	}
	return goElems["any"], true
}

// goAppend appends values to a PHP array or a Go slice, growing nil into an array.
func goAppend(dst any, values ...any) (any, error) {
	switch target := dst.(type) {
	case nil:
		out := model.NewArraySize(len(values))
		for _, value := range values {
			out.Append(value)
		}
		return out, nil
	case *model.Array:
		for _, value := range values {
			target.Append(value)
		}
		return target, nil
	}
	slice := reflect.ValueOf(dst)
	if slice.Kind() != reflect.Slice {
		return nil, goFail(`Runtime\Go\append: cannot append to a %T`, dst)
	}
	elem := slice.Type().Elem()
	for _, value := range values {
		converted, err := goValue(elem, value)
		if err != nil {
			return nil, err
		}
		slice = reflect.Append(slice, converted)
	}
	return slice.Interface(), nil
}

// goValue converts a PHP value into the shape a Go element type holds.
func goValue(elem reflect.Type, value any) (reflect.Value, error) {
	if elem.Kind() == reflect.Interface && elem.NumMethod() == 0 {
		return reflect.ValueOf(&value).Elem(), nil
	}
	switch elem.Kind() {
	case reflect.Bool:
		return reflect.ValueOf(phpval.Truthy(value)).Convert(elem), nil
	case reflect.String:
		text, err := phpval.StringContext(value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(text).Convert(elem), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflect.ValueOf(phpval.Int(value)).Convert(elem), nil
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(phpval.Float(value)).Convert(elem), nil
	}
	held := reflect.ValueOf(value)
	if held.IsValid() && held.Type().AssignableTo(elem) {
		return held, nil
	}
	return reflect.Value{}, goFail(`Runtime\Go: a %T does not go in a %s`, value, elem)
}

// goLen returns go's len for a string, PHP array or Go collection, with nil as empty.
func goLen(value any) (int64, error) {
	switch text := value.(type) {
	case nil:
		return 0, nil
	case string:
		return int64(len(text)), nil
	}
	if text, ok := phpval.Bytes(value); ok {
		return int64(len(text)), nil
	}
	if length, ok := modelutil.LenValues(value); ok {
		return int64(length), nil
	}
	return 0, goFail(`Runtime\Go\len: a %T has no length`, value)
}

// goCap returns go's cap, with a PHP array answering its length.
func goCap(value any) (int64, error) {
	if value == nil {
		return 0, nil
	}
	held := reflect.ValueOf(value)
	switch held.Kind() {
	case reflect.Slice, reflect.Array, reflect.Chan:
		return int64(held.Cap()), nil
	}
	if length, ok := modelutil.LenValues(value); ok {
		return int64(length), nil
	}
	return 0, goFail(`Runtime\Go\cap: a %T has no capacity`, value)
}

// goCopyArray copies src's values over dst's leading indexes and returns the count.
func goCopyArray(dst *model.Array, src any) (int64, error) {
	limit, copied := int64(dst.Len()), int64(0)
	modelutil.RangeValues(src, func(_, value any) bool {
		if copied >= limit {
			return false
		}
		dst.Set(copied, value)
		copied++
		return true
	})
	return copied, nil
}

// goCopySlice copies src into dst's elements, stopping at the shorter length.
func goCopySlice(dst reflect.Value, src any) (int64, error) {
	limit, copied := int64(dst.Len()), int64(0)
	elem := dst.Type().Elem()
	var failure error
	modelutil.RangeValues(src, func(_, value any) bool {
		if copied >= limit {
			return false
		}
		converted, err := goValue(elem, value)
		if err != nil {
			failure = err
			return false
		}
		dst.Index(int(copied)).Set(converted)
		copied++
		return true
	})
	if failure != nil {
		return 0, failure
	}
	return copied, nil
}

// goDeleteKey removes a key from a Go map, as PHP array narrowing does.
func goDeleteKey(collection reflect.Value, key any) error {
	if collection.Kind() != reflect.Map {
		return goFail(`Runtime\Go\delete: cannot delete from a %s`, collection.Type())
	}
	converted, err := goValue(collection.Type().Key(), key)
	if err != nil {
		return err
	}
	collection.SetMapIndex(converted, reflect.Value{})
	return nil
}

// goClear clears a map or PHP array and zeroes a slice in place.
func goClear(collection any) error {
	if target, ok := collection.(*model.Array); ok {
		target.Clear()
		return nil
	}
	target := reflect.ValueOf(collection)
	switch target.Kind() {
	case reflect.Map:
		target.Clear()
		return nil
	case reflect.Slice:
		zero := reflect.Zero(target.Type().Elem())
		for i := 0; i < target.Len(); i++ {
			target.Index(i).Set(zero)
		}
		return nil
	}
	return goFail(`Runtime\Go\clear: cannot clear a %T`, collection)
}

// goOrdered is go's min and max over values; it does not read a list the way PHP's min does.
func goOrdered(name string, args []any, sign int) (any, error) {
	if len(args) == 0 {
		return nil, goFail(`%s: takes at least one value`, goFunc(name))
	}
	best := args[0]
	for _, value := range args[1:] {
		if phpval.Compare(value, best) == sign {
			best = value
		}
	}
	return best, nil
}

// goPrint writes go's print and println output to the script's echo stream.
func goPrint(rt *runner.Runtime, args []any, newline bool) error {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, phpval.String(arg))
	}
	separator, suffix := "", ""
	if newline {
		separator, suffix = " ", "\n"
	}
	_, err := io.WriteString(rt.Output(), strings.Join(parts, separator)+suffix)
	return err
}
