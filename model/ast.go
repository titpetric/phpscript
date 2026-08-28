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
	// NamespaceLine is the source line of the namespace declaration, which is
	// not a statement of its own. The formatter needs it to place the comments
	// written above it.
	NamespaceLine int
	// SourceSpans records original statement lines when Program came from the
	// parser. Consumers may ignore it; the formatter uses it to retain a single
	// intentional blank line between statements.
	SourceSpans map[Stmt]SourceSpan
	// AnonClasses holds the declaration of every anonymous class written in the
	// file, in source order. An anonymous class is declared inside an
	// expression rather than by a statement, so a consumer that walks Stmts
	// looking for a ClassDecl does not see one; whatever registers or checks
	// the file's classes reads this alongside Stmts.
	AnonClasses []*ClassDecl
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

	// Line is the 1-based line the annotation was written on, for a
	// diagnostic that names where to fix it. Zero when the source is unknown.
	Line int
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
	// ElseLine is the source line of the `else` or `elseif` keyword, which is
	// not a statement of its own. It marks where the then arm ends, so the
	// formatter can tell a comment written above the keyword from one written
	// below it.
	ElseLine int
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
	ReturnType string // declared `: Type`, kept for printing only
	Static     bool
	Abstract   bool // declaration only; Body is empty
	// ByRef is `function &name()`. The runtime has no reference values, so
	// the function returns by value; the marker is kept so the formatter
	// prints the declaration back and the linter can report it.
	ByRef bool
}

// ClassDecl is a trimmed-down class: fields + methods + class constants, no
// inheritance. The `abstract`, `final` and `readonly` modifiers are tolerated
// (parsed) but not enforced (README omits abstract classes; minitpl's Hook is
// abstract only to declare constants).
//
// Parent is recorded so a file the formatter rewrites prints back what it read:
// a name the AST cannot hold is a name the formatter deletes. Nothing in runner
// may read it. phpscript has no inheritance, a catch clause filters on a class
// name and `instanceof` is name equality. See docs/design.md.
//
// Implements is recorded for the same reason and is also checked, by
// CheckInterfaces: every method the listed interfaces name must be declared by
// this class. The check confers nothing; it only reports what is missing.
type ClassDecl struct {
	Name       string
	Parent     string   // `extends Name`, recorded for printing, never inherited from
	Implements []string // `implements A, B`, a contract this class must declare
	Abstract   bool
	Final      bool
	Readonly   bool
	Fields     []Field
	Statics    []Field // `static $name = expr` properties, referenced as Class::$name
	Consts     []Field // class constants (Name + value Expr), referenced as Class::NAME
	Methods    []*FuncDecl
}

// InterfaceDecl is `interface Name extends A, B { ... }`.
//
// An interface is a declaration contract and nothing else. It names method
// signatures and constants, and a class that says `implements` must declare
// every one of those methods itself. No member is ever acquired from it, no
// method body comes from it, and `instanceof` does not consult it, so
// `$a instanceof SomeInterface` stays false. See docs/design.md.
type InterfaceDecl struct {
	Name string
	// Extends is `extends A, B`. The extended interfaces widen the contract:
	// the names a class is checked against are the union of what every listed
	// interface declares. Nothing is inherited, because there is no member to
	// inherit; an interface declares no body and holds no storage.
	Extends []string
	Consts  []Field
	// Methods are signatures: the parameters, the return type and the modifiers
	// are recorded so the formatter prints the declaration back, and Body is
	// always nil. None of them is ever called; they are only names to check a
	// class against.
	Methods []*FuncDecl
}

// Use is `use A\B\C;`, `use A\B\C as D;` or `use function f;`. The parser
// resolves an import to a fully-qualified name while parsing, so the statement
// has no effect at runtime. It is still kept in the AST: the formatter rewrites
// files in place, and a node the printer cannot see is a node it deletes.
type Use struct {
	Kind    string // "", "function" or "const"
	Imports []UseImport
}

// UseImport is one name of a `use` statement. Alias is set only for the
// `as` spelling; the short name of Path is implied otherwise.
type UseImport struct {
	Path  string
	Alias string
}

