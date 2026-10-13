package database

import (
	"testing"
)

// TestDatabaseOption covers the pool sizes a provider opens each driver with.
// sqlite keeps two idle connections out of ten and mysql keeps ten out of ten,
// because a mysql handshake costs a round trip and reopening one per request
// reads as latency, while an idle sqlite handle holds a file open for nothing.
// A driver the table does not name is the zero value, which leaves the pool
// unbounded and keeps none of it idle.
func TestDatabaseOption(t *testing.T) {
	for _, test := range []struct {
		driver string
		want   DatabaseOption
	}{
		{driver: "sqlite", want: DatabaseOption{MaxOpenConns: 10, MaxIdleConns: 2}},
		{driver: "mysql", want: DatabaseOption{MaxOpenConns: 10, MaxIdleConns: 10}},
		{driver: "pgx", want: DatabaseOption{}},
	} {
		t.Run(test.driver, func(t *testing.T) {
			if got := databaseOption(test.driver, "app.db"); got != test.want {
				t.Errorf("databaseOption(%q) = %+v, want %+v", test.driver, got, test.want)
			}
		})
	}
}

// TestDatabaseOption_Apply covers writing the sizes onto a pool, and the nil
// receiver a caller holding no option passes. A nil option leaves the pool as
// the sql package built it, so a provider with no entry for a driver still
// opens one.
func TestDatabaseOption_Apply(t *testing.T) {
	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	option := &DatabaseOption{MaxOpenConns: 4, MaxIdleConns: 2}
	option.Apply(db)
	if got := db.Stats().MaxOpenConnections; got != 4 {
		t.Errorf("MaxOpenConnections = %d, want 4", got)
	}

	var none *DatabaseOption
	none.Apply(db)
	if got := db.Stats().MaxOpenConnections; got != 4 {
		t.Errorf("a nil option rewrote MaxOpenConnections to %d, want 4", got)
	}
}

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
