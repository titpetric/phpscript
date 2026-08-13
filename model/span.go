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
	ID      int           `json:"id"`
	Time    time.Time     `json:"time"`
	Type    Flag          `json:"type"`
	Message template.HTML `json:"message"`
	Open    bool          `json:"open,omitempty"`
	Close   bool          `json:"close,omitempty"`
}

type requestKey struct{}

func WithRequest(ctx context.Context, request *Request) context.Context {
	return context.WithValue(ctx, requestKey{}, request)
}

// Span appends an event to the request in ctx. The type defaults to internal;
// any other string flag selects a custom type.
func Span(ctx context.Context, message string, flags ...Flag) {
	request, _ := ctx.Value(requestKey{}).(*Request)
	if request == nil {
		return
	}
	request.AppendSpan(time.Now(), message, flags...)
}
