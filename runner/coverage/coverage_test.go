package coverage_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner/coverage"
)

// collect returns a collector holding one registration of src under filename,
// with count hits charged to each registered statement.
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

// TestNew covers the state of a collector after one registration: every
// executable statement seeded, declarations skipped, and the file recorded
// even when nothing in it ran.
func TestNew(t *testing.T) {
	t.Parallel()
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
	// block at all, only the method body inside the class does.
	if got := len(c.Blocks()); got != 4 {
		t.Errorf("blocks = %+v, want the four executable statements", c.Blocks())
	}
}

// TestCollector covers the counting: a statement reached twice counts twice;
// one that nothing reached stays at zero, the row a report exists to show.
func TestCollector(t *testing.T) {
	t.Parallel()
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

// TestCollector_Functions covers the declaration spans a per-function report
// charges against: free functions by their name, methods by Class::name.
// An interface declares only signatures and spans nothing.
func TestCollector_Functions(t *testing.T) {
	t.Parallel()
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
// counts it already has: a server includes the same file every request. A
// reset baseline would report the previous run in place of the current one.
func TestCollector_Register(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// TestCollectorMode covers the two recording modes: count adds every pass, and
// atomic marks the first and leaves every later pass a load. The profile names
// the mode that recorded it.
func TestCollectorMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mode coverage.Mode
		want int
	}{
		{coverage.ModeCount, 3},
		{coverage.ModeAtomic, 1},
	} {
		program, err := parser.Parse("<?php\necho 1;\n")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		c := coverage.NewMode(tc.mode)
		c.Register("app.php", program)
		for statement := range program.SourceSpans {
			for range 3 {
				c.Hit(statement)
			}
		}
		var out strings.Builder
		if err := coverage.WriteProfile(&out, coverage.Columns(c.Blocks(), func(string) []string { return nil }), tc.mode); err != nil {
			t.Fatalf("WriteProfile: %v", err)
		}
		if !strings.HasPrefix(out.String(), "mode: "+string(tc.mode)+"\n") {
			t.Errorf("profile header = %q, want mode %s", out.String(), tc.mode)
		}
		for _, block := range c.Blocks() {
			if block.Count != tc.want {
				t.Errorf("mode %s: line %d count = %d, want %d", tc.mode, block.StartLine, block.Count, tc.want)
			}
		}
	}
}

// benchCollector returns a collector over a three-statement program and the
// first registered statement, so a benchmark charges one lookup per op.
func benchCollector(b *testing.B, mode coverage.Mode) (*coverage.Collector, model.Stmt) {
	b.Helper()
	program, err := parser.Parse("<?php\n$a = 1;\necho $a;\n")
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	c := coverage.NewMode(mode)
	c.Register("app.php", program)
	var stmt model.Stmt
	for s := range program.SourceSpans {
		stmt = s
		break
	}
	return c, stmt
}

// BenchmarkCollectorHitCount prices the atomic increment count mode charges.
func BenchmarkCollectorHitCount(b *testing.B) {
	c, stmt := benchCollector(b, coverage.ModeCount)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Hit(stmt)
	}
}

// BenchmarkCollectorHitAtomic prices the compare-and-swap atomic mode runs on
// its first pass and the load it leaves every later one.
func BenchmarkCollectorHitAtomic(b *testing.B) {
	c, stmt := benchCollector(b, coverage.ModeAtomic)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Hit(stmt)
	}
}
