package runner

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/model"
)

// Transpiler lowers expression AST nodes into type-agnostic expr-lang source
// that delegates PHP-specific behavior to runtime helpers.
type Transpiler struct {
	// vars collects every variable referenced by the expression, in first-use
	// order, so the runtime knows which scope values to bind into the expr env.
	// A slice with a linear scan beats a map here: expressions reference a
	// handful of variables and the result is handed to the compiled expression
	// as a slice anyway.
	vars []string
	// idents holds the expr identifier (varIdent) for each entry of vars. It is
	// collected here because emit already builds the string.
	idents []string
	// calls collects, deduplicated and in first-use order, every function name
	// emitted as a bare env identifier (see emitCall). It is what lets the
	// runtime populate an evaluation environment with the handful of registered
	// functions an expression actually calls instead of the whole table.
	// Namespaced and fallback calls are not collected: they dispatch through the
	// __func helper, which resolves against the function table directly.
	calls []string
	// closures collects anonymous functions encountered while emitting, keyed by
	// the synthetic env identifier they are bound to (__cl0, __cl1, ...). The
	// runtime turns each into a callable before evaluation. Both maps stay nil
	// until something is put in them: most expressions have neither a closure nor
	// an evaluation marker, and the maps outlive the transpiler (the compiled
	// expression retains them), so they cannot be reused.
	closures map[string]*model.Closure
	exprs    map[string]model.Expr
}

// NewTranspiler returns a fresh transpiler.
func NewTranspiler() *Transpiler {
	return &Transpiler{}
}

// transpilers pools transpilers across compiles. A transpiler holds no state
// between runs beyond the collections Reset drops, and compiling an expression
// is a leaf operation (emit never re-enters the runtime), so one instance is
// enough per goroutine in flight.
var transpilers sync.Pool

// acquireTranspiler returns a reset transpiler from the pool.
func acquireTranspiler() *Transpiler {
	if t, ok := transpilers.Get().(*Transpiler); ok {
		t.Reset()
		return t
	}
	return NewTranspiler()
}

// releaseTranspiler returns t to the pool. The caller must not hold on to the
// slices returned by Transpile, because Reset keeps their backing arrays.
func releaseTranspiler(t *Transpiler) {
	t.Reset()
	transpilers.Put(t)
}

// Reset clears the state collected by the last Transpile, keeping the backing
// arrays of the variable slices.
func (t *Transpiler) Reset() {
	t.vars = t.vars[:0]
	t.idents = t.idents[:0]
	t.calls = t.calls[:0]
	t.closures = nil
	t.exprs = nil
}

// Transpile converts e into expr-lang source and returns the source plus the
// set of variable names it references, in first-use order.
//
// The returned slice aliases the transpiler's own storage; copy it (or use
// Idents, which is kept in step with it) before the transpiler is reused.
func (t *Transpiler) Transpile(e model.Expr) (src string, vars []string, err error) {
	t.vars = t.vars[:0]
	t.idents = t.idents[:0]
	t.calls = t.calls[:0]
	t.closures = nil
	t.exprs = nil
	src, err = t.emit(e)
	if err != nil {
		return "", nil, err
	}
	return src, t.vars, nil
}

// Idents returns the expr identifiers of the variables collected during the
// last Transpile, positionally matching its vars result.
func (t *Transpiler) Idents() []string { return t.idents }

// Calls returns the function names the last Transpile emitted as bare env
// identifiers, deduplicated and in first-use order. Like Idents, it aliases the
// transpiler's own storage.
func (t *Transpiler) Calls() []string { return t.calls }

// addCall records a function name emitted as a bare env identifier. Expressions
// call a handful of distinct functions, so a linear scan beats a map for the
// same reason addVar uses one.
func (t *Transpiler) addCall(name string) {
	for _, have := range t.calls {
		if have == name {
			return
		}
	}
	t.calls = append(t.calls, name)
}

// Closures returns the anonymous functions collected during the last Transpile,
// keyed by their env identifier. It is nil when the expression has none.
func (t *Transpiler) Closures() map[string]*model.Closure { return t.closures }

// Exprs returns the sub-expressions marked for deferred evaluation during the
// last Transpile. It is nil when the expression has none.
func (t *Transpiler) Exprs() map[string]model.Expr { return t.exprs }

// addVar records a referenced variable and returns its expr identifier.
func (t *Transpiler) addVar(name string) string {
	for i, have := range t.vars {
		if have == name {
			return t.idents[i]
		}
	}
	ident := varIdent(name)
	t.vars = append(t.vars, name)
	t.idents = append(t.idents, ident)
	return ident
}

