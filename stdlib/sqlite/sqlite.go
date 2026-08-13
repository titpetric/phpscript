package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

var validIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Adapter wraps *sql.DB with user-friendly SQLite helpers and sensible defaults.
type Adapter struct {
	db  *sql.DB
	dsn string
}

// Open creates an Adapter connected to a SQLite database file or DSN with WAL & busy_timeout.
func Open(dsn string) (*Adapter, error) {
	if dsn == "" {
		dsn = "sqlite://file:memory?mode=memory&cache=shared"
	}

	raw := dsn
	if strings.HasPrefix(raw, "sqlite://") {
		raw = strings.TrimPrefix(raw, "sqlite://")
	}

	if !strings.Contains(raw, "_foreign_keys=") {
		if strings.Contains(raw, "?") {
			raw += "&_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON"
		} else {
			raw += "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON"
		}
	}

	db, err := sql.Open("sqlite", raw)
	if err != nil {
		return nil, fmt.Errorf("sqlite open error: %w", err)
	}

	// Enforce connection limits to prevent FD leaks & pool exhaustion
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Execute PRAGMAs with strict error checking
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to set pragma %q: %w", pragma, err)
		}
	}

	return &Adapter{
		db:  db,
		dsn: dsn,
	}, nil
}

// Memory opens an in-memory SQLite Adapter.
func Memory() (*Adapter, error) {
	return Open("sqlite://file:memory?mode=memory&cache=shared")
}

// DB returns the underlying *sql.DB instance.
func (a *Adapter) DB() *sql.DB {
	return a.db
}

// Close closes the database connection.
func (a *Adapter) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	err := a.db.Close()
	a.db = nil
	return err
}

// Execute runs a DDL/DML statement and returns affected rows count.
func (a *Adapter) Execute(ctx context.Context, query string, args ...any) (int64, error) {
	if a == nil || a.db == nil {
		return 0, fmt.Errorf("sqlite adapter not connected")
	}
	res, err := a.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("execute query failed: %w", err)
	}
	return res.RowsAffected()
}

// Insert inserts a map of values into table and returns the last inserted ID.
func (a *Adapter) Insert(ctx context.Context, table string, data map[string]any) (int64, error) {
	if a == nil || a.db == nil {
		return 0, fmt.Errorf("sqlite adapter not connected")
	}
	if len(data) == 0 {
		return 0, fmt.Errorf("insert data cannot be empty")
	}

	quotedTable, err := sanitizeIdent(table)
	if err != nil {
		return 0, err
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	quotedCols := make([]string, len(keys))
	placeholders := make([]string, len(keys))
	vals := make([]any, len(keys))
	for i, k := range keys {
		qCol, err := sanitizeIdent(k)
		if err != nil {
			return 0, err
		}
		quotedCols[i] = qCol
		placeholders[i] = "?"
		vals[i] = data[k]
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quotedTable, strings.Join(quotedCols, ", "), strings.Join(placeholders, ", "))
	res, err := a.db.ExecContext(ctx, query, vals...)
	if err != nil {
		return 0, fmt.Errorf("insert failed: %w", err)
	}
	return res.LastInsertId()
}

// First queries and returns a single row as a map[string]any, or nil if not found.
func (a *Adapter) First(ctx context.Context, query string, args ...any) (map[string]any, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("sqlite adapter not connected")
	}

	q := strings.TrimRight(strings.TrimSpace(query), ";")
	upper := strings.ToUpper(q)
	if strings.HasPrefix(upper, "SELECT") && !strings.Contains(upper, " LIMIT ") && !strings.HasSuffix(upper, " LIMIT") {
		q += " LIMIT 1"
	}

	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("first query failed: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to read columns: %w", err)
	}

	if !rows.Next() {
		return nil, nil
	}

	vals := make([]any, len(cols))
	valPtrs := make([]any, len(cols))
	for i := range vals {
		valPtrs[i] = &vals[i]
	}

	if err := rows.Scan(valPtrs...); err != nil {
		return nil, fmt.Errorf("row scan failed: %w", err)
	}

	rowMap := make(map[string]any, len(cols))
	for i, col := range cols {
		val := vals[i]
		if b, ok := val.([]byte); ok {
			rowMap[col] = string(b)
		} else {
			rowMap[col] = val
		}
	}
	return rowMap, rows.Err()
}

// All queries and returns all matching rows as []map[string]any.
func (a *Adapter) All(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("sqlite adapter not connected")
	}

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("all query failed: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to read columns: %w", err)
	}

	var results []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		valPtrs := make([]any, len(cols))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}

		if err := rows.Scan(valPtrs...); err != nil {
			return nil, fmt.Errorf("row scan failed: %w", err)
		}

		rowMap := make(map[string]any, len(cols))
		for i, col := range cols {
			val := vals[i]
			if b, ok := val.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = val
			}
		}
		results = append(results, rowMap)
	}

	return results, rows.Err()
}

