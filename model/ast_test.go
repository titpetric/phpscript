package model

import (
	"reflect"
	"strings"
	"testing"
)

// TestNode covers the two marker interfaces the runner switches on. A statement
// is executed for its side effects and an expression evaluates to a value, so a
// node reaches the interpreter through one marker or the other, and a type
// carrying neither is unreachable.
//
// Include carries both. PHP writes an include as a standalone statement and as
// a value-producing expression, so one node stands in both positions.
func TestNode(t *testing.T) {
	t.Parallel()
	statements := map[string]Node{
		"Foreach":   &Foreach{},
		"DoWhile":   &DoWhile{},
		"Use":       &Use{},
		"Declare":   &Declare{},
		"Unset":     &Unset{},
		"StaticVar": &StaticVar{},
		"Global":    &Global{},
		"ConstDecl": &ConstDecl{},
		"Throw":     &Throw{},
		"Try":       &Try{},
		"Break":     &Break{},
		"Continue":  &Continue{},
	}
	for name, node := range statements {
		if _, ok := node.(Stmt); !ok {
			t.Errorf("%s is no Stmt, so no statement list holds it", name)
		}
		if _, ok := node.(Expr); ok {
			t.Errorf("%s is an Expr as well, which of the statements only Include is", name)
		}
	}

	expressions := map[string]Node{
		"Ref":        &Ref{},
		"ClassConst": &ClassConst{},
		"StaticProp": &StaticProp{},
		"Cast":       &Cast{},
		"AssignExpr": &AssignExpr{},
		"ListExpr":   &ListExpr{},
	}
	for name, node := range expressions {
		if _, ok := node.(Expr); !ok {
			t.Errorf("%s is no Expr, so nothing evaluates it", name)
		}
		if _, ok := node.(Stmt); ok {
			t.Errorf("%s is a Stmt as well, which of the expressions only Include is", name)
		}
	}

	var include Node = &Include{Keyword: "require_once", Once: true}
	if _, ok := include.(Stmt); !ok {
		t.Error("Include is no Stmt, so a statement list cannot hold it")
	}
	if _, ok := include.(Expr); !ok {
		t.Error("Include is no Expr, so `$x = include 'f.php'` has no node")
	}
}

// TestSourceSpan covers the line range of a statement and the map the formatter
// reads it from. The range is inclusive, so a one-line statement records the
// same number twice.
//
// The key is the statement node, so two statements of one kind need two
// addresses: Go assigns every zero-sized allocation the same one, and Break and
// Continue carry their source line so neither is zero-sized.
func TestSourceSpan(t *testing.T) {
	t.Parallel()
	first := &Break{Line: 4}
	middle := &Continue{Line: 6}
	last := &Break{Line: 9}

	prog := &Program{
		Stmts: []Stmt{first, middle, last},
		SourceSpans: map[Stmt]SourceSpan{
			first:  {Start: 4, End: 4},
			middle: {Start: 6, End: 6},
			last:   {Start: 9, End: 9},
		},
	}
	if got := len(prog.SourceSpans); got != 3 {
		t.Fatalf("the span map holds %d entries, want 3: two nodes shared an address", got)
	}
	for _, stmt := range prog.Stmts {
		span, ok := prog.SourceSpans[stmt]
		if !ok {
			t.Fatalf("%T carries no span", stmt)
		}
		if span.Start != span.End {
			t.Errorf("%T spans %d to %d, want one line", stmt, span.Start, span.End)
		}
	}

	same := map[Stmt]SourceSpan{}
	same[&Break{Line: 4}] = SourceSpan{Start: 4, End: 4}
	same[&Break{Line: 4}] = SourceSpan{Start: 7, End: 7}
	if got := len(same); got != 2 {
		t.Errorf("two Break nodes written on one line share %d map key, want 2 keys", got)
	}

	span := SourceSpan{Start: 10, End: 12}
	if got := span.End - span.Start + 1; got != 3 {
		t.Errorf("the span 10 to 12 covers %d lines, want 3", got)
	}
}

