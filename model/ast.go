// Package model holds the shared data structures (AST, runtime values, class
// metadata) used by both the parser and the runner packages.
//
// The split is deliberate: parser/ produces these structures from PHP source,
// runner/ consumes them. Neither package depends on the other; they only share
// model/.
package model

// Node is the root interface for every AST element.
type Node interface {
	node()
}

// Stmt is a statement: something executed for its side effects (echo, control
// flow, assignment, declarations). Statements are interpreted directly by the
// runner because expr-lang has no concept of statements, loops or mutation.
type Stmt interface {
	Node
	stmt()
}

// Expr is an expression: something that evaluates to a value. Expressions are
// the unit the runner transpiles into go-expr (expr-lang) source and evaluates
// through the embedded VM.
type Expr interface {
	Node
	expr()
}

// Program is the top-level result of parsing a single PHP file.
type Program struct {
	Stmts     []Stmt
	Namespace string // set when the file declares `namespace Name;`
	// SourceSpans records original statement lines when Program came from the
	// parser. Consumers may ignore it; the formatter uses it to retain a single
	// intentional blank line between statements.
	SourceSpans map[Stmt]SourceSpan
}

// SourceSpan is the inclusive source-line range occupied by a statement.
type SourceSpan struct {
	Start int
	End   int
}

// RouteAnnotation is one // @route declaration found in a PHP source file.
type RouteAnnotation struct {
	Method string
	Path   string
}

func (*Program) node() {}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

// InlineHTML is raw text outside of <?php ... ?> tags. It is emitted verbatim.
type InlineHTML struct {
	Text string
}

// Echo writes the evaluated arguments to the output buffer.
type Echo struct {
	Args []Expr
}

// ExprStmt is an expression evaluated for its side effects (e.g. a method call).
type ExprStmt struct {
	X Expr
}

// Assign is `Target = Value`. Op may be "=", ".=", "+=", "[]=" (append).
// expr-lang cannot mutate, so assignment is handled entirely by the runner.
type Assign struct {
	Target Expr // Var, PropAccess or Index
	Op     string
	Value  Expr
}

// If is `if (Cond) { Then } elseif... else { Else }`.
type If struct {
	Cond Expr
	Then []Stmt
	Else []Stmt // may itself contain a single nested *If for elseif chains
}

// Foreach is `foreach (Source as [KeyTarget =>] ValTarget) { Body }`.
//
// ByRef records the `as &$v` spelling. It selects between PHP's two loop
// semantics: by value the target holds a copy of the element, so writing to it
// leaves the source alone; by reference the target is the element, so writing
// to it edits the source.
type Foreach struct {
	Source    Expr
	KeyTarget Expr // nil if not captured
	ValTarget Expr
	ByRef     bool   // `as &$v`: the target writes back into Source
	KeyVar    string // deprecated: use KeyTarget
	ValVar    string // deprecated: use ValTarget
	Body      []Stmt
}

// For is `for (Init; Cond; Post) { Body }`. `while` is parsed into a For with
// nil Init/Post.
type For struct {
	Init Stmt
	Cond Expr
	Post Stmt
	Body []Stmt
}

// Return exits the current function with an optional value.
type Return struct {
	Value Expr // may be nil
}

// Include pulls in another file (include / include_once / require). PHP allows
// include constructs both as standalone statements and as value-producing
// expressions.
type Include struct {
	Path          Expr
	Keyword       string // include, include_once, require, or require_once
	Once          bool
	Parenthesized bool
}

// FuncDecl is a free function or a class method declared with the
// `function Class::method()` syntax described in the README.
type FuncDecl struct {
	Class      string // "" for free functions
	Name       string
	Filename   string
	Params     []Param
	Body       []Stmt
	Visibility string // "public", "protected", "private", or ""
	Static     bool
	Abstract   bool // declaration only; Body is empty
}

// ClassDecl is a trimmed-down class: fields + methods + class constants, no
// inheritance. Abstract is tolerated (parsed) but not enforced (README omits
// abstract classes; minitpl's Hook is abstract only to declare constants).
type ClassDecl struct {
	Name     string
	Abstract bool
	Fields   []Field
	Statics  []Field // `static $name = expr` properties, referenced as Class::$name
	Consts   []Field // class constants (Name + value Expr), referenced as Class::NAME
	Methods  []*FuncDecl
}

// Unset is `unset($a, $b[$k], $o->p, C::$s)`. Each target is removed from the
// scope, array or property bag holding it.
type Unset struct {
	Targets []Expr
}

// Throw raises an exception. The VM has no exception model; it surfaces as a
// runtime error (sufficient for minitpl's error-path `throw`s, which the happy
// compile path never hits).
type Throw struct {
	X Expr
}

