package logger

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/go-bridget/mig/migrate"

	"github.com/titpetric/phpscript/telemetry"
)

// The logger is handed to libraries that ask for one, so what they ask for has
// to be what it is. mig is the caller it was written for.
var _ migrate.Logger = (*Logger)(nil)

// discard is a logger writing its lines nowhere, which is what a test that is
// looking at the trace wants of the log.
func discard(l *Logger) *Logger {
	return l.WithLogger(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
}

// TestLoggerFailsSpanOnError covers what the logger has to do with the trace and
// the extent of it: an error fails the span the work runs in, and nothing else
// that was logged is recorded anywhere near it.
func TestLoggerFailsSpanOnError(t *testing.T) {
	tracer, err := telemetry.New(telemetry.NewOptions())
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}

	ctx, span := telemetry.Start(ctx, "migrate", telemetry.KindDatabase)
	log := discard(New(ctx, "migrate"))
	log.Info("migration", "file", "schema.up.sql", "status", "OK")
	log.Error("failed", "file", "broken.up.sql", "error", errors.New("syntax error"))
	span.End()

	// The trace holds the span the work opened and nothing besides: log output is
	// not a span, and the values a message carried are not attributes.
	spans := trace.Clone().Spans[1:]
	if len(spans) != 1 {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].Name != "migrate" || len(spans[0].Attributes) != 0 {
		t.Fatalf("span = %+v", spans[0])
	}

	// A migration that did not apply is red on the front end rather than a line
	// to read, and the error it failed with is the one mig reported.
	if spans[0].Err() == nil || !strings.Contains(spans[0].Error, "failed: syntax error") {
		t.Fatalf("span error = %q", spans[0].Error)
	}
}

// TestLoggerErrorWithoutErrorValue covers a caller that reported what happened
// without an error to go with it: the message is what the span fails with.
func TestLoggerErrorWithoutErrorValue(t *testing.T) {
	tracer, err := telemetry.New(telemetry.NewOptions())
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}

	ctx, span := telemetry.Start(ctx, "migrate", telemetry.KindDatabase)
	discard(New(ctx, "migrate")).Error("failed", "file", "broken.up.sql")
	span.End()

	spans := trace.Clone().Spans[1:]
	if len(spans) != 1 || spans[0].Error != "failed" {
		t.Fatalf("spans = %+v", spans)
	}
}

// TestLoggerWritesToSlog covers the half of the logger that is a logger: the
// message and its values reach slog whether or not a trace is running.
func TestLoggerWritesToSlog(t *testing.T) {
	var logged strings.Builder
	log := New(context.Background(), "migrate").
		WithLogger(slog.New(slog.NewTextHandler(&logged, nil)))
	log.Info("migration", "file", "schema.up.sql")
	log.Error("failed", "error", errors.New("syntax error"))

	lines := strings.Split(strings.TrimRight(logged.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("logged = %q", logged.String())
	}
	if !strings.Contains(lines[0], `level=INFO msg="migrate migration" file=schema.up.sql`) {
		t.Errorf("info line = %q", lines[0])
	}
	if !strings.Contains(lines[1], `level=ERROR msg="migrate failed" error="syntax error"`) {
		t.Errorf("error line = %q", lines[1])
	}
}

// TestLoggerDefaultsToSlogDefault covers a logger nobody configured, which
// writes where the process writes the rest of its log.
func TestLoggerDefaultsToSlogDefault(t *testing.T) {
	var logged strings.Builder
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	defer slog.SetDefault(previous)

	New(context.Background(), "migrate").Info("migration", "file", "schema.up.sql")

	if !strings.Contains(logged.String(), `msg="migrate migration"`) {
		t.Errorf("logged = %q", logged.String())
	}
}

// TestLoggerWithoutTrace covers a CLI run: no trace in the context, and no
// logger at all, which is what a library that was handed nothing calls.
func TestLoggerWithoutTrace(t *testing.T) {
	log := discard(New(context.Background(), "migrate"))
	log.Info("migration", "file", "schema.up.sql")
	log.Error("failed", "error", errors.New("syntax error"))

	var absent *Logger
	absent.Info("migration", "file", "schema.up.sql")
	absent.Error("failed", "error", errors.New("syntax error"))
	if absent.WithLogger(slog.Default()) != nil {
		t.Error("WithLogger on a nil logger returned a logger")
	}
}

func TestLoggerNames(t *testing.T) {
	for _, test := range []struct {
		name string
		msg  string
		want string
	}{
		{name: "migrate", msg: "migration", want: "migrate migration"},
		{name: "", msg: "migration", want: "migration"},
		{name: "migrate", msg: "", want: "migrate"},
	} {
		if got := New(context.Background(), test.name).label(test.msg); got != test.want {
			t.Errorf("label(%q, %q) = %q, want %q", test.name, test.msg, got, test.want)
		}
	}
}
