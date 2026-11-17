package parser

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// The go runtime is the one binding area a file opts into. `use Runtime\Go`
// binds go's builtin names for the rest of that file: a bare `make(...)` is the
// binding and no name php resolves, and the call prints as the source wrote
// it, which keeps the formatter round-tripping.
//
// The binding is per file, as every `use` is. It wins over the enclosing
// namespace: a file that imported go's `make` meant go's, the way php reads an
// imported name against one it would otherwise resolve. It binds the spelling
// go uses and no other, where php folds a function's case: a file that writes
// `Make()` reaches php for it.
//
// A qualified call decides for itself and needs no import, here and in the
// runtime both: `\strlen($s)` is php's whatever the file imported, and
// `Runtime\Go\make(...)` is go's.

// useGo records the import of the go runtime, the one import the parser reads
// for itself. A leading separator and the namespace's case are php's to ignore.
func (p *parser) useGo(name string) {
	if strings.EqualFold(strings.Trim(name, "\\"), modelutil.GoNamespace) {
		p.goRuntime = true
	}
}

// isGoErrorType reports whether a catch type names go's error.
func isGoErrorType(name string) bool {
	return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(name), "\\"), "error")
}

// goBound reports whether a bare name is a go builtin this file's imports bind.
func (p *parser) goBound(name string) bool {
	return p.goRuntime && modelutil.GoBuiltin(name)
}

// atGoSlice reports whether the tokens at a type position open a `[]T` type: a
// `[]` pair with an element type behind it, which is a slice, a nested one, or
// a qualified class name.
func (p *parser) atGoSlice() bool {
	if !p.goRuntime || !p.isOp("[") || !p.isOpAt(1, "]") {
		return false
	}
	return p.peek(2).kind == tIdent || p.isOpAt(2, "[") || p.isOpAt(2, "\\") || p.isOpAt(2, "*")
}

// atGoPointer reports whether a type position opens go's `*T` pointer type.
func (p *parser) atGoPointer() bool {
	return p.goRuntime && p.isOp("*")
}

// atGoMake reports whether the call ahead reads `make([]`, the one place a
// type stands where php puts an expression. Everywhere else `[` opens an
// array literal, and `foreach ([] as $v)` reads it there.
func (p *parser) atGoMake(name string) bool {
	return name == "make" && p.isOpAt(1, "[") && p.isOpAt(2, "]")
}

// parseGoMakeArgs reads make's argument list: a type, then the length and the
// capacity, which are expressions like any other.
func (p *parser) parseGoMakeArgs() ([]model.Expr, error) {
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	mark := p.exprs.mark()
	elem, err := p.parseGoTypeArg()
	if err != nil {
		p.exprs.drop(mark)
		return nil, err
	}
	p.exprs.push(elem)
	for p.isOp(",") {
		p.next()
		if p.isOp(")") {
			break
		}
		size, err := p.parseExpr()
		if err != nil {
			p.exprs.drop(mark)
			return nil, err
		}
		p.exprs.push(size)
	}
	return p.exprs.take(mark), p.eatOp(")")
}

// parseGoTypeArg reads a type spelling, which php has no expression for: one
// `[]` per dimension and then the element type, go's or a class.
//
// The value is the spelling the binding reads, with a class name qualified the
// way the file qualifies any other name, and the source spelling goes in as the
// one the formatter prints.
func (p *parser) parseGoTypeArg() (model.Expr, error) {
	dims := ""
	for p.isOp("[") && p.isOpAt(1, "]") {
		p.next()
		p.next()
		dims += "[]"
	}
	name, absolute, err := p.parseQualifiedName(true)
	if err != nil {
		return nil, fmt.Errorf("line %d: %s: %w", p.cur().line, modelutil.GoNamespace+`\make`, err)
	}
	// `make([]http.Request, 2)` takes the type an import enumerated, where the
	// element is the type and not the package. See goimport.go.
	if full, source, ok := p.goMember(name); ok {
		return p.newStringLit(dims+full, dims+source), nil
	}
	qualified := name
	if !modelutil.GoType(name) {
		qualified = p.qualify(name, absolute)
	}
	return p.newStringLit(dims+qualified, dims+name), nil
}