// TestUnwrapParenthesized covers reading an expression's shape without
// evaluating it. A consumer that switches on the node type needs grouping to be
// transparent, so `($a)` and `((($a)))` both read as the variable.
//
// The grouping stays on the tree. The formatter rewrites files in place and
// prints the parentheses an author wrote, so peeling them copies nothing and
// edits nothing.
func TestUnwrapParenthesized(t *testing.T) {
	t.Parallel()
	v := &Var{Name: "v"}
	one := &Lit{Value: int64(1)}

	for _, tc := range []struct {
		name string
		expr Expr
		want Expr
	}{
		{"bare", v, v},
		{"one layer", &Parenthesized{X: v}, v},
		{"three layers", &Parenthesized{X: &Parenthesized{X: &Parenthesized{X: v}}}, v},
		{"literal", one, one},
		{"nothing", nil, nil},
	} {
		if got := UnwrapParenthesized(tc.expr); got != tc.want {
			t.Errorf("%s: UnwrapParenthesized() = %#v, want %#v", tc.name, got, tc.want)
		}
	}

	grouped := &Binary{
		Op:    "*",
		Left:  &Parenthesized{X: &Binary{Op: "+", Left: v, Right: one}},
		Right: one,
	}
	if UnwrapParenthesized(grouped.Left) == grouped.Left {
		t.Fatal("UnwrapParenthesized answered the grouping it was given")
	}
	if _, ok := grouped.Left.(*Parenthesized); !ok {
		t.Error("the grouping came off the tree, so a printer cannot put it back")
	}
}

// TestArrayItem covers how an array literal tells a list-style entry from a
// keyed one. Key is nil for `[v]`, where the position is assigned when the
// literal is evaluated.
//
// `[null => v]` carries a key holding the null literal, which PHP normalises to
// the empty string. A consumer reading Key for nil alone would turn the second
// spelling into the first.
func TestArrayItem(t *testing.T) {
	t.Parallel()
	appended := ArrayItem{Val: &Lit{Value: "v"}}
	keyedNull := ArrayItem{Key: &Lit{Value: nil}, Val: &Lit{Value: "v"}}

	if appended.Key != nil {
		t.Error("a list-style entry carries a key expression")
	}
	if keyedNull.Key == nil {
		t.Fatal("`[null => v]` lost its key expression")
	}
	if got := keyedNull.Key.(*Lit).Value; got != nil {
		t.Errorf("the key literal holds %v, want nil", got)
	}

	lit := &ArrayLit{Items: []ArrayItem{appended, keyedNull}}
	var keys []Expr
	for _, item := range lit.Items {
		if item.Val == nil {
			t.Error("an entry carries no value")
		}
		keys = append(keys, item.Key)
	}
	if want := []Expr{nil, keyedNull.Key}; !reflect.DeepEqual(keys, want) {
		t.Errorf("the literal's keys are %v, want %v", keys, want)
	}
}

// TestUse covers what survives parsing. The parser resolves an import to a
// fully-qualified name while it reads the file, so the statement has no runtime
// effect; the node is kept because the formatter rewrites files in place, and a
// node the printer cannot see is a node it deletes.
//
// Alias is set for the `as` spelling alone. The short name of Path is implied
// otherwise, so an empty Alias is the mark of an import printed as written.
func TestUse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		stmt  *Use
		kind  string
		alias string
	}{
		{"class", &Use{Imports: []UseImport{{Path: `A\B\C`}}}, "", ""},
		{"aliased class", &Use{Imports: []UseImport{{Path: `A\B\C`, Alias: "D"}}}, "", "D"},
		{"function", &Use{Kind: "function", Imports: []UseImport{{Path: `A\f`}}}, "function", ""},
		{"const", &Use{Kind: "const", Imports: []UseImport{{Path: `A\K`}}}, "const", ""},
	} {
		if got := tc.stmt.Kind; got != tc.kind {
			t.Errorf("%s: Kind = %q, want %q", tc.name, got, tc.kind)
		}
		if got := tc.stmt.Imports[0].Alias; got != tc.alias {
			t.Errorf("%s: Alias = %q, want %q", tc.name, got, tc.alias)
		}
		if got := tc.stmt.Imports[0].Path; got == "" {
			t.Errorf("%s: the import carries no path", tc.name)
		}
	}

	grouped := &Use{Imports: []UseImport{{Path: "A"}, {Path: "B", Alias: "C"}}}
	if got := len(grouped.Imports); got != 2 {
		t.Errorf("`use A, B as C;` holds %d imports, want 2", got)
	}
}

