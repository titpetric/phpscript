package ps

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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

// A statement is classified by the keyword it starts with, and the tag it
// carries is recorded beside it: a trace groups by what a query was for, which
// the statement text alone does not answer.
func TestDatabaseObserverRecordsQueryTypeAndComment(t *testing.T) {
	span, observe := newObservedSpan(t)
	observe(client.QueryLogEntry{Query: "/* userGet */ SELECT * FROM user WHERE id = ?"})

	if span.Attributes["query_type"] != "select" || span.Attributes["query_comment"] != "userGet" {
		t.Fatalf("attributes = %+v", span.Attributes)
	}
	// The comment stays in the statement, so `show processlist` shows it too.
	if span.Attributes["query"] != "/* userGet */ SELECT * FROM user WHERE id = ?" {
		t.Fatalf("query = %#v", span.Attributes["query"])
	}

	// An untagged statement has no attribute rather than an empty one.
	span, observe = newObservedSpan(t)
	observe(client.QueryLogEntry{Query: "select 1"})
	if _, ok := span.Attributes["query_comment"]; ok || span.Attributes["query_type"] != "select" {
		t.Fatalf("attributes = %+v", span.Attributes)
	}
}

// The CRUD helpers write by definition, so a read-only client refuses them
// before a statement is built. The nil Bridge is the assertion that nothing
// reached the database: running one would panic.
func TestDatabaseReadonlyRefusesWrites(t *testing.T) {
	database := &Database{IsReadonly: true}
	ctx := context.Background()

	for _, test := range []struct {
		name string
		call func() (any, error)
		want string
	}{
		{name: "insert", want: "insert", call: func() (any, error) {
			return database.Insert(ctx, "users", map[string]any{"name": "Ada"})
		}},
		{name: "replace", want: "replace", call: func() (any, error) {
			return database.Replace(ctx, "users", map[string]any{"name": "Ada"})
		}},
		{name: "update", want: "update", call: func() (any, error) {
			return database.Update(ctx, "users", map[string]any{"name": "Ada"}, "id")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := test.call()
			if !errors.Is(err, ErrReadOnly) || !strings.Contains(err.Error(), test.want+" is not allowed") {
				t.Fatalf("err = %v", err)
			}
			if value != nil {
				t.Fatalf("value = %#v, want nil", value)
			}
		})
	}
}

// Statements reach the database as text, so the boundary reads the text: what
// the statement starts with decides, and a tag in front of it does not.
func TestDatabaseReadonlyRefusesWritingStatements(t *testing.T) {
	for _, test := range []struct {
		query   string
		refused bool
	}{
		{query: "select id from users"},
		{query: "/* userGet */ SELECT * FROM user"},
		{query: "  \n show tables"},
		{query: "describe users"},
		{query: "insert into users (name) values ('Ada')", refused: true},
		{query: "/* userSave */ UPDATE user SET name = 'Ada'", refused: true},
		{query: "delete from users", refused: true},
		{query: "create table t (id integer)", refused: true},
		{query: "alter table users add column notes text", refused: true},
		{query: "-- just a comment", refused: true},
		{query: "", refused: true},
	} {
		t.Run(test.query, func(t *testing.T) {
			database := &Database{IsReadonly: true}
			err := database.refuseQuery(context.Background(), test.query)
			if refused := errors.Is(err, ErrReadOnly); refused != test.refused {
				t.Fatalf("refused = %t, err = %v", refused, err)
			}

			// The same statement on an unrestricted client is never refused.
			writable := &Database{}
			if err := writable.refuseQuery(context.Background(), test.query); err != nil {
				t.Fatalf("writable client refused %q: %v", test.query, err)
			}
		})
	}
}

// A refused statement never reaches the query log, so the span carrying the
// call is where it is recorded — otherwise a boundary that refuses the wrong
// thing leaves nothing behind to debug it with.
func TestDatabaseReadonlyRecordsRefusalOnSpan(t *testing.T) {
	tracer, err := telemetry.New(telemetry.NewOptions())
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}

	database := &Database{ID: "db", IsReadonly: true}
	if _, err := database.Query(ctx, "/* purge */ DELETE FROM user"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("err = %v", err)
	}

	spans := trace.Clone().Spans
	span := spans[len(spans)-1]
	if span.Name != "db.Query" || !span.Failed() || !strings.Contains(span.Error, "delete is not allowed") {
		t.Fatalf("span = %+v", span)
	}
	if span.Attributes["query_type"] != "delete" || span.Attributes["query_comment"] != "purge" {
		t.Fatalf("attributes = %+v", span.Attributes)
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