// Update updates rows matching condition in table with data.
func (a *Adapter) Update(ctx context.Context, table string, data map[string]any, where string, whereArgs ...any) (int64, error) {
	if a == nil || a.db == nil {
		return 0, fmt.Errorf("sqlite adapter not connected")
	}
	if len(data) == 0 {
		return 0, fmt.Errorf("update data cannot be empty")
	}

	quotedTable, err := sanitizeIdent(table)
	if err != nil {
		return 0, err
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	setClauses := make([]string, len(keys))
	queryArgs := make([]any, 0, len(keys)+len(whereArgs))
	for i, k := range keys {
		qCol, err := sanitizeIdent(k)
		if err != nil {
			return 0, err
		}
		setClauses[i] = fmt.Sprintf("%s = ?", qCol)
		queryArgs = append(queryArgs, data[k])
	}
	queryArgs = append(queryArgs, whereArgs...)

	query := fmt.Sprintf("UPDATE %s SET %s", quotedTable, strings.Join(setClauses, ", "))
	if where != "" {
		query += " WHERE " + where
	}

	res, err := a.db.ExecContext(ctx, query, queryArgs...)
	if err != nil {
		return 0, fmt.Errorf("update failed: %w", err)
	}
	return res.RowsAffected()
}

// Delete deletes rows matching condition in table.
func (a *Adapter) Delete(ctx context.Context, table string, where string, whereArgs ...any) (int64, error) {
	if a == nil || a.db == nil {
		return 0, fmt.Errorf("sqlite adapter not connected")
	}

	quotedTable, err := sanitizeIdent(table)
	if err != nil {
		return 0, err
	}

	query := fmt.Sprintf("DELETE FROM %s", quotedTable)
	if where != "" {
		query += " WHERE " + where
	}

	res, err := a.db.ExecContext(ctx, query, whereArgs...)
	if err != nil {
		return 0, fmt.Errorf("delete failed: %w", err)
	}
	return res.RowsAffected()
}

// Transaction executes fn inside a database transaction with automatic rollback on error.
func (a *Adapter) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("sqlite adapter not connected")
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("transaction failed and rolled back: %w", err)
	}

	return tx.Commit()
}

// Register registers the PHP bridge for PS\SQLite constructor on Runtime.
func Register(rt *runner.Runtime) {
	rt.RegisterConstructor("PS\\SQLite", NewPHPBridge)
	rt.RegisterConstructor("SQLite", NewPHPBridge)
}

// NewPHPBridge constructor bridge for PHP `new PS\SQLite($dsn)` or `new SQLite($dsn)`
func NewPHPBridge(ctx context.Context, dsn ...string) (*PHPBridge, error) {
	target := "sqlite://file:memory?mode=memory&cache=shared"
	if len(dsn) > 0 && dsn[0] != "" {
		target = dsn[0]
	}
	adapter, err := Open(target)
	if err != nil {
		return nil, err
	}

	bridge := &PHPBridge{adapter: adapter, ctx: ctx}
	runtime.SetFinalizer(bridge, func(b *PHPBridge) {
		if b != nil && b.adapter != nil {
			_ = b.adapter.Close()
		}
	})
	return bridge, nil
}

// PHPBridge provides methods exposed to PHP scripts.
type PHPBridge struct {
	adapter *Adapter
	ctx     context.Context
}

// Execute executes a DDL/DML query with optional arguments.
func (b *PHPBridge) Execute(query string, args ...any) (int64, error) {
	return b.adapter.Execute(b.ctx, query, unpackArgs(args)...)
}

// Insert inserts a row map into table and returns inserted ID.
func (b *PHPBridge) Insert(table string, data any) (int64, error) {
	return b.adapter.Insert(b.ctx, table, modelArrayToMap(data))
}

// First fetches the first row matching query.
func (b *PHPBridge) First(query string, args ...any) (*model.Array, error) {
	row, err := b.adapter.First(b.ctx, query, unpackArgs(args)...)
	if err != nil || row == nil {
		return nil, err
	}
	return mapToModelArray(row), nil
}

// All fetches all rows matching query.
func (b *PHPBridge) All(query string, args ...any) (*model.Array, error) {
	rows, err := b.adapter.All(b.ctx, query, unpackArgs(args)...)
	if err != nil {
		return nil, err
	}
	arr := model.NewArray()
	for i, row := range rows {
		arr.Set(int64(i), mapToModelArray(row))
	}
	return arr, nil
}

// Update updates rows in table matching where condition.
func (b *PHPBridge) Update(table string, data any, where string, whereArgs ...any) (int64, error) {
	return b.adapter.Update(b.ctx, table, modelArrayToMap(data), where, unpackArgs(whereArgs)...)
}

// Delete deletes rows in table matching where condition.
func (b *PHPBridge) Delete(table string, where string, whereArgs ...any) (int64, error) {
	return b.adapter.Delete(b.ctx, table, where, unpackArgs(whereArgs)...)
}

// Close closes the underlying SQLite connection.
func (b *PHPBridge) Close() error {
	if b == nil || b.adapter == nil {
		return nil
	}
	return b.adapter.Close()
}

func sanitizeIdent(s string) (string, error) {
	if !validIdent.MatchString(s) {
		return "", fmt.Errorf("invalid SQL identifier %q: must match ^[A-Za-z_][A-Za-z0-9_]*$", s)
	}
	return `"` + s + `"`, nil
}

func modelArrayToMap(val any) map[string]any {
	out := make(map[string]any)
	if arr, ok := val.(*model.Array); ok && arr != nil {
		arr.Range(func(k, v any) bool {
			out[fmt.Sprint(k)] = v
			return true
		})
	} else if m, ok := val.(map[string]any); ok {
		return m
	}
	return out
}

func unpackArgs(args []any) []any {
	if len(args) == 1 {
		if arr, ok := args[0].(*model.Array); ok && arr != nil {
			var slice []any
			arr.Range(func(_, v any) bool {
				slice = append(slice, v)
				return true
			})
			return slice
		}
	}
	return args
}

func mapToModelArray(m map[string]any) *model.Array {
	arr := model.NewArray()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		arr.Set(k, m[k])
	}
	return arr
}
