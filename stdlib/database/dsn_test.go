package database

import "testing"

func TestCleanDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "empty DSN",
			dsn:  "",
			want: "?collation=utf8mb4_general_ci&parseTime=true&loc=Local",
		},
		{
			name: "dsn with question mark",
			dsn:  "user:pass@tcp(localhost:3306)/dbname?",
			want: "user:pass@tcp(localhost:3306)/dbname?collation=utf8mb4_general_ci&parseTime=true&loc=Local",
		},
		{
			name: "dsn with collation set",
			dsn:  "user:pass@tcp(localhost:3306)/dbname?collation=utf8",
			want: "user:pass@tcp(localhost:3306)/dbname?collation=utf8&parseTime=true&loc=Local",
		},
		{
			name: "dsn with parseTime set",
			dsn:  "user:pass@tcp(localhost:3306)/dbname?parseTime=false",
			want: "user:pass@tcp(localhost:3306)/dbname?parseTime=false&collation=utf8mb4_general_ci&loc=Local",
		},
		{
			name: "dsn with loc set",
			dsn:  "user:pass@tcp(localhost:3306)/dbname?loc=UTC",
			want: "user:pass@tcp(localhost:3306)/dbname?loc=UTC&collation=utf8mb4_general_ci&parseTime=true",
		},
		{
			name: "dsn with all options set",
			dsn:  "user:pass@tcp(localhost:3306)/dbname?collation=abc&parseTime=abc&loc=abc",
			want: "user:pass@tcp(localhost:3306)/dbname?collation=abc&parseTime=abc&loc=abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanDSN("mysql", tt.dsn); got != tt.want {
				t.Errorf("cleanDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanDSNDriverSpecific(t *testing.T) {
	dsn := "user=postgres password=secret host=127.0.0.1 port=15432 dbname=postgres sslmode=verify-ca"

	for _, driver := range []string{"postgres", "pgx"} {
		t.Run(driver, func(t *testing.T) {
			if got := cleanDSN(driver, dsn); got != dsn {
				t.Errorf("cleanDSN(%s) = %q, want %q", driver, got, dsn)
			}
		})
	}
}

func TestCleanSQLiteDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "file database",
			dsn:  "app.db",
			want: "app.db?_busy_timeout=5000&_journal_mode=wal",
		},
		{
			name: "existing query",
			dsn:  "file:app.db?cache=shared",
			want: "file:app.db?cache=shared&_busy_timeout=5000&_journal_mode=wal",
		},
		{
			name: "explicit options",
			dsn:  "app.db?_busy_timeout=1000&_journal_mode=delete",
			want: "app.db?_busy_timeout=1000&_journal_mode=delete",
		},
		{
			name: "missing journal mode",
			dsn:  "app.db?_busy_timeout=1000",
			want: "app.db?_busy_timeout=1000&_journal_mode=wal",
		},
		{
			name: "memory database",
			dsn:  ":memory:",
			want: ":memory:",
		},
		{
			name: "memory URI",
			dsn:  "file::memory:?cache=shared",
			want: "file::memory:?cache=shared",
		},
		{
			name: "named shared memory database",
			dsn:  "file:app?mode=memory&cache=shared",
			want: "file:app?mode=memory&cache=shared",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanDSN("sqlite", tt.dsn); got != tt.want {
				t.Errorf("cleanDSN(sqlite) = %q, want %q", got, tt.want)
			}
		})
	}
}
