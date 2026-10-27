package telemetry

import (
	"context"
	"testing"
)

// TestSourceValue covers what a foreign context value answers for the two
// span-source keys: the values the WithSpan wrappers attach, and a false for
// an empty location and the caller falls down its parent chain the way an
// unwrapped context does.
func TestSourceValue(t *testing.T) {
	t.Parallel()
	carrier := WithSpanLine(WithSpanFilename(context.Background(), "x"), 1)
	ctx := carrierThrough{Context: context.Background(), filename: "public/index.php", line: 23}
	if filename, line := SpanSource(ctx); filename != "public/index.php" || line != 23 {
		t.Errorf("SpanSource() = %q, %d, want the carried fields", filename, line)
	}
	empty := carrierThrough{Context: carrier}
	if filename, line := SpanSource(empty); filename != "x" || line != 1 {
		t.Errorf("SpanSource() = %q, %d, want the parent's values when the fields are empty", filename, line)
	}
}

// carrierThrough is the shape runner.bindingCtx has: the location as fields,
// answered through SourceValue, with the parent for everything else.
type carrierThrough struct {
	context.Context
	filename string
	line     int
}

func (c carrierThrough) Value(key any) any {
	if v, ok := SourceValue(key, c.filename, c.line); ok {
		return v
	}
	return c.Context.Value(key)
}

// TestSpanSource covers the source location a span records. PHP has no lexical
// scope a Go caller can read and the interpreter publishes the file and line of
// the statement it runs into the context a binding runs under. The write drops
// an empty filename and a line at or below zero and a span never records a
// location of "" or 0 as though a PHP frame had published it.
func TestSpanSource(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		ctx      context.Context
		filename string
		line     int
	}{
		{
			name: "no PHP frame published one",
			ctx:  context.Background(),
		},
		{
			name:     "file and line",
			ctx:      WithSpanLine(WithSpanFilename(context.Background(), "public/index.php"), 23),
			filename: "public/index.php",
			line:     23,
		},
		{
			name: "an empty filename is dropped",
			ctx:  WithSpanFilename(context.Background(), ""),
		},
		{
			name: "a zero line is dropped",
			ctx:  WithSpanLine(context.Background(), 0),
		},
		{
			name: "a negative line is dropped",
			ctx:  WithSpanLine(context.Background(), -1),
		},
		{
			name:     "the innermost frame answers",
			ctx:      WithSpanLine(WithSpanLine(WithSpanFilename(context.Background(), "queries/users.php"), 1), 9),
			filename: "queries/users.php",
			line:     9,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			filename, line := SpanSource(test.ctx)
			if filename != test.filename || line != test.line {
				t.Errorf("SpanSource() = %q, %d, want %q, %d", filename, line, test.filename, test.line)
			}
		})
	}
}

// TestWithSpanFilename covers the location reaching a span. A binding
// instruments without naming a file and every span started from that context
// carries the file the interpreter published and the line beside it.
func TestWithSpanFilename(t *testing.T) {
	t.Parallel()
	tracer := newRecordingTracer(t)
	ctx, trace, err := tracer.StartTrace(context.Background(), "GET /users.php")
	if err != nil {
		t.Fatal(err)
	}

	StartSpan(WithSpanLine(WithSpanFilename(ctx, "queries/users.php"), 23), "SELECT users", KindDatabase).End()

	spans := trace.Clone().Spans
	span := spans[len(spans)-1]
	if span.Filename != "queries/users.php" || span.Line != 23 {
		t.Errorf("span source = %q:%d, want queries/users.php:23", span.Filename, span.Line)
	}

	// A span started without a location carries none and a Go-only call does
	// not inherit the line of whatever ran before it.
	StartSpan(ctx, "flush", KindInternal).End()
	spans = trace.Clone().Spans
	if last := spans[len(spans)-1]; last.Filename != "" || last.Line != 0 {
		t.Errorf("span source = %q:%d, want nothing recorded", last.Filename, last.Line)
	}
}
