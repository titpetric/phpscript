package coverage_test

import (
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner/coverage"
)

// collect returns a collector holding one registration of src under filename,
// with count hits charged to every statement it registered.
func collect(t *testing.T, filename, src string, count int) *coverage.Collector {
	t.Helper()
	program, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := coverage.New()
	c.Register(filename, program)
	for statement := range program.SourceSpans {
		for range count {
			c.Hit(statement)
		}
	}
	return c
}

const collectorSource = `<?php
$a = 1;
echo $a;
function helper() { return 2; }
class Thing { public function method() { return 3; } }
interface Shape {}
`

// TestNew covers what a collector holds after one registration: every
// executable statement seeded, declarations skipped, and the file recorded
// whether or not anything in it ran.
func TestNew(t *testing.T) {
	c := collect(t, "app.php", collectorSource, 0)

	if files := c.Files(); len(files) != 1 || files[0] != "app.php" {
		t.Errorf("files = %v, want app.php", files)
	}
	for _, block := range c.Blocks() {
		if block.Count != 0 {
			t.Errorf("line %d count = %d, want the zero baseline", block.StartLine, block.Count)
		}
	}
	// The declarations are not executable: the class and the interface seed no
	// block of their own, only the method body inside the class does.
	if got := len(c.Blocks()); got != 4 {
		t.Errorf("blocks = %+v, want the four executable statements", c.Blocks())
	}
}

// TestCollector covers the counting: a statement reached twice counts twice,
// and one nothing reached stays at zero, which is the row a report exists to
// show.
func TestCollector(t *testing.T) {
	c := collect(t, "app.php", collectorSource, 2)
	for _, block := range c.Blocks() {
		if block.Count != 2 {
			t.Errorf("line %d count = %d, want 2", block.StartLine, block.Count)
		}
	}

	empty := collect(t, "app.php", collectorSource, 0)
	if got := coverage.Percent(coverage.Columns(empty.Blocks(), func(string) []string { return nil })); got != 0 {
		t.Errorf("uncounted percent = %v, want 0", got)
	}
}

// TestCollector_Functions covers the declaration spans a per-function report is
// charged against: free functions under their name, methods under Class::name.
// An interface declares only signatures and spans nothing.
func TestCollector_Functions(t *testing.T) {
	funcs := collect(t, "app.php", collectorSource, 1).Functions()

	names := map[string]coverage.FuncSpan{}
	for _, fn := range funcs {
		names[fn.Name] = fn
	}
	for _, want := range []string{"helper", "Thing::method"} {
		fn, ok := names[want]
		if !ok {
			t.Errorf("%s is not registered, got %v", want, funcs)
			continue
		}
		if fn.File != "app.php" || fn.StartLine < 1 || fn.EndLine < fn.StartLine {
			t.Errorf("%s span = %+v", want, fn)
		}
	}
	if len(funcs) != 2 {
		t.Errorf("functions = %+v, want the two declarations with a body", funcs)
	}
}

// TestCollector_Register pins that re-registering a cached program keeps the
// counts it already has: a server includes the same file every request, and a
// reset baseline would report the last one instead of the run.
func TestCollector_Register(t *testing.T) {
	program, err := parser.Parse(collectorSource)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := coverage.New()
	c.Register("app.php", program)
	for statement := range program.SourceSpans {
		c.Hit(statement)
	}
	c.Register("app.php", program)

	for _, block := range c.Blocks() {
		if block.Count != 1 {
			t.Errorf("line %d count = %d after a second Register, want 1", block.StartLine, block.Count)
		}
	}
}

// TestName covers the two spellings a collector sees for one file: the
// interpreter anchors an entrypoint at the source root for __FILE__, and names
// an include as it resolved it.
func TestName(t *testing.T) {
	for _, tc := range [][2]string{
		{"/app.php", "app.php"},
		{"app.php", "app.php"},
		{"public/index.php", "public/index.php"},
		{"", ""},
	} {
		if got := coverage.Name(tc[0]); got != tc[1] {
			t.Errorf("Name(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}

	a := coverage.NewAggregator()
	a.Add(collect(t, "/app.php", aggregatorSource, 1))
	a.Add(collect(t, "app.php", aggregatorSource, 1))
	if files := a.Files(); len(files) != 1 {
		t.Errorf("files = %v, want the two spellings folded into one", files)
	}
}
