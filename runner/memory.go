package runner

import (
	"reflect"
	"unsafe"

	"github.com/titpetric/phpscript/model"
)

// runtimeBaseline is the baseline memory overhead of the Runtime struct before script execution.
var runtimeBaseline = int64(unsafe.Sizeof(Runtime{}))

// Memory-limit checkpoint intervals. A checkpoint walks every live value, so
// it cannot run per step; statements are far heavier than VM instructions,
// hence the distinct intervals. Both apply only when a MemoryLimit is set.
const (
	memCheckStatements   = 256
	memCheckInstructions = 4096
)

// RuntimeException represents a PHP RuntimeException raised by the runner.
type RuntimeException struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e *RuntimeException) Error() string {
	return e.Message
}

func (e *RuntimeException) GetMessage() string {
	return e.Message
}

func (e *RuntimeException) GetCode() int {
	return e.Code
}

// ThrowableClass names the PHP class, implementing Throwable, so a catch
// filters on it the way it filters on one a script constructed.
func (e *RuntimeException) ThrowableClass() string { return "RuntimeException" }

// NewRuntimeException creates a new RuntimeException instance.
func NewRuntimeException(message string, code int) *RuntimeException {
	return &RuntimeException{
		Message: message,
		Code:    code,
	}
}

// visitedSet dedups containers by pointer identity during a deep walk, so an
// array or object reachable through several variables is counted once and
// self-referential containers terminate.
type visitedSet map[any]struct{}

// DeepSize estimates the bytes held by a PHP value, recursing into
// script-owned containers. A container already in visited costs nothing.
// Native Go values returned by bindings are host-owned and get a shallow
// size, the same ownership rule flatHost.SetEntry applies.
func DeepSize(v any, visited visitedSet) int64 {
	switch x := v.(type) {
	case *model.Array:
		if x == nil {
			return 0
		}
		if _, seen := visited[x]; seen {
			return 0
		}
		visited[x] = struct{}{}
		total := int64(64)
		x.Range(func(key, val any) bool {
			total += DeepSize(key, visited) + DeepSize(val, visited)
			return true
		})
		return total
	case *model.Object:
		if x == nil {
			return 0
		}
		if _, seen := visited[x]; seen {
			return 0
		}
		visited[x] = struct{}{}
		total := int64(64)
		for k, val := range x.Props {
			total += 16 + int64(len(k)) + DeepSize(val, visited)
		}
		return total
	case []any:
		total := int64(24)
		for _, item := range x {
			total += DeepSize(item, visited)
		}
		return total
	case map[string]any:
		total := int64(48)
		for k, val := range x {
			total += 16 + int64(len(k)) + DeepSize(val, visited)
		}
		return total
	case map[any]any:
		total := int64(48)
		for k, val := range x {
			total += DeepSize(k, visited) + DeepSize(val, visited)
		}
		return total
	case map[string]string:
		total := int64(48)
		for k, val := range x {
			total += 32 + int64(len(k)) + int64(len(val))
		}
		return total
	case map[string][]string:
		total := int64(48)
		for k, vals := range x {
			total += 16 + int64(len(k)) + 24
			for _, val := range vals {
				total += 16 + int64(len(val))
			}
		}
		return total
	case Context:
		return x.memoryFootprint(visited)
	default:
		return EstimateValueSize(v)
	}
}

// EstimateValueSize calculates a shallow estimate of memory consumed by a PHP/Go value.
// It is closed and non-recursive:
// - scalars / strings: direct size + string length
// - byte slices: slice header + length
// - arrays: header + keys + shallow one-level elements
// - objects: header + property names + shallow one-level properties
// - pointers/structs: shallow sizeof
func EstimateValueSize(v any) int64 {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case bool:
		return 1
	case int, int64, uint, uint64, float64:
		return 8
	case int32, uint32, float32:
		return 4
	case int16, uint16:
		return 2
	case int8, uint8:
		return 1
	case string:
		return 16 + int64(len(x))
	case []byte:
		return 24 + int64(len(x))
	case []string:
		total := int64(24)
		for _, s := range x {
			total += 16 + int64(len(s))
		}
		return total
	case []any:
		total := int64(24)
		for _, item := range x {
			total += shallowElementSize(item)
		}
		return total
	case *model.Array:
		if x == nil {
			return 0
		}
		total := int64(64)
		x.Range(func(key, val any) bool {
			total += shallowElementSize(key) + shallowElementSize(val)
			return true
		})
		return total
	case *model.Object:
		if x == nil {
			return 0
		}
		total := int64(64)
		for k, val := range x.Props {
			total += 16 + int64(len(k)) + shallowElementSize(val)
		}
		return total
	case map[string]any:
		total := int64(48)
		for k, val := range x {
			total += 16 + int64(len(k)) + shallowElementSize(val)
		}
		return total
	case map[any]any:
		total := int64(48)
		for k, val := range x {
			total += shallowElementSize(k) + shallowElementSize(val)
		}
		return total
	default:
		t := reflect.TypeOf(v)
		if t != nil {
			if t.Kind() == reflect.Pointer {
				return int64(t.Size()) + int64(t.Elem().Size())
			}
			return int64(t.Size())
		}
		return 16
	}
}

func shallowElementSize(v any) int64 {
	if v == nil {
		return 8
	}
	switch val := v.(type) {
	case bool:
		return 1
	case int, int64, uint, uint64, float64:
		return 8
	case int32, uint32, float32:
		return 4
	case int16, uint16:
		return 2
	case int8, uint8:
		return 1
	case string:
		return 16 + int64(len(val))
	case []byte:
		return 24 + int64(len(val))
	case *model.Array:
		return 64
	case *model.Object:
		return 64
	default:
		return 16
	}
}
