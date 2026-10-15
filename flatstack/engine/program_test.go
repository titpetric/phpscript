package engine

import (
	"testing"

	"github.com/titpetric/phpscript/parser"
)

// semanticHost records what the engine delegated. Every method the test
// programs reach is implemented and the rest is left to the embedded nil
// interface, so a program reaching an unexpected one fails loudly and never
// silently.
type semanticHost struct {
	Host
	binary  []string
	entries []Entry
	echoed  []any
}

func (h *semanticHost) SetGlobal(string, any) bool { return false }

// Truthy answers a bool as itself, so a comparison the VM computed drives the
// jump, and reads "yes" as the one true string.
func (h *semanticHost) Truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "yes"
	}
	return false
}

// Binary answers a tag naming itself, so a result the engine computed on its
// own is told apart from one the host returned.
func (h *semanticHost) Binary(op string, left, right any) (any, error) {
	h.binary = append(h.binary, op)
	return "host(" + op + ")", nil
}

func (h *semanticHost) Entries(any) []Entry { return h.entries }

func (h *semanticHost) Echo(value any) error {
	h.echoed = append(h.echoed, value)
	return nil
}

// runHost compiles source and runs it against host.
func runHost(t *testing.T, host Host, source string) {
	t.Helper()

	ast, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	program, err := Compile(ast)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(host, program); err != nil {
		t.Fatal(err)
	}
}

// TestEntry covers the pair foreach walks. The engine owns iteration state
// and no value semantics, so what a collection holds is the host's answer:
// Entries renders it to a key and a value per step, and the loop binds them
// in the order the slice holds.
func TestEntry(t *testing.T) {
	t.Parallel()
	host := &semanticHost{entries: []Entry{
		{Key: "a", Value: int64(1)},
		{Key: "b", Value: int64(2)},
	}}

	runHost(t, host, `<?php $row = 0; foreach ($row as $key => $value) { echo $key; echo $value; }`)

	want := []any{"a", int64(1), "b", int64(2)}
	if len(host.echoed) != len(want) {
		t.Fatalf("echoed = %v, want %v", host.echoed, want)
	}
	for i, value := range want {
		if host.echoed[i] != value {
			t.Errorf("echoed[%d] = %v, want %v", i, host.echoed[i], value)
		}
	}

	// A key the source did not name is still a step: a list has an index per
	// entry, so the value alone is bound and the engine reads the same
	// pairs.
	host.echoed = nil
	runHost(t, host, `<?php $row = 0; foreach ($row as $value) { echo $value; }`)
	if len(host.echoed) != 2 || host.echoed[0] != int64(1) || host.echoed[1] != int64(2) {
		t.Errorf("echoed = %v, want the two values", host.echoed)
	}
}

// TestHost covers the split the interface draws. The engine owns
// compilation, operand storage, jumps and iteration state, and PHP value
// semantics are the host's, so an operation on operand types the engine has
// no path for arrives at Binary and the result it returns is what the script
// sees. Both backends share one host, so a script reads the same semantics
// either way.
func TestHost(t *testing.T) {
	t.Parallel()
	host := &semanticHost{}

	runHost(t, host, `<?php $n = 1.5; echo $n + 2;`)

	if len(host.binary) != 1 || host.binary[0] != "+" {
		t.Fatalf("the engine delegated %v to Binary, want one +", host.binary)
	}
	if len(host.echoed) != 1 || host.echoed[0] != "host(+)" {
		t.Fatalf("echoed = %v, want the value the host returned", host.echoed)
	}

	// Two int operands are answered inside the VM and never reach the host:
	// the fast path covers the arithmetic a hot loop runs, and the host is
	// left the shapes it cannot.
	host.binary, host.echoed = nil, nil
	runHost(t, host, `<?php $n = 1; echo $n + 2;`)
	if len(host.binary) != 0 {
		t.Errorf("the engine delegated %v for two ints, want nothing", host.binary)
	}
	if len(host.echoed) != 1 || host.echoed[0] != int64(3) {
		t.Errorf("echoed = %v, want the VM's own 3", host.echoed)
	}

	// Truthiness is the host's: the engine jumps on what Truthy answered and
	// never on the value itself.
	host.echoed = nil
	runHost(t, host, `<?php $t = "yes"; $f = "no"; if ($t) { echo "taken"; } if ($f) { echo "untaken"; }`)
	if len(host.echoed) != 1 || host.echoed[0] != "taken" {
		t.Errorf("echoed = %v, want only the branch Truthy allowed", host.echoed)
	}
}
