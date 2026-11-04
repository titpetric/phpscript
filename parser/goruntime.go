package parser

import (
	"strings"

	"github.com/titpetric/phpscript/model"
)

// The go runtime is the one binding area a file opts into. `use Runtime\Go`
// binds go's builtin names for the rest of that file: a bare `make(...)` is the
// binding and not a name php resolves, and the call prints as the source wrote
// it, so the formatter round-trips.
//
// The binding is per file, as every `use` is, and it wins over the enclosing
// namespace: a file that imported go's `make` meant go's, which is how php
// reads an imported name against one it would otherwise resolve. It binds the
// spelling go uses and no other, where php folds a function's case: a file that
// writes `Make()` asked php for it.
//
// A qualified call decides for itself and needs no import, here and in the
// runtime both: `\strlen($s)` is php's whatever the file imported, and
// `Runtime\Go\make(...)` is go's.

// useGo records the import of the go runtime, the one import the parser reads
// for itself. A leading separator and the namespace's case are php's to ignore.
func (p *parser) useGo(name string) {
	if strings.EqualFold(strings.Trim(name, "\\"), model.GoNamespace) {
		p.goRuntime = true
	}
}

// goBound reports whether a bare name is a go builtin this file's imports bind.
func (p *parser) goBound(name string) bool {
	return p.goRuntime && model.GoBuiltin(name)
}

// goFold is the conversion a builtin type applies to a literal, applied where
// the file parses: `int64(0)` is the literal 0, it reaches no function table at
// run time and both engines read what is left.
//
// Only a pair the conversion decides on its own folds. A value php has to read
// first, a string under a numeric type, reaches the binding, which reads it
// through phpval and applies the same conversion to the result.
//
// A folded value prints as the literal it became, which is the spelling the
// printer already picks for a number: it prints `0x10` as 16.
func (p *parser) goFold(name string, args []model.Expr) (model.Expr, bool) {
	if len(args) != 1 || !model.GoType(name) {
		return nil, false
	}
	value, ok := goLitValue(args[0])
	if !ok {
		return nil, false
	}
	folded, ok := model.GoFold(name, value)
	if !ok {
		return nil, false
	}
	// A conversion that changed nothing keeps the node, and with it the source
	// spelling a string literal carries.
	if lit, isLit := args[0].(*model.Lit); isLit && folded == lit.Value {
		return lit, true
	}
	return p.newLit(folded), true
}

// goLitValue reads the value of a literal argument. php has no negative
// literal: `-1` is a unary minus over 1, go folds the sign into the constant,
// and a conversion of a literal reads both the same way.
func goLitValue(arg model.Expr) (any, bool) {
	switch n := model.UnwrapParenthesized(arg).(type) {
	case *model.Lit:
		return n.Value, true
	case *model.Unary:
		if n.Op != "-" || n.Postfix {
			return nil, false
		}
		lit, ok := model.UnwrapParenthesized(n.X).(*model.Lit)
		if !ok {
			return nil, false
		}
		switch v := lit.Value.(type) {
		case int64:
			return -v, true
		case float64:
			return -v, true
		}
	}
	return nil, false
}