// Declare is `declare(strict_types=1);` or `declare(ticks=1) { ... }`. The
// runtime has one set of semantics and no directive varies it, so the
// directives are recorded for printing and otherwise ignored; a block form
// still runs its body.
type Declare struct {
	Directives []DeclareDirective
	Body       []Stmt
	Block      bool // the `declare(...) { ... }` spelling, even with an empty body
}

// DeclareDirective is one `name=value` pair of a Declare.
type DeclareDirective struct {
	Name  string
	Value Expr
}

// Unset is `unset($a, $b[$k], $o->p, C::$s)`. Each target is removed from the
// scope, array or property bag holding it.
type Unset struct {
	Targets []Expr
}

// StaticVar is `static $x [= expr][, $y ...];` inside a function body. Each
// initializer runs once per function lifetime; the bindings persist across
// calls (per closure value for closures), which is PHP's function-static
// semantics. Storage lives on the runtime, keyed by this node's address.
type StaticVar struct {
	Vars []StaticVarDecl
}

// StaticVarDecl is one `$name [= expr]` entry of a StaticVar.
type StaticVarDecl struct {
	Name    string
	Default Expr // nil when the declaration has no initializer
}

// Global is `global $x[, $y];`. The statement parses into a node so the
// formatter can print it back; at runtime it is a documented no-op — the
// variable stays unset (docs/design.md), and `phpscript lint` reports it.
type Global struct {
	Names []string
}

// Throw raises an exception. The VM has no exception model; it surfaces as a
// runtime error (sufficient for minitpl's error-path `throw`s, which the happy
// compile path never hits).
type Throw struct {
	X Expr
}

// Try is `try { Body } catch (Type $var) { ... } finally { ... }`. The first
// clause whose declared type matches the error raised in Body (a throw or a
// runtime error from a forwarded Go call) handles it; an error no clause
// matches keeps propagating. Finally always runs either way. Matching is
// by Go error type rather than a PHP class hierarchy, so two throwable names
// backed by the same type cannot be told apart.
type Try struct {
	Body        []Stmt
	Catches     []Catch
	Finally     []Stmt
	FinallyLine int // source line of the `finally` keyword, for comment placement
}

// Catch is one `catch (...) { ... }` clause. Var is the bound variable name
// (without `$`); the caught error is assigned to it so `echo $e` prints it.
// Type is the declared filter, `Exception` or the union form `A|B`, which
// decides whether this clause handles the error. PHP requires it: a catch
// clause printed without it is a syntax error.
type Catch struct {
	Type string
	Var  string
	Body []Stmt
	Line int // source line of the `catch` keyword, for comment placement
}

// Switch is `switch (Cond) { case V: ...; default: ... }`. Case bodies fall
// through unless they break (PHP semantics); the runner handles break/return.
type Switch struct {
	Cond    Expr
	Cases   []SwitchCase
	Default []Stmt
}

// SwitchCase is one `case Value:` arm of a Switch. Line is the source line of
// the `case` keyword, which the formatter uses to place comments.
type SwitchCase struct {
	Value Expr
	Body  []Stmt
	Line  int
}

// Break exits the nearest loop or switch. Line is the source line it was
// written on, which also gives the node an address of its own: Go hands every
// zero-sized allocation the same one, and the formatter keys source spans by
// node.
type Break struct {
	Line int
}

// Continue restarts the nearest loop. Line is the source line, for the reason
// given on Break.
type Continue struct {
	Line int
}

// Param is a single function parameter with an optional default value.
//
// The runtime binds a parameter by name and ignores everything declared around
// it, but the formatter rewrites files in place, so the declaration is kept:
// Modifiers holds the `public readonly` of a promoted constructor property,
// Type the type hint, and ByRef and Variadic the `&` and `...` markers.
type Param struct {
	Name      string
	Default   Expr // nil if required
	Modifiers string
	Type      string
	ByRef     bool
	Variadic  bool
}