func (t *Transpiler) mark(e model.Expr) string {
	id := strconv.Itoa(len(t.exprs)) + "__expr"
	if t.exprs == nil {
		t.exprs = make(map[string]model.Expr, 1)
	}
	t.exprs[id] = e
	return id
}

// varIdent is the expr identifier used for a PHP variable. `$this` becomes the
// identifier `this`; everything else is prefixed to avoid clashing with
// forwarded function names that share the env namespace.
func varIdent(name string) string {
	if name == "this" {
		return "this"
	}
	return "v_" + name
}

func (t *Transpiler) emit(e model.Expr) (string, error) {
	switch n := e.(type) {
	case *model.Lit:
		return litSource(n.Value), nil

	case *model.Var:
		return t.addVar(n.Name), nil

	case *model.Interp:
		return t.emitInterp(n)

	case *model.Unary:
		if n.Op == "++" || n.Op == "--" {
			return "__eval(" + strconv.Quote(t.mark(n)) + ")", nil
		}
		x, err := t.emit(n.X)
		if err != nil {
			return "", err
		}
		op := n.Op
		if op == "!" {
			// expr-lang `!` requires a bool; coerce with PHP truthiness first.
			return "!__bool(" + x + ")", nil
		}
		if op == "~" {
			// expr-lang has no bitwise complement, and PHP's operates on bytes
			// for a string operand, so it is a helper rather than an operator.
			return "__bitnot(" + x + ")", nil
		}
		if op == "-" {
			// expr-lang's own `-` is a Go int64 negation, which wraps:
			// -PHP_INT_MIN would come back as PHP_INT_MIN, a positive quantity
			// with a negative sign. The helper overflows to a float instead,
			// and keeps the sign of a negative zero.
			return "__neg(" + x + ")", nil
		}
		return op + "(" + x + ")", nil

	case *model.Parenthesized:
		x, err := t.emit(n.X)
		if err != nil {
			return "", err
		}
		return "(" + x + ")", nil

	case *model.Binary:
		return t.emitBinary(n)

	case *model.Ternary:
		c, err := t.emit(n.Cond)
		if err != nil {
			return "", err
		}
		// expr ternary requires a bool condition; apply PHP truthiness.
		c = "__bool(" + c + ")"
		th, err := t.emit(n.Then)
		if err != nil {
			return "", err
		}
		el, err := t.emit(n.Else)
		if err != nil {
			return "", err
		}
		return "(" + c + ") ? (" + th + ") : (" + el + ")", nil

	case *model.ArrayLit:
		return t.emitArray(n)

	case *model.Index:
		base, err := t.emit(n.Base)
		if err != nil {
			return "", err
		}
		idx, err := t.emit(n.Index)
		if err != nil {
			return "", err
		}
		return "__index(" + base + ", " + idx + ")", nil

	case *model.PropAccess:
		base, err := t.emit(n.Base)
		if err != nil {
			return "", err
		}
		return "__get(" + base + ", " + strconv.Quote(n.Name) + ")", nil

	case *model.Call:
		return t.emitCall(n)

	case *model.ClassConst:
		return "__classconst(" + strconv.Quote(n.Class) + ", " + strconv.Quote(n.Name) + ")", nil

	case *model.StaticProp:
		return "__staticprop(" + strconv.Quote(n.Class) + ", " + strconv.Quote(n.Name) + ")", nil

	case *model.StaticCall:
		args, err := t.emitArgs(n.Args)
		if err != nil {
			return "", err
		}
		return joinCall("__static", strconv.Quote(n.Class), strconv.Quote(n.Method), args), nil

	case *model.Invoke:
		callee, err := t.emit(n.Callee)
		if err != nil {
			return "", err
		}
		args, err := t.emitArgs(n.Args)
		if err != nil {
			return "", err
		}
		return joinCall("__invoke", callee, "", args), nil

	case *model.Cast:
		x, err := t.emit(n.X)
		if err != nil {
			return "", err
		}
		return "__cast(" + strconv.Quote(n.Type) + ", " + x + ")", nil

	case *model.AssignExpr:
		v, ok := model.UnwrapParenthesized(n.Target).(*model.Var)
		if !ok {
			return "", fmt.Errorf("transpile: assignment expression supports only $var targets, got %T", n.Target)
		}
		val, err := t.emit(n.Value)
		if err != nil {
			return "", err
		}
		// Compound ops in an expression context are rare; support plain `=`.
		if n.Op != "=" && n.Op != "" {
			return "", fmt.Errorf("transpile: assignment expression op %q unsupported", n.Op)
		}
		return "__set(" + strconv.Quote(v.Name) + ", " + val + ")", nil

	case *model.Include:
		return "__eval(" + strconv.Quote(t.mark(n)) + ")", nil

	case *model.Closure:
		id := "__cl" + strconv.Itoa(len(t.closures))
		if t.closures == nil {
			t.closures = make(map[string]*model.Closure, 1)
		}
		t.closures[id] = n
		return id, nil

	case *model.MethodCall:
		base, err := t.emit(n.Base)
		if err != nil {
			return "", err
		}
		args, err := t.emitArgs(n.Args)
		if err != nil {
			return "", err
		}
		return joinCall("__call", base, strconv.Quote(n.Method), args), nil

	case *model.New:
		args, err := t.emitArgs(n.Args)
		if err != nil {
			return "", err
		}
		return joinCall("__new", strconv.Quote(n.Class), "", args), nil

	default:
		return "", fmt.Errorf("transpile: unsupported expression %T", e)
	}
}