// TestDeclare covers the spelling a consumer has to tell apart. The runtime has
// one set of semantics and no directive varies them, so the directives are
// recorded for printing.
//
// A `declare(...) { ... }` still runs its body, and Block marks the spelling
// even where that body is empty: the statement form has no body at all, so the
// flag is the only thing separating the two.
func TestDeclare(t *testing.T) {
	t.Parallel()
	strict := &Declare{Directives: []DeclareDirective{
		{Name: "strict_types", Value: &Lit{Value: int64(1)}},
	}}
	if strict.Block {
		t.Error("`declare(strict_types=1);` is marked as the block spelling")
	}
	if got := len(strict.Directives); got != 1 {
		t.Fatalf("the statement holds %d directives, want 1", got)
	}
	if got := strict.Directives[0].Name; got != "strict_types" {
		t.Errorf("the directive is named %q, want \"strict_types\"", got)
	}
	if strict.Directives[0].Value == nil {
		t.Error("the directive carries no value")
	}

	empty := &Declare{
		Directives: []DeclareDirective{{Name: "ticks", Value: &Lit{Value: int64(1)}}},
		Block:      true,
	}
	if !empty.Block {
		t.Error("an empty `declare(ticks=1) {}` lost the block spelling")
	}
	if got := len(empty.Body); got != 0 {
		t.Errorf("the block holds %d statements, want 0", got)
	}

	pair := &Declare{Directives: []DeclareDirective{
		{Name: "ticks", Value: &Lit{Value: int64(1)}},
		{Name: "strict_types", Value: &Lit{Value: int64(1)}},
	}}
	if got := len(pair.Directives); got != 2 {
		t.Errorf("`declare(ticks=1, strict_types=1)` holds %d directives, want 2", got)
	}
}

// TestStaticVar covers the per-entry initializer of `static $x = 1, $y;`. Each
// initializer runs once per function lifetime, so an entry with no initializer
// has to read apart from one initialized to null: Default is nil for `static
// $y;` and a literal for `static $y = null;`.
func TestStaticVar(t *testing.T) {
	t.Parallel()
	decl := &StaticVar{Vars: []StaticVarDecl{
		{Name: "count", Default: &Lit{Value: int64(0)}},
		{Name: "seen"},
		{Name: "cache", Default: &Lit{Value: nil}},
	}}

	if got := len(decl.Vars); got != 3 {
		t.Fatalf("the declaration holds %d entries, want 3", got)
	}
	for _, entry := range decl.Vars {
		if entry.Name == "" {
			t.Error("an entry carries no name")
		}
		if strings.HasPrefix(entry.Name, "$") {
			t.Errorf("the name %q kept its sigil", entry.Name)
		}
	}
	if decl.Vars[1].Default != nil {
		t.Error("`static $seen;` carries an initializer")
	}
	if decl.Vars[2].Default == nil {
		t.Fatal("`static $cache = null;` lost its initializer")
	}
	if got := decl.Vars[2].Default.(*Lit).Value; got != nil {
		t.Errorf("the initializer holds %v, want nil", got)
	}
}

