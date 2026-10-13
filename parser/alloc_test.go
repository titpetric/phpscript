package parser

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestNodeChunkKeepsPointerIdentity covers the property the chunked allocator
// has to hold: every node is handed out once and keeps its own address, so a
// map keyed by model.Expr tells two nodes apart. A chunk reset or reused would
// make two expressions share an address, and the compiled-expression cache in
// runner would answer one with the other's code.
func TestNodeChunkKeepsPointerIdentity(t *testing.T) {
	var chunk nodeChunk[model.Lit]

	// Past nodeChunkMax, so several backing arrays are carved and the
	// addresses have to stay distinct across the boundary between them.
	const count = 400
	seen := make(map[*model.Lit]bool, count)
	nodes := make([]*model.Lit, 0, count)
	for i := range count {
		node := chunk.new()
		if seen[node] {
			t.Fatalf("node %d repeats an address already handed out", i)
		}
		seen[node] = true
		node.Value = int64(i)
		nodes = append(nodes, node)
	}

	// Writing through one node never reaches another, so a field set on a
	// later node does not rewrite an earlier one in the same array.
	for i, node := range nodes {
		if node.Value != int64(i) {
			t.Fatalf("node %d holds %#v, want the value written into it", i, node.Value)
		}
	}
}

// TestNodeChunkGrows covers the sizing: a chunk starts at nodeChunkMin so a
// two-line script pays for a small block, doubles while it is under
// nodeChunkMax, and stops there so one long file does not reserve an array
// the rest of the parse cannot use.
func TestNodeChunkGrows(t *testing.T) {
	var chunk nodeChunk[model.Var]

	chunk.new()
	if chunk.size != nodeChunkMin {
		t.Errorf("the first chunk is %d, want %d", chunk.size, nodeChunkMin)
	}

	// Drain the first block and one node past it, which carves the second.
	for range nodeChunkMin {
		chunk.new()
	}
	if chunk.size != nodeChunkMin*2 {
		t.Errorf("the second chunk is %d, want %d", chunk.size, nodeChunkMin*2)
	}

	for range nodeChunkMax * 4 {
		chunk.new()
	}
	if chunk.size != nodeChunkMax {
		t.Errorf("the chunk grew to %d, want it capped at %d", chunk.size, nodeChunkMax)
	}
}

// TestScratchTakesExactlySized covers what the shared stack buys: one
// exactly-sized allocation per list, so a slice of call arguments has cap
// equal to len and never the slack appending would leave. An empty list
// answers nil, matching what appending to a nil slice produced.
func TestScratchTakesExactlySized(t *testing.T) {
	var stack scratch[model.Expr]

	mark := stack.mark()
	if taken := stack.take(mark); taken != nil {
		t.Errorf("take on an empty list = %#v, want nil", taken)
	}

	mark = stack.mark()
	for i := range 5 {
		stack.push(&model.Lit{Value: int64(i)})
	}
	taken := stack.take(mark)
	if len(taken) != 5 || cap(taken) != 5 {
		t.Fatalf("take = len %d cap %d, want 5 and 5", len(taken), cap(taken))
	}
	for i, item := range taken {
		if lit, ok := item.(*model.Lit); !ok || lit.Value != int64(i) {
			t.Errorf("element %d = %#v", i, item)
		}
	}
	if stack.mark() != 0 {
		t.Errorf("the stack holds %d elements after take, want it emptied", stack.mark())
	}
}

// TestScratchIsReentrant covers nesting, the state a call argument holding an
// array literal parses in: the inner list pushes and pops above the outer
// mark, so the outer elements stay contiguous and come back in the order they
// were pushed.
func TestScratchIsReentrant(t *testing.T) {
	var stack scratch[model.Expr]

	outer := stack.mark()
	stack.push(&model.Lit{Value: "first"})

	inner := stack.mark()
	stack.push(&model.Lit{Value: "nested-a"})
	stack.push(&model.Lit{Value: "nested-b"})
	nested := stack.take(inner)
	if len(nested) != 2 {
		t.Fatalf("the inner list = %#v, want two elements", nested)
	}

	stack.push(&model.Lit{Value: "second"})
	got := stack.take(outer)
	if len(got) != 2 {
		t.Fatalf("the outer list = %#v, want two elements", got)
	}
	for i, want := range []string{"first", "second"} {
		if lit, ok := got[i].(*model.Lit); !ok || lit.Value != want {
			t.Errorf("outer element %d = %#v, want %q", i, got[i], want)
		}
	}
}

// TestScratchDropsOnFailure covers the error path: a parse that gave up
// abandons what it pushed, so the next list starts from the mark the caller
// above it holds and never inherits the elements of the one that failed.
func TestScratchDropsOnFailure(t *testing.T) {
	var stack scratch[model.Expr]

	outer := stack.mark()
	stack.push(&model.Lit{Value: "kept"})

	inner := stack.mark()
	stack.push(&model.Lit{Value: "abandoned"})
	stack.drop(inner)

	got := stack.take(outer)
	if len(got) != 1 {
		t.Fatalf("the outer list = %#v, want the one element it pushed", got)
	}
	if lit, ok := got[0].(*model.Lit); !ok || lit.Value != "kept" {
		t.Errorf("element 0 = %#v, want the kept literal", got[0])
	}
}

// TestNodeChunkBacksTheParser covers the allocator against a real parse: two
// parses of the same source build separate nodes, so an AST cached by pointer
// cannot be answered with another program's.
func TestNodeChunkBacksTheParser(t *testing.T) {
	const src = `<?php echo 1 + 2;`
	first := mustParse(t, src)
	second := mustParse(t, src)

	if first.Stmts[0] == second.Stmts[0] {
		t.Error("two parses returned the same statement node")
	}
}
