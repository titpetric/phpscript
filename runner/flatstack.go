package runner

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	flatvm "github.com/titpetric/phpscript/flatstack/engine"
	"github.com/titpetric/phpscript/model"
)

func (rt *Runtime) runFlat(ast *model.Program) (bool, error) {
	if rt.errorHandler != nil {
		// The interpreter can recover per statement through OnError. The flat
		// backend currently returns the first execution error.
		return false, nil
	}
	program, ok := rt.exprCache.getFlat(ast)
	if !ok {
		var err error
		program, err = flatvm.Compile(ast)
		if err != nil {
			return false, nil
		}
		rt.exprCache.setFlat(ast, program)
	}
	return true, flatvm.Run(program, flatHost{runtime: rt})
}

type flatHost struct {
	runtime *Runtime
}

func (h flatHost) Construct(class string, args []any) (any, error) {
	return h.runtime.helperNew(&scopeRef{scope: NewScope()})(strings.TrimPrefix(class, "\\"), args...)
}

func (h flatHost) CallMethod(receiver any, method string, args []any) (any, error) {
	return h.runtime.helperCall(&scopeRef{scope: NewScope()})(receiver, method, args...)
}

func (h flatHost) GetProperty(receiver any, name string) any {
	return h.runtime.helperGet(&scopeRef{})(receiver, name)
}

// SetProperty writes an object property, the same way the interpreter does:
// a PHP object carries the value in its property map, and a Go binding assigns
// the struct field the name resolves to — `$db->is_readonly = true` sets
// IsReadonly.
func (h flatHost) SetProperty(receiver any, name string, value any, op string) error {
	if object, ok := receiver.(*model.Object); ok {
		object.Props[name] = applyAssignOp(op, object.Props[name], value)
		return nil
	}
	return assignGoField(receiver, name, func(current any) any {
		return applyAssignOp(op, current, value)
	})
}

func (h flatHost) Echo(value any) error {
	_, err := io.WriteString(h.runtime.out, phpString(value))
	return err
}

func (h flatHost) Lookup(name string) any {
	if value, ok := h.runtime.globals[name]; ok {
		return value
	}
	return h.runtime.constants[name]
}

func (h flatHost) Array(items []model.ArrayItemValue) any { return helperArray(items...) }

func (h flatHost) Index(base, index any) any { return helperIndex(base, index) }

func (h flatHost) Truthy(value any) bool { return phpTruthy(value) }

func (h flatHost) SetIndex(base, key, value any, appendValue bool, op string) error {
	array, ok := base.(*model.Array)
	if !ok {
		return fmt.Errorf("assign: target is not an array")
	}
	if appendValue {
		array.Append(value)
		return nil
	}
	key = normalizeKey(key)
	if op != "" && op != "=" {
		current, _ := array.Get(key)
		binaryOp := strings.TrimSuffix(op, "=")
		var err error
		value, err = h.Binary(binaryOp, current, value)
		if err != nil {
			return err
		}
	}
	array.Set(key, value)
	return nil
}

func (h flatHost) Binary(op string, left, right any) (any, error) {
	switch op {
	case ".":
		return helperConcat(left, right), nil
	case "+", "-", "*", "/", "%":
		return phpArith(op, left, right), nil
	case "==":
		return phpLooseEqual(left, right), nil
	case "!=":
		return !phpLooseEqual(left, right), nil
	case "===", "!==":
		equal := reflect.TypeOf(left) == reflect.TypeOf(right) && reflect.DeepEqual(left, right)
		if op == "!==" {
			equal = !equal
		}
		return equal, nil
	case "&&":
		return phpTruthy(left) && phpTruthy(right), nil
	case "||":
		return phpTruthy(left) || phpTruthy(right), nil
	case "<", "<=", ">", ">=":
		var cmp int
		if isNumeric(left) && isNumeric(right) {
			a, b := toFloat(left), toFloat(right)
			if a < b {
				cmp = -1
			} else if a > b {
				cmp = 1
			}
		} else {
			cmp = strings.Compare(phpString(left), phpString(right))
		}
		switch op {
		case "<":
			return cmp < 0, nil
		case "<=":
			return cmp <= 0, nil
		case ">":
			return cmp > 0, nil
		default:
			return cmp >= 0, nil
		}
	default:
		return nil, fmt.Errorf("unsupported binary operator %q", op)
	}
}

func (h flatHost) Unary(op string, value any) (any, error) {
	switch op {
	case "!":
		return !phpTruthy(value), nil
	case "+":
		return phpArith("+", int64(0), value), nil
	case "-":
		return phpArith("-", int64(0), value), nil
	default:
		return nil, fmt.Errorf("unsupported unary operator %q", op)
	}
}

func (h flatHost) Entries(value any) []flatvm.Entry {
	var entries []flatvm.Entry
	if array, ok := value.(*model.Array); ok {
		array.Range(func(key, value any) bool {
			entries = append(entries, flatvm.Entry{Key: key, Value: value})
			return true
		})
		return entries
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			entries = append(entries, flatvm.Entry{Key: int64(i), Value: rv.Index(i).Interface()})
		}
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			entries = append(entries, flatvm.Entry{Key: key.Interface(), Value: rv.MapIndex(key).Interface()})
		}
	}
	return entries
}

func (h flatHost) Call(name, fallback string, args []any) (any, error) {
	return h.runtime.helperFunc(&scopeRef{scope: NewScope()})(name, fallback, args...)
}