// Field is a class property declaration (also reused for class constants).
type Field struct {
	Name       string
	Default    Expr   // nil if none
	Visibility string // "public", "protected", "private", or ""
	Type       string // declared type hint, kept for printing only
	// Span is the source-line range of the declaration when Field came from
	// the parser. The formatter uses it to keep the blank lines an author put
	// between groups of properties.
	Span SourceSpan
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

func (*InterfaceDecl) node() {}

func (*Throw) node() {}

func (*Try) node() {}

func (*Switch) node() {}

func (*Break) node() {}

func (*Continue) node() {}

func (*Unset) node() {}

func (*StaticVar) node() {}

func (*Global) node() {}

func (*Use) node() {}

func (*Declare) node() {}

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

func (*InterfaceDecl) stmt() {}

func (*Throw) stmt() {}

func (*Try) stmt() {}

func (*Switch) stmt() {}

func (*Break) stmt() {}

func (*Continue) stmt() {}

func (*Unset) stmt() {}

func (*StaticVar) stmt() {}

func (*Global) stmt() {}

func (*Use) stmt() {}

func (*Declare) stmt() {}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// Lit is a literal scalar: nil, bool, int64, float64 or string.
//
// Raw holds the source spelling, quotes included, for a string literal that
// came from a parsed file. Decoding a string is lossy (`'$a'` and `"\$a"`
// decode to the same value, and only one of them can be re-encoded from it),
// so the formatter prints from Raw and falls back to encoding Value for nodes
// that were built rather than parsed.
type Lit struct {
	Value any
	Raw   string
}

// Interp is a double-quoted string literal that embeds expressions, such as
// `"hello $name"` or `"{$row['id']}: $count"`.
//
// Parts alternates literal runs, held as *Lit strings with their escapes already
// decoded, and the expressions written between them. Evaluating one converts
// every part to a string and joins them, which is what `.` concatenation does,
// so an Interp and the equivalent concatenation produce the same value.
//
// Raw holds the source spelling, quotes included, for the same reason Lit does:
// the formatter rewrites files in place and prints the literal the way it was
// written rather than re-encoding it.
type Interp struct {
	Parts []Expr
	Raw   string
}

// Var is a `$name` reference (the `$` is stripped during parsing).
//
// A bare identifier, a constant such as `PHP_EOL` or a magic constant such as
// `__DIR__`, is also a Var, because both resolve the same way at runtime: the
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
// the global-namespace name to try if Name is undefined. PHP resolves an
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
//
// Decl is set for an anonymous class, `new class { ... }`, and holds the
// declaration written in place of the name. Class still names the class, using
// a name the parser synthesized, so that everything downstream of the parser
// resolves an anonymous class the same way it resolves a written one. The
// declarations a program builds this way are collected on Program.AnonClasses,
// because they are not statements and nothing else would find them.
type New struct {
	Class string
	Args  []Expr
	Decl  *ClassDecl
}

// Ref is the reference marker `&$var` written in expression position, as in
// `$a = &$b`. The runtime has no reference values (docs/design.md, "`&`
// outside `foreach`"), so evaluating a Ref evaluates X and binds by value;
// the node exists so the formatter prints the source back as written and the
// linter can report the marker.
type Ref struct {
	X Expr
}

// Unary is a prefix/postfix operator: "!", "-", "+", "~", "++", "--".
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

// Binary is an infix operator. Op covers arithmetic (+ - * / % **), string
// concat ("."), comparison (== != === !== < <= > >=), logical (&& ||) and
// bitwise (& | ^ << >>).
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
// spellings. The runtime forwards the current receiver when an instance method
// reaches a non-static declaration through this syntax; genuinely static calls
// run against the class alone.
//
// The variable spelling `Class::$m(args...)` calls the method whose name is
// held in `$m`: MethodExpr carries that expression and Method is empty.
type StaticCall struct {
	Class      string
	Method     string
	MethodExpr Expr // set for `Class::$m(...)`; Method is "" then
	Args       []Expr
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
	Params     []Param
	Uses       []ClosureUse
	Body       []Stmt
	ReturnType string // declared `: Type`, kept for printing only
	Static     bool
	ByRef      bool // `function &() {}`; returns by value, kept for printing
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

func (*Interp) node() {}

func (*Var) node() {}

func (*ArrayLit) node() {}

func (*Index) node() {}

func (*PropAccess) node() {}

func (*Call) node() {}

func (*MethodCall) node() {}

func (*New) node() {}

func (*Ref) node() {}

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

func (*Interp) expr() {}

func (*Var) expr() {}

func (*ArrayLit) expr() {}

func (*Index) expr() {}

func (*PropAccess) expr() {}

func (*Call) expr() {}

func (*MethodCall) expr() {}

func (*New) expr() {}

func (*Ref) expr() {}

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
