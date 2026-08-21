package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/titpetric/pdo/client"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/telemetry"
)

// Database adds request tracing to the database binding.
type Database struct {
	Bridge *client.Bridge
	ID     string

	// IsReadonly restricts the client to statements that only read. It is a
	// property of the client, not of the connection: PHP reads and writes it as
	// `$db->is_readonly`, and it lives as long as the client does, which for a
	// served request is the request.
	//
	// It refuses insert(), replace() and update() outright, and refuses any
	// statement passed to query(), get() or get_all() that does not start with
	// a read-only keyword (see readOnlyStatements). Transactions, connection
	// pinning and the result accessors stay available, since a read-only
	// transaction is a read.
	//
	// The restriction is a boundary for the code holding this client, not a
	// sandbox around the script: the script that set it can unset it. A
	// connection that must not write belongs to a database user without the
	// grant to, and this marks the code that must not write.
	IsReadonly bool

	// transaction is the span opened by Begin and ended by Commit or Rollback,
	// so a transaction reads as one region rather than three markers.
	transaction *telemetry.Span
}

type databaseSpanKey struct{}

// ErrReadOnly is what a refusal by a read-only client matches with errors.Is.
// The runtime surfaces the refusal to PHP as a thrown exception, so a script
// catches it like any other database error.
var ErrReadOnly = errors.New("database is read-only")

// readOnlyError is a refusal by a read-only client. It names the statement it
// refused, and matches ErrReadOnly without wrapping it: the flat backend
// unwraps a caught error to its root cause, and the caught message has to say
// which statement was lost rather than only that the client cannot write.
type readOnlyError struct {
	refusal string
}

// Error renders the refusal as the message PHP catches.
func (e readOnlyError) Error() string {
	return ErrReadOnly.Error() + ": " + e.refusal
}

// Is matches the sentinel, so errors.Is(err, ErrReadOnly) holds for a refusal.
func (e readOnlyError) Is(target error) bool {
	return target == ErrReadOnly
}

// SetID records the PHP variable receiving this constructed client.
func (b *Database) SetID(id string) {
	b.ID = id
}

// Insert inserts a dynamically typed named value.
func (b *Database) Insert(ctx context.Context, table string, value any) (any, error) {
	ctx, end := b.withSpan(ctx, "Insert")
	defer end()

	if err := b.refuseWrite(ctx, "insert"); err != nil {
		return nil, err
	}
	return b.Bridge.Insert(ctx, table, value)
}

// Replace replaces a row using a dynamically typed named value.
func (b *Database) Replace(ctx context.Context, table string, value any) (any, error) {
	ctx, end := b.withSpan(ctx, "Replace")
	defer end()

	if err := b.refuseWrite(ctx, "replace"); err != nil {
		return nil, err
	}
	return b.Bridge.Replace(ctx, table, value)
}

// Update updates a row using dynamically typed named values and key columns.
func (b *Database) Update(ctx context.Context, table string, value any, keyColumns ...any) (any, error) {
	ctx, end := b.withSpan(ctx, "Update")
	defer end()

	if err := b.refuseWrite(ctx, "update"); err != nil {
		return nil, err
	}
	return b.Bridge.Update(ctx, table, value, keyColumns...)
}

// Query executes a statement with dynamically typed arguments.
func (b *Database) Query(ctx context.Context, query string, args ...any) (any, error) {
	ctx, end := b.withSpan(ctx, "Query")
	defer end()

	if err := b.refuseQuery(ctx, query); err != nil {
		return nil, err
	}
	return b.Bridge.Query(ctx, query, args...)
}

// Get returns the first result row, or false when the query matches no rows.
//
// A row is a native map: foreach walks it, $row["col"] indexes it, and
// $row["extra"] = 1 writes to it. A map carries no column order, so
// `foreach ($row as $column => $value)` visits columns in arbitrary order;
// a script that needs a stable order names its columns in the SELECT and
// indexes them.
func (b *Database) Get(ctx context.Context, query string, args ...any) (any, error) {
	// Rows reach PHP as the bridge produced them, a map[string]any per row and
	// a []map[string]any per result set, rather than being copied into a
	// *model.Array; the copy cost two allocations plus an interface box per
	// column on every row of every query, and the bridge's map had already
	// lost the column order the copy would have fixed.
	ctx, end := b.withSpan(ctx, "Get")
	defer end()

	if err := b.refuseQuery(ctx, query); err != nil {
		return nil, err
	}
	value, err := b.Bridge.Get(ctx, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		recordRows(ctx, 0)
		return false, nil
	}
	if err == nil {
		recordRows(ctx, 1)
	}
	return value, err
}

// GetAll returns all result rows. See Get for the row representation.
func (b *Database) GetAll(ctx context.Context, query string, args ...any) (any, error) {
	ctx, end := b.withSpan(ctx, "GetAll")
	defer end()

	if err := b.refuseQuery(ctx, query); err != nil {
		return nil, err
	}
	value, err := b.Bridge.GetAll(ctx, query, args...)
	if err == nil {
		if rows, ok := model.LenValues(value); ok {
			recordRows(ctx, rows)
		}
	}
	return value, err
}

// Connect reserves an exclusive connection from the pool.
func (b *Database) Connect(ctx context.Context) (any, error) {
	defer b.span(ctx, "Connect").End()
	return b.Bridge.Connect(ctx)
}

// Close releases an exclusive connection back to the pool.
func (b *Database) Close(ctx context.Context) (any, error) {
	defer b.span(ctx, "Close").End()
	return b.Bridge.Close(ctx)
}

