package runner

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// Transpiler lowers expression AST nodes into type-agnostic expr-lang source
// that delegates PHP-specific behavior to runtime helpers.
type Transpiler struct {
	// vars collects every variable referenced by the expression so the runtime
	// knows which scope values to bind into the expr env.
	vars map[string]struct{}
	// closures collects anonymous functions encountered while emitting, keyed by
	// the synthetic env identifier they are bound to (__cl0, __cl1, ...). The
	// runtime turns each into a callable before evaluation.
	closures map[string]*model.Closure
	exprs    map[string]model.Expr
}

// NewTranspiler returns a fresh transpiler.
func NewTranspiler() *Transpiler {
	return &Transpiler{vars: map[string]struct{}{}, closures: map[string]*model.Closure{}, exprs: map[string]model.Expr{}}
}

// Transpile converts e into expr-lang source and returns the source plus the
// sorted set of variable names it references.
func (t *Transpiler) Transpile(e model.Expr) (src string, vars []string, err error) {
	t.vars = map[string]struct{}{}
	t.closures = map[string]*model.Closure{}
	t.exprs = map[string]model.Expr{}
	src, err = t.emit(e)
	if err != nil {
		return "", nil, err
	}
	for name := range t.vars {
		vars = append(vars, name)
	}
	sort.Strings(vars)
	return src, vars, nil
}

// Closures returns the anonymous functions collected during the last Transpile,
// keyed by their env identifier.
func (t *Transpiler) Closures() map[string]*model.Closure { return t.closures }

func (t *Transpiler) Exprs() map[string]model.Expr { return t.exprs }

func (t *Transpiler) mark(e model.Expr) string {
	id := fmt.Sprintf("%d__expr", len(t.exprs))
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
		t.vars[n.Name] = struct{}{}
		return varIdent(n.Name), nil

	case *model.Unary:
		if n.Op == "++" || n.Op == "--" {
			id := t.mark(n)
			return fmt.Sprintf("__eval(%q)", id), nil
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
		return op + "(" + x + ")", nil

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
		return fmt.Sprintf("__index(%s, %s)", base, idx), nil

	case *model.PropAccess:
		base, err := t.emit(n.Base)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("__get(%s, %q)", base, n.Name), nil

	case *model.Call:
		return t.emitCall(n)

	case *model.ClassConst:
		return fmt.Sprintf("__classconst(%q, %q)", n.Class, n.Name), nil

	case *model.Cast:
		x, err := t.emit(n.X)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("__cast(%q, %s)", n.Type, x), nil

	case *model.AssignExpr:
		v, ok := n.Target.(*model.Var)
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
		return fmt.Sprintf("__set(%q, %s)", v.Name, val), nil

	case *model.Closure:
		id := fmt.Sprintf("__cl%d", len(t.closures))
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
		parts := append([]string{base, strconv.Quote(n.Method)}, args...)
		return fmt.Sprintf("__call(%s)", strings.Join(parts, ", ")), nil

	case *model.New:
		args, err := t.emitArgs(n.Args)
		if err != nil {
			return "", err
		}
		parts := append([]string{strconv.Quote(n.Class)}, args...)
		return fmt.Sprintf("__new(%s)", strings.Join(parts, ", ")), nil

	default:
		return "", fmt.Errorf("transpile: unsupported expression %T", e)
	}
}

// byRefArgs lists, per function, the argument positions that are passed by
// reference (output parameters). minitpl only needs preg_match_all's $matches.
var byRefArgs = map[string]map[int]bool{
	"preg_match_all": {2: true},
	"preg_match":     {2: true},
}

// emitCall emits a free-function call. Free functions resolve from the env by
// name (forwarded Go symbols or user-registered/PHP functions); expr-lang
// builtins are disabled at compile time so PHP names like `count` never collide.
// By-reference output arguments that are plain variables are emitted as __ref
// setters so the shim can write the result back into scope.
func (t *Transpiler) emitCall(n *model.Call) (string, error) {
	refs := byRefArgs[n.Name]
	args := make([]string, 0, len(n.Args))
	for i, a := range n.Args {
		if refs[i] {
			if v, ok := a.(*model.Var); ok {
				t.vars[v.Name] = struct{}{}
				args = append(args, fmt.Sprintf("__ref(%q)", v.Name))
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
		parts := append([]string{strconv.Quote(n.Name), strconv.Quote(n.Fallback)}, args...)
		return fmt.Sprintf("__func(%s)", strings.Join(parts, ", ")), nil
	}
	return fmt.Sprintf("%s(%s)", n.Name, strings.Join(args, ", ")), nil
}

func (t *Transpiler) emitBinary(n *model.Binary) (string, error) {
	l, err := t.emit(n.Left)
	if err != nil {
		return "", err
	}
	r, err := t.emit(n.Right)
	if err != nil {
		return "", err
	}
	switch n.Op {
	case ".":
		// PHP string concatenation -> helper (expr `+` is numeric/typed).
		return fmt.Sprintf("__concat(%s, %s)", l, r), nil
	case "===":
		return fmt.Sprintf("(%s) == (%s)", l, r), nil
	case "!==":
		return fmt.Sprintf("(%s) != (%s)", l, r), nil
	case "&&", "||":
		// Logical operators need bool operands in expr-lang.
		return fmt.Sprintf("__bool(%s) %s __bool(%s)", l, n.Op, r), nil
	case "==", "!=", "<", "<=", ">", ">=", "+", "-", "*", "/", "%":
		return fmt.Sprintf("(%s) %s (%s)", l, n.Op, r), nil
	default:
		return "", fmt.Errorf("transpile: unsupported operator %q", n.Op)
	}
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
		pairs = append(pairs, fmt.Sprintf("__pair(%s, %s)", key, val))
	}
	return fmt.Sprintf("__array(%s)", strings.Join(pairs, ", ")), nil
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
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", x)
	}
}
