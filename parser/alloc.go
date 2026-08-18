package parser

import "github.com/titpetric/phpscript/model"

// Allocation strategy for the AST.
//
// Parsing a 10 KB PHP file builds a few thousand AST nodes and a few hundred
// child slices. Both were allocated one object at a time; this file replaces
// that with two chunked structures, mirroring parser/tokenizer.go's emitArr.
// See docs/allocation-performance.md rules 6 and 7.
//
//   - nodeChunk carves nodes of one concrete type out of a backing array.
//     Every node keeps its own address, is handed out exactly once and is
//     never reset or reused, so pointer identity is preserved, which
//     runner.compileExpr relies on for its map[model.Expr]*compiledExpr cache.
//     The trade is retention: a chunk stays reachable while any node in it
//     does. That costs nothing here because the AST is retained as a whole
//     (a *model.Program), and a failed parse drops all of it at once.
//
//   - scratch is a per-type stack used to collect the children of a node
//     whose count is only known once parsed (call arguments, statement
//     bodies, parameters, array items). Pushing onto the shared stack and
//     copying out once yields one exactly-sized allocation per list instead
//     of the 1→2→4→8 growth of appending to a nil slice. It is re-entrant:
//     nested lists push and pop above the outer mark, so the outer elements
//     stay contiguous.

const (
	nodeChunkMin = 16
	nodeChunkMax = 128
)

// nodeChunk hands out individually addressable *T values from chunked backing
// arrays. Chunks start small and double so a two-line script does not pay for
// a large block and a whole file amortises the per-node allocation away.
type nodeChunk[T any] struct {
	free []T
	size int
}

func (c *nodeChunk[T]) new() *T {
	if len(c.free) == 0 {
		switch {
		case c.size == 0:
			c.size = nodeChunkMin
		case c.size < nodeChunkMax:
			c.size *= 2
		}
		c.free = make([]T, c.size)
	}
	n := &c.free[0]
	c.free = c.free[1:]
	return n
}

// scratch is a re-entrant stack of pending list elements.
type scratch[T any] struct {
	buf []T
}

// mark records the current top of the stack.
func (s *scratch[T]) mark() int { return len(s.buf) }

func (s *scratch[T]) push(v T) { s.buf = append(s.buf, v) }

// take pops everything pushed since mark and returns it as a slice with
// cap == len. It returns nil for an empty list, matching what appending to a
// nil slice produced before.
func (s *scratch[T]) take(mark int) []T {
	n := len(s.buf) - mark
	if n == 0 {
		s.buf = s.buf[:mark]
		return nil
	}
	out := make([]T, n)
	copy(out, s.buf[mark:])
	s.buf = s.buf[:mark]
	return out
}

// drop abandons everything pushed since mark. Used on error paths so a
// recovered parser does not leak elements onto the stack.
func (s *scratch[T]) drop(mark int) { s.buf = s.buf[:mark] }

// ---------------------------------------------------------------------------
// Node constructors
//
// One per chunked node type. Chunk memory is zeroed by make, and every node is
// handed out once, so a constructor that leaves a field unset is equivalent to
// a composite literal that omits it.
// ---------------------------------------------------------------------------

func (p *parser) newVar(name string) *model.Var {
	n := p.varNodes.new()
	n.Name = name
	return n
}

// newConstRef allocates the Var node for a bare identifier (a constant).
func (p *parser) newConstRef(name string) *model.Var {
	n := p.varNodes.new()
	n.Name = name
	n.Const = true
	return n
}

func (p *parser) newLit(v any) *model.Lit {
	n := p.litNodes.new()
	n.Value = v
	return n
}

func (p *parser) newBinary(op string, left, right model.Expr) *model.Binary {
	n := p.binaryNodes.new()
	n.Op, n.Left, n.Right = op, left, right
	return n
}

func (p *parser) newIndex(base, index model.Expr) *model.Index {
	n := p.indexNodes.new()
	n.Base, n.Index = base, index
	return n
}

func (p *parser) newProp(base model.Expr, name string) *model.PropAccess {
	n := p.propNodes.new()
	n.Base, n.Name = base, name
	return n
}

func (p *parser) newMethodCall(base model.Expr, method string, args []model.Expr) *model.MethodCall {
	n := p.methodNodes.new()
	n.Base, n.Method, n.Args = base, method, args
	return n
}

func (p *parser) newCall(name, fallback string, args []model.Expr) *model.Call {
	n := p.callNodes.new()
	n.Name, n.Fallback, n.Args = name, fallback, args
	return n
}

func (p *parser) newParen(x model.Expr) *model.Parenthesized {
	n := p.parenNodes.new()
	n.X = x
	return n
}

func (p *parser) newUnary(op string, x model.Expr, postfix bool) *model.Unary {
	n := p.unaryNodes.new()
	n.Op, n.X, n.Postfix = op, x, postfix
	return n
}

func (p *parser) newAssignExpr(target model.Expr, op string, value model.Expr, line int) *model.AssignExpr {
	n := p.assignExprNodes.new()
	n.Target, n.Op, n.Value, n.Line = target, op, value, line
	return n
}

func (p *parser) newExprStmt(x model.Expr) *model.ExprStmt {
	n := p.exprStmtNodes.new()
	n.X = x
	return n
}

func (p *parser) newAssign(target model.Expr, op string, value model.Expr) *model.Assign {
	n := p.assignNodes.new()
	n.Target, n.Op, n.Value = target, op, value
	return n
}

func (p *parser) newArrayLit(items []model.ArrayItem) *model.ArrayLit {
	n := p.arrayLitNodes.new()
	n.Items = items
	return n
}

func (p *parser) newEcho(args []model.Expr) *model.Echo {
	n := p.echoNodes.new()
	n.Args = args
	return n
}
