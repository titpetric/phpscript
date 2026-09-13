package engine

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
)

func compileSource(t *testing.T, source string) *Program {
	t.Helper()
	ast, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	program, err := Compile(ast)
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func countOps(p *Program, op opcode) int {
	n := 0
	for _, inst := range p.code {
		if inst.op == op {
			n++
		}
	}
	return n
}

// The shapes the pass exists for actually fuse: a loop condition is
// slot-const, arithmetic over two variables is slot-slot, and a plain
// assignment of a fused result folds the store.
func TestFuseRewritesTheCommonShapes(t *testing.T) {
	program := compileSource(t, `<?php
$a = 1;
$b = 2;
$c = $a + $b;
for ($i = 0; $i < 10; $i++) {
	$c = $c % 7;
}
echo $c;
`)
	if n := countOps(program, opBinLL); n == 0 {
		t.Error("no opBinLL emitted for $a + $b")
	}
	if n := countOps(program, opBinLC); n == 0 {
		t.Error("no opBinLC emitted for $i < 10 / $c % 7")
	}
	stores := 0
	for _, inst := range program.code {
		if (inst.op == opBinLL || inst.op == opBinLC || inst.op == opBinTC || inst.op == opBinary) && inst.target != 0 {
			stores++
		}
	}
	if stores == 0 {
		t.Error("no store fold emitted for the plain assignments")
	}
}

// A jump target inside a would-be run blocks the fusion so control can still
// land there; a ternary's join-point store is the everyday case.
func TestFuseKeepsJumpTargetsAddressable(t *testing.T) {
	program := compileSource(t, `<?php
$x = 1;
$y = $x > 0 ? "pos" : "neg";
echo $y;
`)
	for _, inst := range program.code {
		switch inst.op {
		case opJump, opJumpFalse, opJumpTrue:
			if inst.target < 0 || inst.target >= len(program.code) {
				t.Fatalf("jump target %d out of range after fusion", inst.target)
			}
		}
	}
}

// fusedRunHost is the minimal host the semantics tests below need.
type fusedRunHost struct {
	Host
	globals map[string]any
	out     strings.Builder
}

func (h *fusedRunHost) SetGlobal(name string, value any) bool {
	if _, ok := h.globals[name]; ok {
		h.globals[name] = value
		return true
	}
	return false
}

func (h *fusedRunHost) Lookup(name string) any { return h.globals[name] }

func (h *fusedRunHost) Echo(value any) error {
	switch v := value.(type) {
	case string:
		h.out.WriteString(v)
	case int64:
		h.out.WriteString(itoa(v))
	}
	return nil
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// An uninitialised operand of a fused binary still resolves through the
// host, the way opLoad resolves it: the global is visible without a frame
// write.
func TestFusedBinaryReadsUninitialisedThroughHost(t *testing.T) {
	program := compileSource(t, `<?php $sum = $seed + $seed; echo $sum;`)
	if countOps(program, opBinLL) == 0 {
		t.Fatal("expected $seed + $seed to fuse")
	}
	host := &fusedRunHost{globals: map[string]any{"seed": int64(21)}}
	if err := Run(program, host); err != nil {
		t.Fatal(err)
	}
	if got := host.out.String(); got != "42" {
		t.Fatalf("output = %q, want 42", got)
	}
}

// A folded store still offers the value to the host first, so a superglobal
// write stays request state exactly as opStore keeps it.
func TestFusedStoreOffersSetGlobal(t *testing.T) {
	program := compileSource(t, `<?php $claimed = $a + $b; echo $claimed;`)
	fused := false
	for _, inst := range program.code {
		if inst.op == opBinLL && inst.target != 0 {
			fused = true
		}
	}
	if !fused {
		t.Fatal("expected a folded store for $claimed")
	}
	host := &fusedRunHost{globals: map[string]any{"claimed": int64(0), "a": int64(2), "b": int64(3)}}
	if err := Run(program, host); err != nil {
		t.Fatal(err)
	}
	if host.globals["claimed"] != int64(5) {
		t.Fatalf("claimed global = %v, want 5", host.globals["claimed"])
	}
	if got := host.out.String(); got != "5" {
		t.Fatalf("output = %q, want 5", got)
	}
}

// A folded store applies the type-reassignment check the way opStore does.
func TestFusedStoreKeepsReassignCheck(t *testing.T) {
	program := compileSource(t, `<?php $s = "text"; $s = $a + $b;`)
	host := &fusedRunHost{globals: map[string]any{"a": int64(1), "b": int64(2)}}
	err := Run(program, host)
	if err == nil || !strings.Contains(err.Error(), "no reassignment: $s") {
		t.Fatalf("err = %v, want the reassignment violation", err)
	}
}