// goFold is the conversion a builtin type applies to a literal, applied where
// the file parses: `int64(0)` is the literal 0, it reaches no function table at
// run time and both engines read the literal.
//
// Only a pair the conversion decides on its own folds. A value php has to read
// first, a string under a numeric type, reaches the binding, which reads it
// through phpval and applies the same conversion to the result.
//
// A folded value prints as the literal it became, which is the spelling the
// printer already picks for a number: it prints `0x10` as 16.
func (p *parser) goFold(name string, args []model.Expr) (model.Expr, bool) {
	if len(args) != 1 || !modelutil.GoType(name) {
		return nil, false
	}
	value, ok := goLitValue(args[0])
	if !ok {
		return nil, false
	}
	folded, ok := modelutil.GoFold(name, value)
	if !ok {
		return nil, false
	}
	// A conversion that changed nothing keeps the node, and with it the source
	// spelling a string literal holds.
	if lit, isLit := args[0].(*model.Lit); isLit && folded == lit.Value {
		return lit, true
	}
	return p.newLit(folded), true
}

// goLitValue reads the value of a literal argument. php has no negative
// literal: `-1` is a unary minus over 1, go folds the sign into the constant,
// and a conversion of a literal reads both the same way.
func goLitValue(arg model.Expr) (any, bool) {
	switch n := modelutil.UnwrapParenthesized(arg).(type) {
	case *model.Lit:
		return n.Value, true
	case *model.Unary:
		if n.Op != "-" || n.Postfix {
			return nil, false
		}
		lit, ok := modelutil.UnwrapParenthesized(n.X).(*model.Lit)
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

// atGoSliceLit reports whether the tokens ahead open a typed slice literal.
func (p *parser) atGoSliceLit() bool {
	if !p.goRuntime || !p.isOp("[") || !p.isOpAt(1, "]") {
		return false
	}
	i := 0
	for p.isOpAt(i, "[") && p.isOpAt(i+1, "]") {
		i += 2
	}
	if p.isOpAt(i, "{") {
		return true
	}
	if tok := p.peek(i); tok.kind == tIdent && tok.val == "map" && p.isOpAt(i+1, "[") {
		return true
	}
	if p.peek(i).kind != tIdent {
		return false
	}
	i++
	for p.isOpAt(i, ".") || p.isOpAt(i, "\\") {
		i++
		if p.peek(i).kind != tIdent {
			return false
		}
		i++
	}
	return p.isOpAt(i, "{")
}

// parseGoListLiteral reads `[a, b]` as a slice, or `[]T{...}` as a typed one.
func (p *parser) parseGoListLiteral() (model.Expr, error) {
	if p.atGoSliceLit() {
		return p.parseGoSliceLit()
	}
	if err := p.eatOp("["); err != nil {
		return nil, err
	}
	items, err := p.parseGoLitItems("]", false, "")
	if err != nil {
		return nil, err
	}
	return &model.GoLit{Items: items}, p.eatOp("]")
}

// parseGoSliceLit reads the type a slice literal names and its entries.
func (p *parser) parseGoSliceLit() (model.Expr, error) {
	dims := ""
	for p.isOp("[") && p.isOpAt(1, "]") {
		p.next()
		p.next()
		dims += "[]"
	}
	if p.isOp("{") {
		return p.parseGoInferredDims(dims)
	}
	typ, err := p.parseGoLitType()
	if err != nil {
		return nil, err
	}
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	litType := dims + typ
	items, err := p.parseGoLitItems("}", false, strings.TrimPrefix(litType, "[]"))
	if err != nil {
		return nil, err
	}
	return &model.GoLit{ValType: litType, Items: items}, p.eatOp("}")
}

// parseGoInferredDims reads `[][]{...}` and infers the base type from values.
func (p *parser) parseGoInferredDims(dims string) (model.Expr, error) {
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	items, err := p.parseGoInferredItems("}", len(dims)/2)
	if err != nil {
		return nil, err
	}
	return &model.GoLit{ValType: dims, Items: items}, p.eatOp("}")
}

// parseGoInferredItems reads entries depth levels from the base type.
func (p *parser) parseGoInferredItems(closeOp string, depth int) ([]model.ArrayItem, error) {
	items := make([]model.ArrayItem, 0, 4)
	for !p.isOp(closeOp) {
		var value model.Expr
		if depth > 1 && p.isOp("{") {
			if err := p.eatOp("{"); err != nil {
				return nil, err
			}
			inner, err := p.parseGoInferredItems("}", depth-1)
			if err != nil {
				return nil, err
			}
			value = &model.GoLit{ValType: strings.Repeat("[]", depth-1), Items: inner}
			if err := p.eatOp("}"); err != nil {
				return nil, err
			}
		} else {
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			value = expr
		}
		items = append(items, model.ArrayItem{Val: value})
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	return items, nil
}

// parseGoMapLiteral reads `{key: value}` as a map with inferred types.
func (p *parser) parseGoMapLiteral() (model.Expr, error) {
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	items, err := p.parseGoLitItems("}", true, "")
	if err != nil {
		return nil, err
	}
	return &model.GoLit{Map: true, Items: items}, p.eatOp("}")
}

// parseGoMapTyped reads `map[K]V{...}` after the `map` keyword.
func (p *parser) parseGoMapTyped() (model.Expr, error) {
	if err := p.eatOp("["); err != nil {
		return nil, err
	}
	key, err := p.parseGoLitType()
	if err != nil {
		return nil, err
	}
	if err := p.eatOp("]"); err != nil {
		return nil, err
	}
	val, err := p.parseGoLitType()
	if err != nil {
		return nil, err
	}
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	items, err := p.parseGoLitItems("}", true, val)
	if err != nil {
		return nil, err
	}
	return &model.GoLit{Map: true, KeyType: key, ValType: val, Items: items}, p.eatOp("}")
}

// parseGoLitType reads the name, [] nesting or map[K]V a literal names.
func (p *parser) parseGoLitType() (string, error) {
	dims := ""
	for p.isOp("[") && p.isOpAt(1, "]") {
		p.next()
		p.next()
		dims += "[]"
	}
	if tok := p.peek(0); tok.kind == tIdent && tok.val == "map" && p.isOpAt(1, "[") {
		p.next()
		p.next()
		key, err := p.parseGoLitType()
		if err != nil {
			return "", err
		}
		if err := p.eatOp("]"); err != nil {
			return "", err
		}
		val, err := p.parseGoLitType()
		if err != nil {
			return "", err
		}
		return dims + "map[" + key + "]" + val, nil
	}
	name, absolute, err := p.parseQualifiedName(true)
	if err != nil {
		return "", err
	}
	if full, _, ok := p.goMember(name); ok {
		return dims + full, nil
	}
	if modelutil.GoType(name) {
		return dims + name, nil
	}
	return dims + p.qualify(name, absolute), nil
}

// parseGoLitItems reads the entries of a go literal up to closeOp.
func (p *parser) parseGoLitItems(closeOp string, mapForm bool, elemType string) ([]model.ArrayItem, error) {
	items := make([]model.ArrayItem, 0, 4)
	for !p.isOp(closeOp) {
		first, err := p.parseGoLitValue(elemType, mapForm)
		if err != nil {
			return nil, err
		}
		item := model.ArrayItem{Val: first}
		if p.isOp(":") {
			if !mapForm {
				return nil, fmt.Errorf("line %d: a slice literal takes values and no keys", p.cur().line)
			}
			p.next()
			val, err := p.parseGoLitValue(elemType, false)
			if err != nil {
				return nil, err
			}
			item = model.ArrayItem{Key: first, Val: val}
		} else if mapForm {
			return nil, fmt.Errorf("line %d: a map literal entry reads key: value", p.cur().line)
		}
		items = append(items, item)
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	return items, nil
}

// parseGoLitValue reads one entry, eliding elemType at a braced one.
func (p *parser) parseGoLitValue(elemType string, isKey bool) (model.Expr, error) {
	if isKey || elemType == "" || !p.isOp("{") {
		return p.parseExpr()
	}
	return p.parseGoElided(elemType)
}

// parseGoElided reads `{...}` under an element type it does not repeat.
func (p *parser) parseGoElided(elemType string) (model.Expr, error) {
	line := p.cur().line
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	if elem, ok := strings.CutPrefix(elemType, "[]"); ok {
		items, err := p.parseGoLitItems("}", false, elem)
		if err != nil {
			return nil, err
		}
		return &model.GoLit{ValType: elemType, Items: items}, p.eatOp("}")
	}
	if keyName, valName, ok := splitGoLitMap(elemType); ok {
		items, err := p.parseGoLitItems("}", true, valName)
		if err != nil {
			return nil, err
		}
		return &model.GoLit{Map: true, KeyType: keyName, ValType: valName, Items: items}, p.eatOp("}")
	}
	return nil, fmt.Errorf("line %d: a braced entry needs a slice or map element type, and %s is neither", line, elemType)
}

// splitGoLitMap splits a map[K]V spelling into its key and value types.
func splitGoLitMap(name string) (keyName, valName string, ok bool) {
	rest, ok := strings.CutPrefix(name, "map[")
	if !ok {
		return "", "", false
	}
	depth := 1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 && i+1 < len(rest) {
				return rest[:i], rest[i+1:], true
			}
		}
	}
	return "", "", false
}
