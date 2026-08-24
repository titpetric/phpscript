package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/titpetric/phpscript/telemetry"
)

// Logger is a slog logger that fails the span it runs in when it is told about
// an error. Log output is the log's business and stays there; what reaches the
// trace is the one thing a trace has a place for, which is that the work went
// wrong.
//
// The context it holds is the one the work runs under: a logger is handed to a
// library for the length of one call, so it lives as long as the context it was
// built with.
type Logger struct {
	ctx  context.Context
	log  *slog.Logger
	name string
}

// New returns a logger writing to the default slog logger, failing the span in
// ctx on Error.
//
// The name prefixes the message, which is what tells two libraries logging the
// same word apart. An empty name logs the message alone.
func New(ctx context.Context, name string) *Logger {
	return &Logger{
		ctx:  ctx,
		name: name,
	}
}

// WithLogger returns a copy of the logger writing to log rather than to the
// default slog logger. A nil log restores the default.
func (l *Logger) WithLogger(log *slog.Logger) *Logger {
	if l == nil {
		return nil
	}

	out := *l
	out.log = log
	return &out
}

// Info logs a message at info level.
func (l *Logger) Info(msg string, args ...any) {
	if l == nil {
		return
	}

	l.slog().InfoContext(l.ctx, l.label(msg), args...)
}

// Error logs a message at error level and records it on the span the logger was
// built in, which fails that span and the trace with it. The error is the first
// one among the values when the caller passed one, so the span keeps what went
// wrong and not only what it was called. The span is not ended: it belongs to
// whoever opened it, and the work it measures is still theirs to finish.
func (l *Logger) Error(msg string, args ...any) {
	if l == nil {
		return
	}

	l.slog().ErrorContext(l.ctx, l.label(msg), args...)
	telemetry.SpanFromContext(l.ctx).RecordError(cause(msg, args))
}

// slog is where the message is written. A logger that was given none writes to
// the default logger, and reads it per call rather than at construction, so a
// process that configures logging after the fact still gets the output.
func (l *Logger) slog() *slog.Logger {
	if l.log != nil {
		return l.log
	}
	return slog.Default()
}

// label is what the message is logged as.
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
// values, kept under the message it was reported with, and the message alone
// when the caller passed none.
func cause(msg string, args []any) error {
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			return fmt.Errorf("%s: %w", msg, err)
		}
	}
	return errors.New(msg)
}