// emitInterp emits an interpolated string literal as the concatenation it is.
// __concat applies PHP's string conversion to each side, which is what turns the
// embedded values into text, so a literal with one embedded expression and no
// surrounding text still yields a string: the empty leading run is kept for
// exactly that.
func (t *Transpiler) emitInterp(n *model.Interp) (string, error) {
	if len(n.Parts) == 0 {
		return `""`, nil
	}
	out, err := t.emit(n.Parts[0])
	if err != nil {
		return "", err
	}
	if len(n.Parts) == 1 {
		if _, ok := n.Parts[0].(*model.Lit); ok {
			return out, nil
		}
		return concat("__concat(", `""`, ", ", out, ")"), nil
	}
	for _, part := range n.Parts[1:] {
		s, err := t.emit(part)
		if err != nil {
			return "", err
		}
		out = concat("__concat(", out, ", ", s, ")")
	}
	return out, nil
}

// emitCall emits a free-function call. Free functions resolve from the env by
// name (forwarded Go symbols or user-registered/PHP functions); expr-lang
// builtins are disabled at compile time so PHP names like `count` never collide.
// By-reference output arguments that are plain variables are emitted as __ref
// setters so the shim can write the result back into scope.
func (t *Transpiler) emitCall(n *model.Call) (string, error) {
	args := make([]string, 0, len(n.Args))
	for i, a := range n.Args {
		// Inside a namespaced file Name is qualified ("MiniTPL\preg_match_all")
		// and only Fallback carries the global name the table is keyed by.
		if model.ByRefArg(n.Name, n.Fallback, i) {
			if v, ok := model.UnwrapParenthesized(a).(*model.Var); ok {
				t.addVar(v.Name)
				args = append(args, "__ref("+strconv.Quote(v.Name)+")")
				continue
			}
		}
		s, err := t.emit(a)
		if err != nil {
			return "", err
		}
		args = append(args, s)
	}
	// Namespaced names contain '\' (not a valid expr identifier) and calls that
	// carry a global fallback both dispatch through the __func helper, which
	// resolves Name then Fallback in the runtime function table. Plain, global
	// calls keep resolving as a bare env identifier (the fast, original path).
	if n.Fallback != "" || strings.ContainsRune(n.Name, '\\') {
		return joinCall("__func", strconv.Quote(n.Name), strconv.Quote(n.Fallback), args), nil
	}
	t.addCall(n.Name)
	return joinCall(n.Name, "", "", args), nil
}

// joinCall renders `name(lead1, lead2, args...)` into one presized buffer.
// Empty leading arguments are omitted, since nothing the transpiler emits is the
// empty string (a quoted empty PHP string is `""`, two characters).
func joinCall(name, lead1, lead2 string, args []string) string {
	size := len(name) + 2
	for _, a := range args {
		size += len(a) + 2
	}
	if lead1 != "" {
		size += len(lead1) + 2
	}
	if lead2 != "" {
		size += len(lead2) + 2
	}
	var b strings.Builder
	b.Grow(size)
	b.WriteString(name)
	b.WriteByte('(')
	first := true
	for _, s := range [2]string{lead1, lead2} {
		if s == "" {
			continue
		}
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(s)
	}
	for _, a := range args {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(a)
	}
	b.WriteByte(')')
	return b.String()
}

