package sqlite55

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// Adapter wraps *sql.DB with user-friendly SQLite helpers and sensible defaults.
type Adapter struct {
	db  *sql.DB
	dsn string
	mu  sync.RWMutex
}

// Open creates an Adapter connected to a SQLite database file or DSN with WAL & busy_timeout.
func Open(dsn string) (*Adapter, error) {
	if dsn == "" {
		dsn = "sqlite://file:memory?mode=memory&cache=shared"
	}
	cleanDSN := dsn
	if !strings.HasPrefix(cleanDSN, "sqlite://") && !strings.HasPrefix(cleanDSN, "file:") && dsn != ":memory:" {
		cleanDSN = fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON", dsn)
	}

	driverDSN := strings.TrimPrefix(cleanDSN, "sqlite://")
	db, err := sql.Open("sqlite", driverDSN)
	if err != nil {
		return nil, fmt.Errorf("sqlite open error: %w", err)
	}

	// Pragma defaults
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	_, _ = db.Exec("PRAGMA busy_timeout=5000;")
	_, _ = db.Exec("PRAGMA foreign_keys=ON;")

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
	return a.db.Close()
}

// Execute runs a DDL/DML statement and returns affected rows count.
func (a *Adapter) Execute(ctx context.Context, query string, args ...any) (int64, error) {
	if a == nil || a.db == nil {
		return 0, fmt.Errorf("sqlite adapter not connected")
	}
	res, err := a.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
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

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	cols := strings.Join(keys, ", ")
	placeholders := make([]string, len(keys))
	vals := make([]any, len(keys))
	for i, k := range keys {
		placeholders[i] = "?"
		vals[i] = data[k]
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, cols, strings.Join(placeholders, ", "))
	res, err := a.db.ExecContext(ctx, query, vals...)
	if err != nil {
		return 0, fmt.Errorf("insert failed: %w", err)
	}
	return res.LastInsertId()
}

// First queries and returns a single row as a map[string]any, or nil if not found.
func (a *Adapter) First(ctx context.Context, query string, args ...any) (map[string]any, error) {
	rows, err := a.All(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// All queries and returns all matching rows as []map[string]any.
func (a *Adapter) All(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("sqlite adapter not connected")
	}

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		valPtrs := make([]any, len(cols))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}

		if err := rows.Scan(valPtrs...); err != nil {
			return nil, err
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

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	setClauses := make([]string, len(keys))
	queryArgs := make([]any, 0, len(keys)+len(whereArgs))
	for i, k := range keys {
		setClauses[i] = fmt.Sprintf("%s = ?", k)
		queryArgs = append(queryArgs, data[k])
	}
	queryArgs = append(queryArgs, whereArgs...)

	query := fmt.Sprintf("UPDATE %s SET %s", table, strings.Join(setClauses, ", "))
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

	query := fmt.Sprintf("DELETE FROM %s", table)
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
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
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
	return &PHPBridge{adapter: adapter, ctx: ctx}, nil
}

// PHPBridge provides methods exposed to PHP scripts.
type PHPBridge struct {
	adapter *Adapter
	ctx     context.Context
}

func (b *PHPBridge) Execute(query string, args ...any) (int64, error) {
	return b.adapter.Execute(b.ctx, query, unpackArgs(args)...)
}

func (b *PHPBridge) Insert(table string, data any) (int64, error) {
	return b.adapter.Insert(b.ctx, table, modelArrayToMap(data))
}

func (b *PHPBridge) First(query string, args ...any) (*model.Array, error) {
	row, err := b.adapter.First(b.ctx, query, unpackArgs(args)...)
	if err != nil || row == nil {
		return nil, err
	}
	return mapToModelArray(row), nil
}

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

func (b *PHPBridge) Update(table string, data any, where string, whereArgs ...any) (int64, error) {
	return b.adapter.Update(b.ctx, table, modelArrayToMap(data), where, unpackArgs(whereArgs)...)
}

func (b *PHPBridge) Delete(table string, where string, whereArgs ...any) (int64, error) {
	return b.adapter.Delete(b.ctx, table, where, unpackArgs(whereArgs)...)
}

func (b *PHPBridge) Close() error {
	return b.adapter.Close()
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
