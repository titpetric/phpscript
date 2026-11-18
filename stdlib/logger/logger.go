package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/titpetric/phpscript/telemetry"
)

// Logger is a slog logger that fails the span it runs in when a caller reports
// an error. Log output belongs to the log and stays in it. The trace receives
// a single fact: that the work went wrong.
//
// It holds the context the work runs in. A caller passes a logger to a
// library for one call's length. It lives as long as that context.
type Logger struct {
	ctx  context.Context
	log  *slog.Logger
	name string
}

// New returns a logger writing to the default slog logger, failing the span in
// ctx on Error. The name prefixes every message and an empty name adds none.
func New(ctx context.Context, name string) *Logger {
	return &Logger{
		ctx:  ctx,
		name: name,
	}
}

// WithLogger returns a copy of the logger writing to log and not to the
// default slog logger. A nil log restores the default.
func (l *Logger) WithLogger(log *slog.Logger) *Logger {
	if l == nil {
		return nil
	}

	out := *l
	out.log = log
	return &out
}

// Info logs a message at `info` level.
func (l *Logger) Info(msg string, args ...any) {
	if l == nil {
		return
	}

	l.slog().InfoContext(l.ctx, l.label(msg), args...)
}

// Warn logs a message at warning level and records nothing on the span, for a
// degradation the work survives.
func (l *Logger) Warn(msg string, args ...any) {
	if l == nil {
		return
	}

	l.slog().WarnContext(l.ctx, l.label(msg), args...)
}

// Error logs a message at error level and records it on the logger's span,
// failing that span and the trace as well. The span is not ended.
func (l *Logger) Error(msg string, args ...any) {
	if l == nil {
		return
	}

	l.slog().ErrorContext(l.ctx, l.label(msg), args...)
	telemetry.SpanFromContext(l.ctx).RecordError(cause(msg, args))
}

// slog returns the logger, falling back to the default one on every call.
func (l *Logger) slog() *slog.Logger {
	if l.log != nil {
		return l.log
	}
	return slog.Default()
}

// label returns the string the logger writes for msg.
func (l *Logger) label(msg string) string {
	if l.name == "" {
		return msg
	}
	if msg == "" {
		return l.name
	}
	return l.name + " " + msg
}

// cause wraps the first error among args under msg, or msg alone when args hold none.
func cause(msg string, args []any) error {
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			return fmt.Errorf("%s: %w", msg, err)
		}
	}
	return errors.New(msg)
}
