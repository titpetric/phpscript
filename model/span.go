package model

import (
	"context"
	"html/template"
	"time"
)

// Flag configures a span. Unrecognized values select the span type so PHP
// callers can pass plain strings without a separate conversion API.
type Flag string

const (
	OpenSpan  Flag = "open"
	CloseSpan Flag = "close"
)

// SpanType contains the conventional span type names.
var SpanType = struct {
	Database Flag
	Internal Flag
	External Flag
	Template Flag
	Cache    Flag
	HTTP     Flag
}{
	Database: "database",
	Internal: "internal",
	External: "external",
	Template: "template",
	Cache:    "cache",
	HTTP:     "http",
}

// RequestSpan is one timestamped event in a request.
type RequestSpan struct {
	ID       int           `json:"id"`
	Time     time.Time     `json:"time"`
	Duration time.Duration `json:"duration_ns,omitempty"`
	Type     Flag          `json:"type"`
	Filename string        `json:"filename,omitempty"`
	Message  template.HTML `json:"message"`
	Open     bool          `json:"open,omitempty"`
	Close    bool          `json:"close,omitempty"`
}

type requestKey struct{}
type spanFilenameKey struct{}

func WithRequest(ctx context.Context, request *Request) context.Context {
	return context.WithValue(ctx, requestKey{}, request)
}

// WithSpanFilename associates spans created from ctx with the active source
// file.
func WithSpanFilename(ctx context.Context, filename string) context.Context {
	if filename == "" {
		return ctx
	}
	return context.WithValue(ctx, spanFilenameKey{}, filename)
}

// Span appends an event to the request in ctx and returns it so callers can add
// measurements after the observed work completes. The type defaults to
// internal; any other string flag selects a custom type.
func Span(ctx context.Context, message string, flags ...Flag) *RequestSpan {
	request, _ := ctx.Value(requestKey{}).(*Request)
	if request == nil {
		return nil
	}
	span := request.AppendSpan(time.Now(), message, flags...)
	span.Filename, _ = ctx.Value(spanFilenameKey{}).(string)
	return span
}
