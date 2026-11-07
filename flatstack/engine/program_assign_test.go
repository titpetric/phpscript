package engine

import (
	"testing"
)

// callOrder lists the function names the program calls, in emission order.
func callOrder(p *Program) []string {
	var names []string
	for _, inst := range p.code {
		if inst.op == opCall {
			names = append(names, inst.name)
		}
	}
	return names
}

// TestAssignmentResolvesTheTargetFirst covers the operand order a store
// emits: the container, the key and the receiver run before the value, php's
// order, and opSetIndex and opSetProperty read it off a.
func TestAssignmentResolvesTheTargetFirst(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		source string
		want   []string
		op     opcode
	}{
		{source: `<?php $a[k()] = v();`, want: []string{"k", "v"}, op: opSetIndex},
		{source: `<?php $a[k()] .= v();`, want: []string{"k", "v"}, op: opSetIndex},
		{source: `<?php r()->p = v();`, want: []string{"r", "v"}, op: opSetProperty},
	} {
		program := compileSource(t, test.source)
		got := callOrder(program)
		if len(got) != len(test.want) || got[0] != test.want[0] || got[1] != test.want[1] {
			t.Errorf("%s calls %v, want %v", test.source, got, test.want)
		}
		for _, inst := range program.code {
			if inst.op == test.op && inst.a != 1 {
				t.Errorf("%s stores with a = %d, want 1 for the target-first order", test.source, inst.a)
			}
		}
	}
}

// TestCompoundAssignExprReadsTheTargetBack covers the value a compound
// operator answers in expression position: the host reports no result, so the
// target is read again through the slots that resolved it.
func TestCompoundAssignExprReadsTheTargetBack(t *testing.T) {
	t.Parallel()
	program := compileSource(t, `<?php $x = $a[k()] += 1;`)
	if n := countOps(program, opCall); n != 1 {
		t.Errorf("compiled %d calls, want 1", n)
	}
	if n := countOps(program, opSetIndex); n != 1 {
		t.Errorf("compiled %d stores, want 1", n)
	}
	if n := countOps(program, opIndex); n != 1 {
		t.Errorf("compiled %d reads, want the one that answers the value", n)
	}
}
