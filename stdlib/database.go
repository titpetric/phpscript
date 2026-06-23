package stdlib

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// RegisterDatabase installs the Go database bridge used by PHP via
// `new DatabaseDriver("driver://dsn")`.
func RegisterDatabase(rt *runner.Runtime) {
	rt.RegisterConstructor("DatabaseDriver", NewDatabaseDriver)
}

// DatabaseDriver wraps a SQL database connection for PHP scripts.
type DatabaseDriver struct {
	db *sql.DB
}

// NewDatabaseDriver opens a database connection from the named DB_DSN_* env var.
func NewDatabaseDriver(ctx context.Context, name string) (*DatabaseDriver, error) {
	envKey := "DB_DSN_" + strings.ToUpper(name)
	dsn := os.Getenv(envKey)
	if dsn == "" {
		return nil, fmt.Errorf("Can't connect to %s, no %s env", name, envKey)
	}

	if !strings.Contains(dsn, "://") {
		return nil, fmt.Errorf("malformed dsn, expecting driver://connection_string, got %q", dsn)
	}

	driver, name, _ := strings.Cut(dsn, "://")
	db, err := func() (*sql.DB, error) {
		// postgres keeps driver in DSN
		if driver == "postgres" {
			return sql.Open("pgx", dsn)
		}
		return sql.Open(driver, name)
	}()
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	// Set connection pool limits - max 20 open connections
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(20)

	return &DatabaseDriver{db: db}, nil
}

// Close closes the underlying database connection.
func (d *DatabaseDriver) Close() {
	d.db.Close()
	d.db = nil
}

// Prepare creates a database statement for query.
func (d *DatabaseDriver) Prepare(query string) (*DatabaseStatement, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("database: nil driver")
	}
	return &DatabaseStatement{db: d.db, query: query, named: map[string]any{}, positional: map[int64]any{}}, nil
}

// LastInsertId returns the SQLite last inserted row ID.
func (d *DatabaseDriver) LastInsertId(ctx context.Context) (int64, error) {
	if d == nil || d.db == nil {
		return 0, fmt.Errorf("database: nil driver")
	}
	var id int64
	if err := d.db.QueryRowContext(ctx, "select last_insert_rowid()").Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// DatabaseStatement holds a prepared query and bound values for execution.
type DatabaseStatement struct {
	db         *sql.DB
	query      string
	named      map[string]any
	positional map[int64]any
	rows       *sql.Rows
}

// BindValue binds value to a named or positional query parameter.
func (s *DatabaseStatement) BindValue(key, value any) error {
	if s == nil {
		return fmt.Errorf("database: nil statement")
	}
	switch k := key.(type) {
	case string:
		s.named[strings.TrimPrefix(k, ":")] = value
	case int64:
		s.positional[k] = value
	case int:
		s.positional[int64(k)] = value
	default:
		return fmt.Errorf("database: unsupported bind key %T", key)
	}
	return nil
}

// Execute runs the statement and stores the result cursor.
func (s *DatabaseStatement) Execute(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("database: nil statement")
	}
	args := s.args()
	rows, err := s.db.QueryContext(ctx, s.query, args...)
	if err != nil {
		return err
	}
	if err := s.CloseCursor(); err != nil {
		rows.Close()
		return err
	}
	s.rows = rows
	return nil
}

// Fetch returns the next result row or false when no rows remain.
func (s *DatabaseStatement) Fetch() (any, error) {
	if s == nil {
		return false, fmt.Errorf("database: nil statement")
	}
	if s.rows == nil || !s.rows.Next() {
		if s.rows != nil {
			if err := s.rows.Err(); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	cols, err := s.rows.Columns()
	if err != nil {
		return false, err
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := s.rows.Scan(ptrs...); err != nil {
		return false, err
	}
	out := model.NewArray()
	for i, col := range cols {
		out.Set(col, normalizeSQLValue(vals[i]))
	}
	return out, nil
}

// CloseCursor closes the active result cursor.
func (s *DatabaseStatement) CloseCursor() error {
	if s == nil {
		return fmt.Errorf("database: nil statement")
	}
	if s.rows == nil {
		return nil
	}
	err := s.rows.Close()
	s.rows = nil
	return err
}

func (s *DatabaseStatement) args() []any {
	if len(s.named) > 0 {
		args := make([]any, 0, len(s.named))
		for k, v := range s.named {
			args = append(args, sql.Named(k, v))
		}
		return args
	}
	args := make([]any, 0, len(s.positional))
	for i := int64(1); i <= int64(len(s.positional)); i++ {
		args = append(args, s.positional[i])
	}
	return args
}

func normalizeSQLValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case int64, float64, string, bool, nil:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}
