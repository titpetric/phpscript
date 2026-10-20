package telemetry

import (
	"context"
)

// PHP has no lexical scope a Go caller can read, so the interpreter publishes
// the file and line of the statement it runs into the binding's context.
// Spans started from such a context record that location, so
// a span in the front end point back at the line of PHP that caused it.

type spanFilenameKey struct{}

type spanLineKey struct{}

// WithSpanFilename associates spans started from ctx with a source file.
func WithSpanFilename(ctx context.Context, filename string) context.Context {
	if filename == "" {
		return ctx
	}
	return context.WithValue(ctx, spanFilenameKey{}, filename)
}

// WithSpanLine associates spans started from ctx with a source line.
func WithSpanLine(ctx context.Context, line int) context.Context {
	if line <= 0 {
		return ctx
	}
	return context.WithValue(ctx, spanLineKey{}, line)
}

// SourceValue answers the two span-source keys from location fields a foreign
// context value type carries, so a host folds the file and the line into one
// allocation beside its own keys. A false sends the caller down its parent
// chain, matching WithSpanFilename and WithSpanLine declining an empty value.
func SourceValue(key any, filename string, line int) (any, bool) {
	switch key.(type) {
	case spanFilenameKey:
		if filename != "" {
			return filename, true
		}
	case spanLineKey:
		if line > 0 {
			return line, true
		}
	}
	return nil, false
}

// SpanSource returns the source location carried by ctx. Both results are zero
// when no PHP frame published one.
func SpanSource(ctx context.Context) (string, int) {
	filename, _ := ctx.Value(spanFilenameKey{}).(string)
	line, _ := ctx.Value(spanLineKey{}).(int)
	return filename, line
}

// withSource records the source location ctx carries on a span, and answers a
// nil span unchanged, so the caller does not have to check.
func withSource(ctx context.Context, span *Span) *Span {
	if span == nil {
		return nil
	}
	if filename, line := SpanSource(ctx); filename != "" || line > 0 {
		span.SetSource(filename, line)
	}
	return span
}
