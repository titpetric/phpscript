package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/telemetry"
)

func TestDatabaseObserverRecordsQuerySpan(t *testing.T) {
	t.Parallel()
	ctx, trace := newRecordedTrace(t)
	ctx = telemetry.WithSpanLine(telemetry.WithSpanFilename(ctx, "queries/users.php"), 23)

	database := &Database{ID: "db"}
	ctx, end := database.withSpan(ctx, "Get")
	database.observe(ctx, QueryLogEntry{
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

	// The values bound to the placeholders are what names the row.
	args, ok := span.Attributes["args"].([]any)
	if !ok || len(args) != 1 || args[0] != int64(42) {
		t.Fatalf("bound arguments = %#v", span.Attributes["args"])
	}
}

func TestDatabaseObserverRecordsNamedArguments(t *testing.T) {
	t.Parallel()
	span, observe := newObservedSpan(t)
	observe(QueryLogEntry{
		Query: "select * from users where id = :id",
		Args:  map[string]any{"id": int64(42)},
	})

	named, ok := span.Attributes["args"].(map[string]any)
	if !ok || named["id"] != int64(42) {
		t.Fatalf("named arguments = %#v", span.Attributes["args"])
	}
}

func TestDatabaseObserverOmitsEmptyArguments(t *testing.T) {
	t.Parallel()
	for _, args := range []any{nil, []any{}, map[string]any{}} {
		span, observe := newObservedSpan(t)
		observe(QueryLogEntry{Query: "select 1", Args: args})

		if value, ok := span.Attributes["args"]; ok {
			t.Fatalf("args = %#v for %#v, want no attribute", value, args)
		}
	}
}

// A query that found no rows is control flow and no failure: recording it would
// fail the span, the trace, and the SLA computed from them.
func TestDatabaseObserverIgnoresSentinelErrors(t *testing.T) {
	t.Parallel()
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
			observe(QueryLogEntry{Query: "select 1", Err: test.err})

			if got := span.Err() != nil; got != test.wantError {
				t.Fatalf("failed = %t, error = %q", got, span.ErrorText)
			}
		})
	}
}

// A statement is classified by the keyword it starts with, and the tag it
// carries is recorded beside it: a trace groups by a query's purpose, which
// the statement text alone does not answer.
func TestDatabaseObserverRecordsQueryTypeAndComment(t *testing.T) {
	t.Parallel()
	span, observe := newObservedSpan(t)
	observe(QueryLogEntry{Query: "/* userGet */ SELECT * FROM user WHERE id = ?"})

	if span.Attributes["query_type"] != "select" || span.Attributes["query_comment"] != "userGet" {
		t.Fatalf("attributes = %+v", span.Attributes)
	}
	// The comment stays in the statement, so `show processlist` shows it too.
	if span.Attributes["query"] != "/* userGet */ SELECT * FROM user WHERE id = ?" {
		t.Fatalf("query = %#v", span.Attributes["query"])
	}

	// An untagged statement has no attribute, and never an empty one.
	span, observe = newObservedSpan(t)
	observe(QueryLogEntry{Query: "select 1"})
	if _, ok := span.Attributes["query_comment"]; ok || span.Attributes["query_type"] != "select" {
		t.Fatalf("attributes = %+v", span.Attributes)
	}
}

