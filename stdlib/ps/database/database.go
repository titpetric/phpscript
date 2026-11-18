package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// Database adds request tracing to the database binding.
type Database struct {
	// bridge is the client every method below uses to run its statement. It
	// stays out of the exported field set for the same reason IsReadonly is a
	// boundary at all: an exported field is a property a script reads, and
	// `$db->bridge->query()` would answer with a client that checks nothing.
	bridge *Bridge

	ID string

	// IsReadonly restricts the client to statements that only read. The flag
	// belongs to the client and not the connection: PHP reads and writes it as
	// `$db->is_readonly`. It lasts as long as the client does, which for a
	// served request is the request.
	//
	// It refuses insert(), replace() and update() outright, and refuses any
	// statement passed to query(), get() or get_all() that does not start with
	// a read-only keyword (see readOnlyStatements). Transactions, connection
	// pinning and the result accessors stay available, since a read-only
	// transaction is a read.
	//
	// The restriction is a boundary for the code holding this client. The
	// boundary does not sandbox the script: the script that set it can unset it.
	// Any connection that must not write belongs to a database user without
	// the grant to. This flag marks the code that must not write.
	IsReadonly bool

	// transaction is the span Begin opens and Commit or Rollback ends. One
	// region in the trace reads as a transaction; three markers do not.
	transaction *telemetry.Span
}

type databaseSpanKey struct{}

// ErrReadOnly matches a refusal by a read-only client through errors.Is. The
// runtime surfaces the refusal to PHP as a thrown exception. A script catches
// it like any other database error.
var ErrReadOnly = errors.New("database is read-only")

// readOnlyError names the statement a read-only client refused, matching ErrReadOnly.
type readOnlyError struct {
	refusal string
}

// Error renders the refusal as the message PHP catches.
func (e readOnlyError) Error() string {
	return ErrReadOnly.Error() + ": " + e.refusal
}

// Is matches the sentinel and errors.Is(err, ErrReadOnly) holds for a refusal.
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
	return b.bridge.Insert(ctx, table, value)
}

// Replace replaces a row using a dynamically typed named value.
func (b *Database) Replace(ctx context.Context, table string, value any) (any, error) {
	ctx, end := b.withSpan(ctx, "Replace")
	defer end()

	if err := b.refuseWrite(ctx, "replace"); err != nil {
		return nil, err
	}
	return b.bridge.Replace(ctx, table, value)
}

// Update updates a row using dynamically typed named values and key columns.
func (b *Database) Update(ctx context.Context, table string, value any, keyColumns ...any) (any, error) {
	ctx, end := b.withSpan(ctx, "Update")
	defer end()

	if err := b.refuseWrite(ctx, "update"); err != nil {
		return nil, err
	}
	return b.bridge.Update(ctx, table, value, keyColumns...)
}

// Query executes a statement with dynamically typed arguments.
func (b *Database) Query(ctx context.Context, query string, args ...any) (any, error) {
	ctx, end := b.withSpan(ctx, "Query")
	defer end()

	if err := b.refuseQuery(ctx, query); err != nil {
		return nil, err
	}
	return b.bridge.Query(ctx, query, args...)
}

// Get returns the first result row, or false when the query matches no rows.
//
// A row is a native map: foreach walks the row, a column name indexes it.
// unset() takes a column out. A map holds no column order; a script
// that needs one names its columns in the SELECT and indexes them.
func (b *Database) Get(ctx context.Context, query string, args ...any) (any, error) {
	// Rows reach PHP as the bridge produced them. Each row is a map[string]any;
	// each result set is a []map[string]any, without a copy into a
	// *model.Array; the copy cost two allocations plus an interface box per
	// column on every row of every query, and the bridge's map had already
	// lost the column order the copy would have fixed.
	ctx, end := b.withSpan(ctx, "Get")
	defer end()

	if err := b.refuseQuery(ctx, query); err != nil {
		return nil, err
	}
	value, err := b.bridge.Get(ctx, query, args...)
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
	value, err := b.bridge.GetAll(ctx, query, args...)
	if err == nil {
		if rows, ok := modelutil.LenValues(value); ok {
			recordRows(ctx, rows)
		}
	}
	return value, err
}

// Connect reserves an exclusive connection from the pool.
func (b *Database) Connect(ctx context.Context) (any, error) {
	defer b.span(ctx, "Connect").End()
	return b.bridge.Connect(ctx)
}

// Close releases an exclusive connection back to the pool.
func (b *Database) Close(ctx context.Context) (any, error) {
	defer b.span(ctx, "Close").End()
	return b.bridge.Close(ctx)
}

// Begin starts a transaction and opens the span measuring the transaction. The span stays
// open until Commit or Rollback ends it.
func (b *Database) Begin(ctx context.Context) (any, error) {
	if b.transaction == nil {
		b.transaction = b.span(ctx, "Begin")
	}
	value, err := b.bridge.Begin(ctx)
	b.transaction.RecordError(err)
	return value, err
}

// Rollback rolls back the active transaction and ends its span.
func (b *Database) Rollback(ctx context.Context) (any, error) {
	value, err := b.bridge.Rollback(ctx)
	b.endTransaction("Rollback", err)
	return value, err
}

// Commit commits the active transaction and ends its span.
func (b *Database) Commit(ctx context.Context) (any, error) {
	value, err := b.bridge.Commit(ctx)
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
	return b.bridge.InsertID(ctx)
}

// RowsAffected returns the affected row count from the last write.
func (b *Database) RowsAffected(ctx context.Context) (any, error) {
	defer b.span(ctx, "RowsAffected").End()
	return b.bridge.RowsAffected(ctx)
}

// observe records query, bound values, transaction depth and error on the span.
func (b *Database) observe(ctx context.Context, entry QueryLogEntry) {
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

// queryArgs returns the bound values untouched, reporting false for an empty set.
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
	if n, ok := modelutil.LenValues(args); ok && n == 0 {
		return nil, false
	}
	return args, true
}

// recordRows records a read's row count on the span measuring it.
func recordRows(ctx context.Context, rows int) {
	databaseSpan(ctx).SetAttribute("rows", rows)
}

// databaseSpan returns the span measuring the call in progress, nil when uninstrumented.
func databaseSpan(ctx context.Context) *telemetry.Span {
	span, _ := ctx.Value(databaseSpanKey{}).(*telemetry.Span)
	return span
}

// refuseWrite reports the error a read-only client returns for a CRUD helper.
func (b *Database) refuseWrite(ctx context.Context, statement string) error {
	if !b.IsReadonly {
		return nil
	}
	err := readOnlyError{refusal: statement + " is not allowed"}
	databaseSpan(ctx).RecordError(err)
	return err
}

// refuseQuery reports the error a read-only client returns for a non-reading statement.
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

// name qualifies a method with the client's PHP variable for a clear trace.
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

// withSpan starts a database span, returning a context and its closer.
func (b *Database) withSpan(ctx context.Context, method string) (context.Context, func()) {
	span := b.span(ctx, method)
	if span == nil {
		return ctx, func() {}
	}
	return context.WithValue(span.Context(ctx), databaseSpanKey{}, span), span.End
}

// Connect opens the connection the host registered under name, the call behind
// `new Database($name)` and Runtime\Go\Database\SQL\Open.
func Connect(rt *runner.Runtime, ctx context.Context, name string) (*Database, error) {
	handle, err := provider(rt).Connect(ctx, name)
	if err != nil {
		return nil, err
	}

	database := &Database{bridge: newBridge(handle)}
	database.bridge.WithObserver(database.observe)
	return database, nil
}
