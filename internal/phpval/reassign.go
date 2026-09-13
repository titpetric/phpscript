package phpval

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

// Variable type immutability is a deliberate phpscript divergence from PHP:
// the first non-null value assigned to a variable declares its type, and a
// later assignment of another type is a RuntimeException at the engines and
// a fatal finding at the linter. A variable whose first value is null is
// declared any, the way Go's `var x any = nil` is, and never constrains; a
// null written over a typed variable is a violation, the way `var s string =
// nil` does not compile. int64 and float64 share one number class because
// PHP arithmetic widens int to float on overflow and uneven division.
//
// The check is stateless: the invariant guarantees a variable's current
// value spells its declared type, so old and new value are all it reads.

// classNone constrains nothing: null/unset, which declares no type, and
// false, PHP's absence sentinel - the stdlib's own T|false return convention
// means `while (($row = fgetcsv($r)) !== false)` writes false over an
// array-typed variable on every normal loop exit. true is an honest bool and
// stays one.
const (
	classNone = iota
	classBool
	classNumber
	classString
	classArray
	classObject
)

func typeClassCode(v any) int {
	switch x := v.(type) {
	case nil:
		return classNone
	case bool:
		if !x {
			return classNone
		}
		return classBool
	case int, int64, float64:
		return classNumber
	case string:
		return classString
	}
	if _, ok := model.LenValues(v); ok {
		return classArray
	}
	return classObject
}

// ReassignAllowed is the hot half of the check: it reports whether writing
// next over cur keeps the variable's declared type, with no message built.
// Call ReassignMessage only after a false, on the throw path.
func ReassignAllowed(cur, next any) bool {
	curClass := typeClassCode(cur)
	if curClass == classNone {
		return true
	}
	nextClass := typeClassCode(next)
	if nextClass == curClass {
		return true
	}
	// The false sentinel writes over anything; null over a typed variable
	// does not.
	return nextClass == classNone && next != nil
}

// TypeClass buckets a runtime value for the reassignment check; see
// typeClassCode for the rules. The empty class constrains nothing.
func TypeClass(v any) string {
	switch typeClassCode(v) {
	case classNone:
		return ""
	case classBool:
		return "bool"
	case classNumber:
		return "number"
	case classString:
		return "string"
	case classArray:
		return "array"
	}
	return "object"
}

// TypeName names a value's type for the message a violation carries: the
// specific spelling (int, float) where the class is wider.
func TypeName(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case int, int64:
		return "int"
	case float64:
		return "float"
	case string:
		return "string"
	}
	if _, ok := model.LenValues(v); ok {
		return "array"
	}
	return "object"
}

// ReassignMessage reports the violation message for writing next over cur,
// "" when the write is allowed: cur unconstrained (unset, null-declared or
// holding the false sentinel), both values in one class, or next being the
// false sentinel itself. Writing null over a typed variable is the one
// unconstrained value that stays a violation - null answers any type, and a
// variable is released with unset(), not laundered through null.
func ReassignMessage(name string, cur, next any) string {
	if ReassignAllowed(cur, next) {
		return ""
	}
	if next == nil {
		return fmt.Sprintf("no reassignment: $%s previously declared as %s, null does not unset it", name, TypeName(cur))
	}
	return fmt.Sprintf("no reassignment: $%s previously declared as %s", name, TypeName(cur))
}