// Begin starts a transaction and opens the span measuring it. The span stays
// open until Commit or Rollback, so a transaction is one region in the trace
// rather than an open marker and a close marker to pair up.
func (b *Database) Begin(ctx context.Context) (any, error) {
	if b.transaction == nil {
		b.transaction = b.span(ctx, "Begin")
	}
	value, err := b.Bridge.Begin(ctx)
	b.transaction.RecordError(err)
	return value, err
}

// Rollback rolls back the active transaction and ends its span.
func (b *Database) Rollback(ctx context.Context) (any, error) {
	value, err := b.Bridge.Rollback(ctx)
	b.endTransaction("Rollback", err)
	return value, err
}

// Commit commits the active transaction and ends its span.
func (b *Database) Commit(ctx context.Context) (any, error) {
	value, err := b.Bridge.Commit(ctx)
	b.endTransaction("Commit", err)
	return value, err
}

// endTransaction ends the span Begin opened, naming it after the outcome.
func (b *Database) endTransaction(outcome string, err error) {
	if b.transaction == nil {
		return
	}
	b.transaction.SetName(b.name(outcome))
	b.transaction.RecordError(err)
	b.transaction.End()
	b.transaction = nil
}

// InsertID returns the ID generated by the last insert.
func (b *Database) InsertID(ctx context.Context) (any, error) {
	defer b.span(ctx, "InsertID").End()
	return b.Bridge.InsertID(ctx)
}

// RowsAffected returns the affected row count from the last write.
func (b *Database) RowsAffected(ctx context.Context) (any, error) {
	defer b.span(ctx, "RowsAffected").End()
	return b.Bridge.RowsAffected(ctx)
}

// observe records what the query log reports onto the span of the call that
// produced it. The span already measures the call, so the statement, the values
// bound to it and the transaction depth are what this adds. Every method is nil
// safe, so an uninstrumented run takes the same path.
//
// The bound values are recorded, not just their count: a placeholder query
// says nothing about which row was read, and reading that back is the reason
// to open the trace at all. They are as sensitive as the columns they filter
// on, so the front end belongs behind something that authenticates.
func (b *Database) observe(ctx context.Context, entry client.QueryLogEntry) {
	span := databaseSpan(ctx)
	span.SetAttribute("query", entry.Query)
	parseQuery(entry.Query).record(span)
	if args, ok := queryArgs(entry.Args); ok {
		span.SetAttribute("args", args)
	}
	span.SetAttribute("transaction_depth", entry.TxDepth)
	if telemetry.Recordable(entry.Err) {
		span.RecordError(entry.Err)
	}
}

// queryArgs renders the values bound to a statement. The bridge hands over
// positional arguments as a slice and named ones as the map or struct they came
// from, so both shapes are kept as they are: the front end renders a slice and
// a map perfectly well, and flattening them would lose which name held what.
// An empty set is not recorded, so a query without placeholders has no
// attribute rather than an empty one.
func queryArgs(args any) (any, bool) {
	if args == nil {
		return nil, false
	}
	if values, ok := args.([]any); ok {
		if len(values) == 0 {
			return nil, false
		}
		return values, true
	}
	if n, ok := model.LenValues(args); ok && n == 0 {
		return nil, false
	}
	return args, true
}

// recordRows records how much a read returned on the span carrying it. The row
// count is the other half of a query: a statement that took 40ms says something
// different when it returned 3 rows than when it returned 30,000.
func recordRows(ctx context.Context, rows int) {
	databaseSpan(ctx).SetAttribute("rows", rows)
}

// databaseSpan returns the span measuring the call in progress, or nil when the
// run is uninstrumented. Every span method is nil safe, so callers do not check.
func databaseSpan(ctx context.Context) *telemetry.Span {
	span, _ := ctx.Value(databaseSpanKey{}).(*telemetry.Span)
	return span
}

// refuseWrite reports the error a read-only client returns for a CRUD helper.
// There is nothing to classify: insert(), replace() and update() write by
// definition, so they are refused before a statement is built for them.
func (b *Database) refuseWrite(ctx context.Context, statement string) error {
	if !b.IsReadonly {
		return nil
	}
	err := readOnlyError{refusal: statement + " is not allowed"}
	databaseSpan(ctx).RecordError(err)
	return err
}

// refuseQuery reports the error a read-only client returns for a statement that
// is not a read, classifying it by the keyword it starts with.
//
// A refused statement never reaches the query log, so what it was is recorded
// here: a boundary nobody can see being enforced is a boundary nobody can debug
// when it refuses the wrong thing.
func (b *Database) refuseQuery(ctx context.Context, query string) error {
	if !b.IsReadonly {
		return nil
	}
	info := parseQuery(query)
	if info.isRead() {
		return nil
	}

	err := readOnlyError{refusal: info.refusal()}
	span := databaseSpan(ctx)
	span.SetAttribute("query", query)
	info.record(span)
	span.RecordError(err)
	return err
}

// name qualifies a method with the PHP variable holding this client, so two
// clients in one request are told apart in the trace.
func (b *Database) name(method string) string {
	if b.ID == "" {
		return method
	}
	return b.ID + "." + method
}

// span records one database span for a client call.
func (b *Database) span(ctx context.Context, method string) *telemetry.Span {
	return telemetry.StartSpan(ctx, b.name(method), telemetry.KindDatabase)
}

// withSpan starts a database span and returns a context carrying it for the
// query log observer, together with the closer that ends it.
func (b *Database) withSpan(ctx context.Context, method string) (context.Context, func()) {
	span := b.span(ctx, method)
	if span == nil {
		return ctx, func() {}
	}
	return context.WithValue(span.Context(ctx), databaseSpanKey{}, span), span.End
}
