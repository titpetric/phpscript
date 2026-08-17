package ps

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/titpetric/pdo/client"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/telemetry"
)

func TestDatabaseObserverRecordsQuerySpan(t *testing.T) {
	tracer, err := telemetry.New(telemetry.NewOptions())
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}
	ctx = telemetry.WithSpanLine(telemetry.WithSpanFilename(ctx, "queries/users.php"), 23)

	database := &Database{ID: "db"}
	ctx, end := database.withSpan(ctx, "Get")
	database.observe(ctx, client.QueryLogEntry{
		Query:   "select * from users where id = ?",
		Args:    []any{int64(42)},
		TxDepth: 1,
	})
	recordRows(ctx, 1)
	end()

	spans := trace.Clone().Spans
	if len(spans) != 2 {
		t.Fatalf("spans = %+v", spans)
	}
	span := spans[1]
	if span.Name != "db.Get" || span.Kind != telemetry.KindDatabase || span.Filename != "queries/users.php" || span.Line != 23 {
		t.Fatalf("database span = %+v", span)
	}
	if !span.Ended() || span.Duration <= 0 {
		t.Fatalf("database span was not measured: %+v", span)
	}
	if span.Attributes["query"] != "select * from users where id = ?" || span.Attributes["transaction_depth"] != 1 || span.Attributes["rows"] != 1 {
		t.Fatalf("database span attributes = %+v", span.Attributes)
	}

	// The values bound to the placeholders are what says which row was read.
	args, ok := span.Attributes["args"].([]any)
	if !ok || len(args) != 1 || args[0] != int64(42) {
		t.Fatalf("bound arguments = %#v", span.Attributes["args"])
	}
}

func TestDatabaseObserverRecordsNamedArguments(t *testing.T) {
	span, observe := newObservedSpan(t)
	observe(client.QueryLogEntry{
		Query: "select * from users where id = :id",
		Args:  map[string]any{"id": int64(42)},
	})

	named, ok := span.Attributes["args"].(map[string]any)
	if !ok || named["id"] != int64(42) {
		t.Fatalf("named arguments = %#v", span.Attributes["args"])
	}
}

func TestDatabaseObserverOmitsEmptyArguments(t *testing.T) {
	for _, args := range []any{nil, []any{}, map[string]any{}} {
		span, observe := newObservedSpan(t)
		observe(client.QueryLogEntry{Query: "select 1", Args: args})

		if value, ok := span.Attributes["args"]; ok {
			t.Fatalf("args = %#v for %#v, want no attribute", value, args)
		}
	}
}

// A query that found no rows is control flow, not a failure: recording it would
// fail the span, the trace, and the SLA computed from them.
func TestDatabaseObserverIgnoresSentinelErrors(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		wantError bool
	}{
		{name: "no rows", err: sql.ErrNoRows},
		{name: "canceled", err: context.Canceled},
		{name: "syntax", err: errors.New("near \"slect\": syntax error"), wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			span, observe := newObservedSpan(t)
			observe(client.QueryLogEntry{Query: "select 1", Err: test.err})

			if got := span.Failed(); got != test.wantError {
				t.Fatalf("failed = %t, error = %q", got, span.Error)
			}
		})
	}
}

// newObservedSpan returns a recorded database span and the observer callback
// reporting a query onto it.
func newObservedSpan(t *testing.T) (*telemetry.Span, func(client.QueryLogEntry)) {
	t.Helper()

	tracer, err := telemetry.New(telemetry.NewOptions())
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}

	database := &Database{}
	ctx, end := database.withSpan(ctx, "Get")
	t.Cleanup(end)

	span := trace.Root()
	for _, recorded := range trace.Spans {
		span = recorded
	}
	return span, func(entry client.QueryLogEntry) {
		database.observe(ctx, entry)
	}
}

// TestDatabaseRowsAreReadableAsPHPArrays pins the row representation Get and
// GetAll hand to the VM: the bridge's own []map[string]any, uncopied. The VM's
// value-model helpers must see it as an array-like value with the columns
// reachable by key, which is what foreach and $row["col"] rely on.
func TestDatabaseRowsAreReadableAsPHPArrays(t *testing.T) {
	rows := any([]map[string]any{{
		"id":   int64(1),
		"name": "catalogue",
	}})

	if !model.IsCollection(rows) {
		t.Fatalf("rows are not array-like: %#v", rows)
	}
	if n, _ := model.LenValues(rows); n != 1 {
		t.Fatalf("rows length = %d", n)
	}

	var row any
	model.RangeValues(rows, func(key, value any) bool {
		if key != int64(0) {
			t.Fatalf("row key = %#v", key)
		}
		row = value
		return false
	})

	columns := map[string]any{}
	model.RangeValues(row, func(key, value any) bool {
		columns[key.(string)] = value
		return true
	})
	if columns["name"] != "catalogue" || columns["id"] != int64(1) {
		t.Fatalf("columns = %#v", columns)
	}

	// A row is a Go map, so a script adding a column mutates the row the result
	// set holds — the same reference semantics the *model.Array copy had.
	row.(map[string]any)["columns"] = int64(6)
	if got := rows.([]map[string]any)[0]["columns"]; got != int64(6) {
		t.Fatalf("added column = %#v", got)
	}
}
