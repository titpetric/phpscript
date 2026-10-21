package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/titpetric/phpscript/telemetry"
)

// Logger is a slog logger that fails the span it runs in when a caller reports
// an error. Log output is the log's business and stays there; what reaches the
// trace is what a trace has a place for, that the work went wrong.
//
// The context it holds is the one the work runs under: a caller passes a logger
// to a library for the length of one call, so it lives as long as that context.
type Logger struct {
	ctx  context.Context
	log  *slog.Logger
	name string
}

// New returns a logger writing to the default slog logger, failing the span in
// ctx on Error.
//
// The name prefixes the message, which separates two libraries logging the
// same word. An empty name logs the message by itself.
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

// Error logs a message at error level and records it on the logger's own span,
// which fails that span and the trace with it. The error is the first one among
// the values where the caller passes one, so the span keeps the failure itself
// and never only its message. The span is not ended: it belongs to
// whoever opened it, and the work it measures is still theirs to finish.
func (l *Logger) Error(msg string, args ...any) {
	if l == nil {
		return
	}

	l.slog().ErrorContext(l.ctx, l.label(msg), args...)
	telemetry.SpanFromContext(l.ctx).RecordError(cause(msg, args))
}

// slog returns the logger to write the message to. A logger holding none writes
// to the default logger, and reads it per call and not at construction, so a
// process that configures logging after the fact still gets the output.
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

// cause returns the error to fail a span with: the first error among the
// values, kept under the message beside it, and the message alone where the
// caller passes none.
func cause(msg string, args []any) error {
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			return fmt.Errorf("%s: %w", msg, err)
		}
	}
	return errors.New(msg)
}