// Try is `try { Body } catch (Type $var) { ... } finally { ... }`. The VM has
// no exception class hierarchy, so catch type filters are parsed but ignored:
// the first catch clause handles any error raised in Body (a throw or a runtime
// error from a forwarded Go call). Finally always runs.
type Try struct {
	Body    []Stmt
	Catches []Catch
	Finally []Stmt
}

// Catch is one `catch (...) { ... }` clause. Var is the bound variable name
// (without `$`); the caught error is assigned to it so `echo $e` prints it.
type Catch struct {
	Var  string
	Body []Stmt
}

// Switch is `switch (Cond) { case V: ...; default: ... }`. Case bodies fall
// through unless they break (PHP semantics); the runner handles break/return.
type Switch struct {
	Cond    Expr
	Cases   []SwitchCase
	Default []Stmt
}

// SwitchCase is one `case Value:` arm of a Switch.
type SwitchCase struct {
	Value Expr
	Body  []Stmt
}

// Break exits the nearest loop or switch.
type Break struct{}

// Continue restarts the nearest loop.
type Continue struct{}

// Param is a single function parameter with an optional default value.
type Param struct {
	Name    string
	Default Expr // nil if required
}

// Field is a class property declaration (also reused for class constants).
type Field struct {
	Name       string
	Default    Expr   // nil if none
	Visibility string // "public", "protected", "private", or ""
}

func (*InlineHTML) node() {}

func (*Echo) node() {}

func (*ExprStmt) node() {}

func (*Assign) node() {}

func (*If) node() {}

func (*Foreach) node() {}

func (*For) node() {}

func (*Return) node() {}

func (*Include) node() {}

func (*FuncDecl) node() {}

func (*ClassDecl) node() {}

func (*Throw) node() {}

func (*Try) node() {}

func (*Switch) node() {}

func (*Break) node() {}

func (*Continue) node() {}

func (*Unset) node() {}

func (*InlineHTML) stmt() {}

func (*Echo) stmt() {}

func (*ExprStmt) stmt() {}

func (*Assign) stmt() {}

func (*If) stmt() {}

func (*Foreach) stmt() {}

func (*For) stmt() {}

func (*Return) stmt() {}

func (*Include) stmt() {}

func (*FuncDecl) stmt() {}

func (*ClassDecl) stmt() {}

func (*Throw) stmt() {}

func (*Try) stmt() {}

func (*Switch) stmt() {}

func (*Break) stmt() {}

func (*Continue) stmt() {}

func (*Unset) stmt() {}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// Lit is a literal scalar: nil, bool, int64, float64 or string.
type Lit struct {
	Value any
}

// Var is a `$name` reference (the `$` is stripped during parsing).
//
// A bare identifier — a constant such as `PHP_EOL`, or a magic constant such as
// `__DIR__` — is also a Var, because both resolve the same way at runtime: the
// current scope first (which is where the magic constants live), then the
// constant table. Const records which spelling the source used, so that
// printing the node back out does not turn `PHP_EOL` into `$PHP_EOL`.
type Var struct {
	Name  string
	Const bool
}

// ArrayLit is `array(...)`, `[...]` or `{...}` (map/list literal).
type ArrayLit struct {
	Items []ArrayItem
}

// ArrayItem is one entry of an ArrayLit. Key is nil for list-style entries.
type ArrayItem struct {
	Key Expr
	Val Expr
}

// Index is `Base[Index]` element access.
type Index struct {
	Base  Expr
	Index Expr
}

// PropAccess is field access. The README allows both `$obj->field` and the new
// `obj.field` notation; both parse to this node.
type PropAccess struct {
	Base Expr
	Name string
}

// Call is a free-function call: `name(args...)`.
//
// Name is the primary (possibly namespace-qualified) function name. Fallback is
// the global-namespace name to try if Name is undefined — PHP resolves an
// unqualified call inside a namespace by first looking in the current namespace
// and then falling back to the global function of the same short name. Fallback
// is "" for calls that need no fallback (the common, non-namespaced case), in
// which case the call resolves as a bare env identifier exactly as before.
type Call struct {
	Name     string
	Fallback string
	Args     []Expr
	Bare     bool // exit/die used without parentheses
}

// MethodCall is `Base->method(args...)` or `Base.method(args...)`.
type MethodCall struct {
	Base   Expr
	Method string
	Args   []Expr
}

// New is `new ClassName` / `new ClassName(args...)`.
type New struct {
	Class string
	Args  []Expr
}

// Unary is a prefix/postfix operator: "!", "-", "+", "++", "--".
type Unary struct {
	Op      string
	X       Expr
	Postfix bool
}

