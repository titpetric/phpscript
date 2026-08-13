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

// Span describes a mutable timed operation.
type Span interface {
	End()
	SetMessage(string)
	SetFilename(string)
	SetLine(int)
	SetTime(time.Time)
	SetDuration(time.Duration)
	SetType(Flag)
	SetAttribute(string, any)
	RecordError(error)
}

// Tracer starts named spans.
type Tracer interface {
	StartSpan(context.Context, string) Span
}

// RequestSpan is one timestamped event in a request.
type RequestSpan struct {
	ID         int            `json:"id"`
	Time       time.Time      `json:"time"`
	Duration   time.Duration  `json:"duration_ns,omitempty"`
	Type       Flag           `json:"type"`
	Filename   string         `json:"filename,omitempty"`
	Line       int            `json:"line,omitempty"`
	Message    template.HTML  `json:"message"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Error      string         `json:"error,omitempty"`
	Open       bool           `json:"open,omitempty"`
	Close      bool           `json:"close,omitempty"`
}

var _ Span = (*RequestSpan)(nil)

// End records the span duration once.
func (s *RequestSpan) End() {
	if s == nil || s.Duration > 0 {
		return
	}
	s.Duration = time.Since(s.Time)
}

// SetMessage replaces the span message.
func (s *RequestSpan) SetMessage(message string) {
	if s != nil {
		s.Message = template.HTML(message)
	}
}

// SetFilename records the source filename associated with the span.
func (s *RequestSpan) SetFilename(filename string) {
	if s != nil {
		s.Filename = filename
	}
}

// SetLine records the source line associated with the span.
func (s *RequestSpan) SetLine(line int) {
	if s != nil {
		s.Line = line
	}
}

// SetTime replaces the span start time.
func (s *RequestSpan) SetTime(started time.Time) {
	if s != nil {
		s.Time = started
	}
}

// SetDuration replaces the measured span duration.
func (s *RequestSpan) SetDuration(duration time.Duration) {
	if s != nil {
		s.Duration = duration
	}
}

// SetType replaces the span type.
func (s *RequestSpan) SetType(spanType Flag) {
	if s != nil {
		s.Type = spanType
	}
}

// SetAttribute records an attribute on the span.
func (s *RequestSpan) SetAttribute(key string, value any) {
	if s == nil {
		return
	}
	if s.Attributes == nil {
		s.Attributes = make(map[string]any)
	}
	s.Attributes[key] = value
}

// RecordError records an error on the span.
func (s *RequestSpan) RecordError(err error) {
	if s != nil && err != nil {
		s.Error = err.Error()
	}
}

type requestKey struct{}
type spanFilenameKey struct{}
type spanLineKey struct{}

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

// WithSpanLine associates spans created from ctx with the active source line.
func WithSpanLine(ctx context.Context, line int) context.Context {
	if line <= 0 {
		return ctx
	}
	return context.WithValue(ctx, spanLineKey{}, line)
}

// StartSpan appends an event to the request in ctx and returns it so callers can
// add measurements after the observed work completes. The type defaults to
// internal; any other string flag selects a custom type.
func StartSpan(ctx context.Context, message string, flags ...Flag) *RequestSpan {
	request, _ := ctx.Value(requestKey{}).(*Request)
	if request == nil {
		return nil
	}
	span := request.AppendSpan(time.Now(), message, flags...)
	filename, _ := ctx.Value(spanFilenameKey{}).(string)
	span.SetFilename(filename)
	line, _ := ctx.Value(spanLineKey{}).(int)
	span.SetLine(line)
	return span
}