// TestCatch covers the clause a Try selects on. Type is the declared filter,
// `Exception` or the union form `A|B`, and PHP requires it: a catch clause
// printed without one is a syntax error. Var is the bound name without its
// sigil, so `echo $e` reads the caught error.
//
// Matching walks the clauses in source order, because it is by Go error type
// and no PHP class hierarchy: the first clause whose type matches handles the
// error, so the order the parser recorded is the order a consumer reads.
func TestCatch(t *testing.T) {
	t.Parallel()
	try := &Try{
		Body: []Stmt{&Throw{X: &New{Class: "RuntimeException"}}},
		Catches: []Catch{
			{Type: "TypeError", Var: "e", Line: 4},
			{Type: "RuntimeException|LogicException", Var: "err", Line: 6},
		},
		Finally:     []Stmt{&Echo{Args: []Expr{&Lit{Value: "done"}}}},
		FinallyLine: 8,
	}

	if got := len(try.Catches); got != 2 {
		t.Fatalf("the statement holds %d catch clauses, want 2", got)
	}
	for i, clause := range try.Catches {
		if clause.Type == "" {
			t.Errorf("clause %d declares no type, which PHP rejects", i)
		}
		if clause.Var == "" {
			t.Errorf("clause %d binds no variable", i)
		}
		if strings.HasPrefix(clause.Var, "$") {
			t.Errorf("clause %d kept the sigil on %q", i, clause.Var)
		}
		if clause.Line == 0 {
			t.Errorf("clause %d records no source line, so a comment above it has nowhere to go", i)
		}
	}
	if try.Catches[0].Line >= try.Catches[1].Line {
		t.Error("the clauses are not in source order")
	}
	if got, want := strings.Split(try.Catches[1].Type, "|"), []string{"RuntimeException", "LogicException"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the union filter names %v, want %v", got, want)
	}
	if try.FinallyLine <= try.Catches[1].Line {
		t.Error("the finally keyword is recorded above the last catch clause")
	}
}

// TestSwitchCase covers the arms of a switch and the line of each `case`
// keyword, which the formatter places comments by.
//
// An arm with an empty body falls through to the next one under PHP's
// semantics, so an empty Body is a written arm and no missing one. Default is held apart from Cases because it matches no value.
func TestSwitchCase(t *testing.T) {
	t.Parallel()
	sw := &Switch{
		Cond: &Var{Name: "code"},
		Cases: []SwitchCase{
			{Value: &Lit{Value: int64(200)}, Line: 3},
			{Value: &Lit{Value: int64(201)}, Line: 4, Body: []Stmt{
				&Echo{Args: []Expr{&Lit{Value: "ok"}}},
				&Break{Line: 6},
			}},
		},
		Default: []Stmt{&Echo{Args: []Expr{&Lit{Value: "other"}}}},
	}

	if got := len(sw.Cases); got != 2 {
		t.Fatalf("the switch holds %d arms, want 2", got)
	}
	if got := len(sw.Cases[0].Body); got != 0 {
		t.Errorf("the falling-through arm holds %d statements, want 0", got)
	}
	if got := len(sw.Cases[1].Body); got != 2 {
		t.Errorf("the breaking arm holds %d statements, want 2", got)
	}
	for i, arm := range sw.Cases {
		if arm.Value == nil {
			t.Errorf("arm %d carries no value, which of the arms only default does", i)
		}
		if arm.Line == 0 {
			t.Errorf("arm %d records no source line", i)
		}
	}
	if sw.Cases[0].Line >= sw.Cases[1].Line {
		t.Error("the arms are not in source order")
	}
	if got := len(sw.Default); got != 1 {
		t.Errorf("the default arm holds %d statements, want 1", got)
	}
}

// TestClosureUse covers a closure's capture list. ByRef records the `&$name`
// spelling; the runtime has no reference values, so both spellings bind the
// same value, and the marker is kept so the formatter prints the source back
// and the linter can report it.
//
// A capture binds into the closure's own frame, so a write through one is
// invisible to the enclosing scope: AssignsTo reports nothing for a body
// assigning to a captured name.
func TestClosureUse(t *testing.T) {
	t.Parallel()
	total := &Var{Name: "total"}
	fn := &Closure{
		Params: []Param{{Name: "item"}},
		Uses:   []ClosureUse{{Name: "total", ByRef: true}, {Name: "sep"}},
		Body: []Stmt{
			&Assign{Target: total, Op: "+=", Value: &Var{Name: "item"}},
		},
	}

	if got := len(fn.Uses); got != 2 {
		t.Fatalf("the capture list holds %d names, want 2", got)
	}
	if !fn.Uses[0].ByRef {
		t.Error("`&$total` lost its reference marker")
	}
	if fn.Uses[1].ByRef {
		t.Error("`$sep` was recorded as a reference capture")
	}
	for _, use := range fn.Uses {
		if strings.HasPrefix(use.Name, "$") {
			t.Errorf("the captured name %q kept its sigil", use.Name)
		}
	}
	if AssignsTo(total, []Stmt{&ExprStmt{X: fn}}) {
		t.Error("a write to a captured name reads as a write to the enclosing frame")
	}
}