// bitCallPrefix holds the __bit call opening for each bitwise operator, spelled
// out once at package scope rather than built per expression.
var bitCallPrefix = map[string]string{
	"&":  `__bit("&", `,
	"|":  `__bit("|", `,
	"^":  `__bit("^", `,
	"<<": `__bit("<<", `,
	">>": `__bit(">>", `,
}

func (t *Transpiler) emitBinary(n *model.Binary) (string, error) {
	l, err := t.emit(n.Left)
	if err != nil {
		return "", err
	}
	if n.Op == "instanceof" {
		// A bare name on the right is the class, not a constant to resolve, so
		// it is passed as the string it is. Anything else is evaluated: PHP
		// accepts a variable holding a class name or an object there.
		if v, ok := model.UnwrapParenthesized(n.Right).(*model.Var); ok && v.Const {
			return concat("__instanceof(", l, ", ", strconv.Quote(v.Name), ")"), nil
		}
	}
	r, err := t.emit(n.Right)
	if err != nil {
		return "", err
	}
	switch n.Op {
	case ".":
		// PHP string concatenation -> helper (expr `+` is numeric/typed).
		return concat("__concat(", l, ", ", r, ")"), nil
	case "instanceof":
		return concat("__instanceof(", l, ", ", r, ")"), nil
	case "===":
		return concat("(", l, ") == (", r, ")"), nil
	case "!==":
		return concat("(", l, ") != (", r, ")"), nil
	case "&&":
		// Logical operators need bool operands in expr-lang.
		return concat("__bool(", l, ") && __bool(", r, ")"), nil
	case "||":
		return concat("__bool(", l, ") || __bool(", r, ")"), nil
	case "+":
		return concat(`__arith("+", `, l, ", ", r, ")"), nil
	case "-":
		return concat(`__arith("-", `, l, ", ", r, ")"), nil
	case "*":
		return concat(`__arith("*", `, l, ", ", r, ")"), nil
	case "/":
		return concat(`__arith("/", `, l, ", ", r, ")"), nil
	case "%":
		return concat(`__arith("%", `, l, ", ", r, ")"), nil
	case "**":
		return concat(`__arith("**", `, l, ", ", r, ")"), nil
	case "&", "|", "^", "<<", ">>":
		// expr-lang spells some of these differently (`^` is exponentiation
		// there) and none of them with PHP's string semantics, so they all go
		// through the helper. The call prefix comes from a table so that
		// emitting one costs no concatenation of its own (rule 7).
		return concat(bitCallPrefix[n.Op], l, ", ", r, ")"), nil
	case "==":
		return concat("(", l, ") == (", r, ")"), nil
	case "!=":
		return concat("(", l, ") != (", r, ")"), nil
	case "<":
		return concat("(", l, ") < (", r, ")"), nil
	case "<=":
		return concat("(", l, ") <= (", r, ")"), nil
	case ">":
		return concat("(", l, ") > (", r, ")"), nil
	case ">=":
		return concat("(", l, ") >= (", r, ")"), nil
	default:
		return "", fmt.Errorf("transpile: unsupported operator %q", n.Op)
	}
}

// concat joins five fragments into one presized string. The Go compiler folds
// the constant fragments, so this is one allocation where a Sprintf would box
// its arguments and grow a buffer.
func concat(a, b, c, d, e string) string {
	var sb strings.Builder
	sb.Grow(len(a) + len(b) + len(c) + len(d) + len(e))
	sb.WriteString(a)
	sb.WriteString(b)
	sb.WriteString(c)
	sb.WriteString(d)
	sb.WriteString(e)
	return sb.String()
}

func (t *Transpiler) emitArray(n *model.ArrayLit) (string, error) {
	pairs := make([]string, 0, len(n.Items))
	for _, it := range n.Items {
		key := "nil"
		if it.Key != nil {
			k, err := t.emit(it.Key)
			if err != nil {
				return "", err
			}
			key = k
		}
		val, err := t.emit(it.Val)
		if err != nil {
			return "", err
		}
		pairs = append(pairs, concat("__pair(", key, ", ", val, ")"))
	}
	return joinCall("__array", "", "", pairs), nil
}

func (t *Transpiler) emitArgs(args []model.Expr) ([]string, error) {
	out := make([]string, 0, len(args))
	for _, a := range args {
		s, err := t.emit(a)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// litSource renders a literal scalar as expr source.
func litSource(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return strconv.Quote(x)
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		// Keep a float marker so expr sees a float literal: `0.0` rendered as
		// bare `0` would become an int and `-0.0` would echo as 0, not -0.
		s := strconv.FormatFloat(x, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	default:
		return fmt.Sprintf("%v", x)
	}
}