// Parenthesized preserves explicit grouping from the source expression.
type Parenthesized struct {
	X Expr
}

// UnwrapParenthesized returns the expression inside any explicit grouping.
// Consumers that inspect expression shape rather than evaluate it should use
// this so parentheses remain semantically transparent.
func UnwrapParenthesized(e Expr) Expr {
	for {
		p, ok := e.(*Parenthesized)
		if !ok {
			return e
		}
		e = p.X
	}
}

// Binary is an infix operator. Op covers arithmetic (+ - * / %), string concat
// ("."), comparison (== != === !== < <= > >=) and logical (&& ||).
type Binary struct {
	Op    string
	Left  Expr
	Right Expr
}

// Ternary is `Cond ? Then : Else`.
type Ternary struct {
	Cond Expr
	Then Expr
	Else Expr
}

// ClassConst is `Class::NAME` / `self::NAME` class-constant access. The
// pseudo-constant `Class::class` is spelled with Name "class" and resolves to
// the fully-qualified class name as a string.
type ClassConst struct {
	Class string
	Name  string
}

// StaticCall is `Class::method(args...)`, including the `self::` and `static::`
// spellings. It carries no receiver: the runtime runs the declaration against a
// class rather than an instance.
type StaticCall struct {
	Class  string
	Method string
	Args   []Expr
}

// StaticProp is `Class::$name` / `self::$name` static-property access. Unlike a
// PropAccess it names storage owned by the class, shared by every instance, so
// it is both read and assigned through the runtime's per-class static table.
type StaticProp struct {
	Class string
	Name  string
}

// Invoke calls a callable held in a value rather than named at the call site:
// `$fn($x)`, `$this->handlers[0]($x)`, `(self::$includeFile)($file)`. The
// callee is resolved through Runtime.Callable, so every PHP callable spelling
// (closure, "func", array($obj, "method")) works.
type Invoke struct {
	Callee Expr
	Args   []Expr
}

// Cast is a type cast like `(bool)$x`, `(int)$x`, `(string)$x`, `(array)$x`.
type Cast struct {
	Type string
	X    Expr
}

// Closure is an anonymous function expression `function($a,$b) use ($c){ ... }`.
// minitpl uses one as the usort() comparator; composer's generated autoloader
// uses the `use (...)` capture form, so Uses records the captured names. Static
// marks `static function(){}`, which PHP declares to have no `$this`.
type Closure struct {
	Params []Param
	Uses   []ClosureUse
	Body   []Stmt
	Static bool
}

// ClosureUse is one entry of a closure's `use (...)` capture list. ByRef marks
// `&$name`; the runtime has no reference values, so a by-reference capture
// binds the same value a by-value one does.
type ClosureUse struct {
	Name  string
	ByRef bool
}

// AssignExpr is assignment used as an expression, e.g. the PHP idiom
// `if (($x = f()) !== false)`. The README forbids assignment in conditions, but
// minitpl relies on it, so it is supported with Var/Index/Prop targets. As a
// statement it is lowered to *Assign by the parser.
type AssignExpr struct {
	Target Expr
	Op     string
	Value  Expr
	Line   int
}

// ListExpr is `list($a, $b, ...)`, valid only as an assignment target. Elements
// may be nil for skipped positions (`list(, $b)`).
type ListExpr struct {
	Elems []Expr
}

func (*Lit) node() {}

func (*Var) node() {}

func (*ArrayLit) node() {}

func (*Index) node() {}

func (*PropAccess) node() {}

func (*Call) node() {}

func (*MethodCall) node() {}

func (*New) node() {}

func (*Unary) node() {}

func (*Parenthesized) node() {}

func (*Binary) node() {}

func (*Ternary) node() {}

func (*ClassConst) node() {}

func (*Cast) node() {}

func (*Closure) node() {}

func (*AssignExpr) node() {}

func (*ListExpr) node() {}

func (*StaticCall) node() {}

func (*StaticProp) node() {}

func (*Invoke) node() {}

func (*Include) expr() {}

func (*Lit) expr() {}

func (*Var) expr() {}

func (*ArrayLit) expr() {}

func (*Index) expr() {}

func (*PropAccess) expr() {}

func (*Call) expr() {}

func (*MethodCall) expr() {}

func (*New) expr() {}

func (*Unary) expr() {}

func (*Parenthesized) expr() {}

func (*Binary) expr() {}

func (*Ternary) expr() {}

func (*ClassConst) expr() {}

func (*Cast) expr() {}

func (*Closure) expr() {}

func (*AssignExpr) expr() {}

func (*ListExpr) expr() {}

func (*StaticCall) expr() {}

func (*StaticProp) expr() {}

func (*Invoke) expr() {}
