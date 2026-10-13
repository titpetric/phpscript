package database

import (
	"testing"
)

// Classification reads the leading text of a statement, so what a query is
// tagged with must not change what it is. The tag is the reason to look: a
// statement in `show processlist` says nothing about the code that issued it.
func TestParseQuery(t *testing.T) {
	for _, test := range []struct {
		name    string
		query   string
		want    queryInfo
		notRead bool
	}{{
		name:  "plain select",
		query: "select * from users",
		want:  queryInfo{Type: "select"},
	}, {
		name:  "tagged select",
		query: "/* userGet */ SELECT * FROM user WHERE id = ?",
		want:  queryInfo{Type: "select", Comment: "userGet"},
	}, {
		name:  "tagged write",
		query: "/* userSave */ INSERT INTO user (name) VALUES (?)",
		want:  queryInfo{Type: "insert", Comment: "userSave"},

		notRead: true,
	}, {
		name:  "leading whitespace and newlines",
		query: "\n\t  select\n\tid\nfrom users",
		want:  queryInfo{Type: "select"},
	}, {
		name:  "first of several comments",
		query: "/* userGet */ /* cached */ select 1",
		want:  queryInfo{Type: "select", Comment: "userGet"},
	}, {
		name:  "empty comment",
		query: "/**/select 1",
		want:  queryInfo{Type: "select"},
	}, {
		name:  "line comments",
		query: "-- the user by name\n# mysql spells it this way\nselect 1",
		want:  queryInfo{Type: "select"},
	}, {
		name:  "comment carried past a line comment",
		query: "/* userGet */\n-- by name\nselect 1",
		want:  queryInfo{Type: "select", Comment: "userGet"},
	}, {
		name:  "show",
		query: "SHOW DATABASES",
		want:  queryInfo{Type: "show"},
	}, {
		name:  "describe",
		query: "describe users",
		want:  queryInfo{Type: "describe"},
	}, {
		name:  "desc",
		query: "DESC users",
		want:  queryInfo{Type: "desc"},
	}, {
		name:    "ddl",
		query:   "DROP TABLE users",
		want:    queryInfo{Type: "drop"},
		notRead: true,
	}, {
		name:    "pragma writes",
		query:   "PRAGMA journal_mode = WAL",
		want:    queryInfo{Type: "pragma"},
		notRead: true,
	}, {
		// EXPLAIN ANALYZE runs the statement it reports on in postgres, so an
		// explain is not reliably a read.
		name:    "explain",
		query:   "explain analyze insert into users (name) values ('Ada')",
		want:    queryInfo{Type: "explain"},
		notRead: true,
	}, {
		name:    "empty",
		query:   "   ",
		notRead: true,
	}, {
		name:    "comment only",
		query:   "/* userGet */",
		want:    queryInfo{Comment: "userGet"},
		notRead: true,
	}, {
		name:    "unterminated comment",
		query:   "/* select * from users",
		notRead: true,
	}, {
		// A statement has to begin with its keyword to be classified, so a
		// parenthesized union is refused, and never guessed at.
		name:    "leading parenthesis",
		query:   "(select 1) union (select 2)",
		notRead: true,
	}, {
		name:  "single word",
		query: "SELECT",
		want:  queryInfo{Type: "select"},
	}} {
		t.Run(test.name, func(t *testing.T) {
			got := parseQuery(test.query)
			if got != test.want {
				t.Fatalf("parseQuery(%q) = %+v, want %+v", test.query, got, test.want)
			}
			if got.isRead() == test.notRead {
				t.Fatalf("isRead() = %t for %q", got.isRead(), test.query)
			}
		})
	}
}

// The refusal names the statement, so a script catching the exception can say
// which call it lost, and not that the database is read-only.
func TestQueryInfoRefusal(t *testing.T) {
	if got := (queryInfo{Type: "drop"}).refusal(); got != "drop is not allowed" {
		t.Fatalf("refusal = %q", got)
	}
	if got := (queryInfo{}).refusal(); got != "the statement does not start with a keyword" {
		t.Fatalf("refusal = %q", got)
	}
}

// TestDatabase_Get covers the two outcomes of a single-row read. A row arrives
// as a native map, so $row["name"] indexes it. A query matching no rows is
// false and no error, because PHP reads a missing row as a falsy value and a
// script tests it with if.
func TestDatabase_Get(t *testing.T) {
	database := newLiveDatabase(t)

	value, err := database.Get(t.Context(), "select name from users where id = ?", 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	row, ok := value.(map[string]any)
	if !ok || row["name"] != "Ada" {
		t.Fatalf("Get returned %#v, want a row holding Ada", value)
	}

	missing, err := database.Get(t.Context(), "select name from users where id = ?", 99)
	if err != nil {
		t.Fatalf("Get over no rows: %v", err)
	}
	if missing != false {
		t.Errorf("Get over no rows = %#v, want false", missing)
	}
}

// TestDatabase_GetAll covers the whole result set: one native map per row in a
// Go slice, which the value model walks as a PHP list, so foreach visits the
// rows in the order the server returned them.
func TestDatabase_GetAll(t *testing.T) {
	database := newLiveDatabase(t)

	value, err := database.GetAll(t.Context(), "select name from users order by id")
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	rows, ok := value.([]map[string]any)
	if !ok {
		t.Fatalf("GetAll returned %T, want a slice of rows", value)
	}
	if len(rows) != 2 || rows[0]["name"] != "Ada" || rows[1]["name"] != "Grace" {
		t.Fatalf("rows = %#v, want Ada then Grace", rows)
	}
}