// The CRUD helpers write by definition, so a read-only client refuses them
// before a statement is built. The nil Bridge is the assertion that nothing
// reached the database: running one would panic.
func TestDatabaseReadonlyRefusesWrites(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
// call is where it is recorded. Otherwise a boundary that refuses the wrong
// thing leaves nothing behind to debug it with.
func TestDatabaseReadonlyRecordsRefusalOnSpan(t *testing.T) {
	t.Parallel()
	ctx, trace := newRecordedTrace(t)

	database := &Database{ID: "db", IsReadonly: true}
	if _, err := database.Query(ctx, "/* purge */ DELETE FROM user"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("err = %v", err)
	}

	spans := trace.Clone().Spans
	span := spans[len(spans)-1]
	if span.Name != "db.Query" || span.Err() == nil || !strings.Contains(span.ErrorText, "delete is not allowed") {
		t.Fatalf("span = %+v", span)
	}
	if span.Attributes["query_type"] != "delete" || span.Attributes["query_comment"] != "purge" {
		t.Fatalf("attributes = %+v", span.Attributes)
	}
}

// newObservedSpan returns a recorded database span and the observer callback
// reporting a query onto it.
func newObservedSpan(t *testing.T) (*telemetry.Span, func(QueryLogEntry)) {
	t.Helper()

	ctx, trace := newRecordedTrace(t)

	database := &Database{}
	ctx, end := database.withSpan(ctx, "Get")
	t.Cleanup(end)

	span := trace.Root()
	for _, recorded := range trace.Spans {
		span = recorded
	}
	return span, func(entry QueryLogEntry) {
		database.observe(ctx, entry)
	}
}

// TestDatabaseRowsAreReadableAsPHPArrays pins the row representation Get and
// GetAll hand to the VM: the bridge's own []map[string]any, uncopied. The VM's
// value-model helpers must see it as an array-like value with the columns
// reachable by key, as foreach and $row["col"] both read it.
func TestDatabaseRowsAreReadableAsPHPArrays(t *testing.T) {
	t.Parallel()
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
	// set holds, the same reference semantics the *model.Array copy had.
	row.(map[string]any)["columns"] = int64(6)
	if got := rows.([]map[string]any)[0]["columns"]; got != int64(6) {
		t.Fatalf("added column = %#v", got)
	}
}

// TestDatabase_SetID covers the name a constructed client is recorded under.
// The runtime passes the receiving PHP variable, and every
// span the client opens carries it, so two clients in one request are told
// apart on the trace.
func TestDatabase_SetID(t *testing.T) {
	t.Parallel()
	database := &Database{}
	if got := database.name("Get"); got != "Get" {
		t.Errorf("an unnamed client records %q, want %q", got, "Get")
	}

	database.SetID("catalogue")
	if database.ID != "catalogue" {
		t.Errorf("ID = %q, want %q", database.ID, "catalogue")
	}
	if got, want := database.name("Get"), "catalogue.Get"; got != want {
		t.Errorf("name(Get) = %q, want %q", got, want)
	}
}

// TestDatabase_Connect covers pinning the client to one connection out of the
// pool. A temporary table and a session variable both live on a connection, so
// a script that creates one reserves the connection first; the reads that
// follow run on it.
func TestDatabase_Connect(t *testing.T) {
	t.Parallel()
	database := newLiveDatabase(t)

	if _, err := database.Connect(t.Context()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	value, err := database.Get(t.Context(), "select name from users where id = 1")
	if err != nil {
		t.Fatalf("Get on the reserved connection: %v", err)
	}
	if row := value.(map[string]any); row["name"] != "Ada" {
		t.Fatalf("the reserved connection read %#v", row)
	}
}

// TestDatabase_Close covers releasing the reserved connection. The pool here
// is one connection wide, so a read after Close is the evidence it went back:
// a reservation left open would block until the context expired.
func TestDatabase_Close(t *testing.T) {
	t.Parallel()
	database := newLiveDatabase(t)

	if _, err := database.Connect(t.Context()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if _, err := database.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := database.Get(t.Context(), "select name from users where id = 1"); err != nil {
		t.Fatalf("Get after Close: %v", err)
	}
}

// TestDatabase_Begin covers the span a transaction opens. It stays open until
// Commit or Rollback ends it, so a transaction reads as one region on the
// trace and no pair of markers a reader has to match up.
func TestDatabase_Begin(t *testing.T) {
	t.Parallel()
	ctx, _ := newRecordedTrace(t)
	database := newLiveDatabase(t)
	database.SetID("db")

	if _, err := database.Begin(ctx); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if database.transaction == nil {
		t.Fatal("Begin opened no span, want one held until the outcome")
	}
	if database.transaction.Ended() {
		t.Error("the transaction span closed at Begin, want it open until the outcome")
	}
}

// TestDatabase_Commit covers the committed outcome: the row survives the
// transaction, and the span Begin opened is renamed for the outcome and ended,
// so one span carries the whole transaction.
func TestDatabase_Commit(t *testing.T) {
	t.Parallel()
	ctx, trace := newRecordedTrace(t)
	database := newLiveDatabase(t)
	database.SetID("db")

	if _, err := database.Begin(ctx); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := database.Insert(ctx, "users", map[string]any{"id": 3, "name": "Katherine"}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := database.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if database.transaction != nil {
		t.Error("the transaction span is still held after Commit")
	}

	if span := spanNamed(t, trace, "db.Commit"); !span.Ended() {
		t.Errorf("transaction span = %+v, want it ended", span)
	}

	value, err := database.Get(ctx, "select name from users where id = 3")
	if err != nil {
		t.Fatalf("Get after Commit: %v", err)
	}
	if row := value.(map[string]any); row["name"] != "Katherine" {
		t.Fatalf("the committed row reads back as %#v", row)
	}
}

// TestDatabase_Rollback covers the discarded outcome: the row is gone, and the
// span Begin opened carries the rollback as its name, so a trace says which
// way a transaction ended without a second span to pair with it.
func TestDatabase_Rollback(t *testing.T) {
	t.Parallel()
	ctx, trace := newRecordedTrace(t)
	database := newLiveDatabase(t)
	database.SetID("db")

	if _, err := database.Begin(ctx); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := database.Insert(ctx, "users", map[string]any{"id": 4, "name": "Hedy"}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := database.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if span := spanNamed(t, trace, "db.Rollback"); !span.Ended() {
		t.Errorf("transaction span = %+v, want it ended", span)
	}

	missing, err := database.Get(ctx, "select name from users where id = 4")
	if err != nil {
		t.Fatalf("Get after Rollback: %v", err)
	}
	if missing != false {
		t.Errorf("the rolled back row reads back as %#v, want false", missing)
	}
}

// TestDatabase_InsertID covers the key the server generated for the last
// insert. It is the one part of an INSERT the statement does not carry, so a
// script writing a row with an autoincrement key reads the new id here.
func TestDatabase_InsertID(t *testing.T) {
	t.Parallel()
	database := newLiveDatabase(t)

	if _, err := database.Insert(t.Context(), "users", map[string]any{"name": "Katherine"}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	id, err := database.InsertID(t.Context())
	if err != nil {
		t.Fatalf("InsertID: %v", err)
	}
	if id != int64(3) {
		t.Errorf("InsertID() = %#v, want 3 after the two seeded rows", id)
	}
}

// TestDatabase_RowsAffected covers the count of the last write. An UPDATE
// names a condition and reports how many rows matched it, so a script learns
// whether the write landed from this and from nothing in the statement.
func TestDatabase_RowsAffected(t *testing.T) {
	t.Parallel()
	database := newLiveDatabase(t)

	if _, err := database.Query(t.Context(), "update users set name = 'Grace Hopper' where id = 2"); err != nil {
		t.Fatalf("Query: %v", err)
	}
	rows, err := database.RowsAffected(t.Context())
	if err != nil {
		t.Fatalf("RowsAffected: %v", err)
	}
	if rows != int64(1) {
		t.Errorf("RowsAffected() = %#v, want 1", rows)
	}
}

// newRecordedTrace returns a context carrying a recording trace, and the trace
// itself, so a test reads back the spans a call left on it.
func newRecordedTrace(t *testing.T) (context.Context, *telemetry.Trace) {
	t.Helper()

	options := telemetry.NewOptions("phpscript")
	// oida records only when it is told to; a test that reads its traces
	// back tells it to.
	options.Enabled = true
	tracer, err := telemetry.New(options)
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}
	return ctx, trace
}

// spanNamed returns the span of one name off a trace. The spans arrive in the
// order they opened, and a transaction span opens first and ends last, so the
// last span recorded is seldom the one a test is reading.
func spanNamed(t *testing.T, trace *telemetry.Trace, name string) *telemetry.Span {
	t.Helper()

	spans := trace.Clone().Spans
	for _, span := range spans {
		if span.Name == name {
			return span
		}
	}
	t.Fatalf("no span named %q among %+v", name, spans)
	return nil
}
