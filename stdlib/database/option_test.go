package database

import "testing"

func TestSQLiteDatabaseOption(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		maxOpen int
		maxIdle int
	}{
		{name: "file database", dsn: "app.db", maxOpen: 10, maxIdle: 2},
		{name: "memory database", dsn: ":memory:", maxOpen: 1, maxIdle: 1},
		{name: "memory URI", dsn: "file::memory:?cache=shared", maxOpen: 1, maxIdle: 1},
		{name: "named shared memory database", dsn: "file:app?mode=memory&cache=shared", maxOpen: 1, maxIdle: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			option := databaseOption("sqlite", tt.dsn)
			if option.MaxOpenConns != tt.maxOpen {
				t.Errorf("MaxOpenConns = %d, want %d", option.MaxOpenConns, tt.maxOpen)
			}
			if option.MaxIdleConns != tt.maxIdle {
				t.Errorf("MaxIdleConns = %d, want %d", option.MaxIdleConns, tt.maxIdle)
			}
		})
	}
}
