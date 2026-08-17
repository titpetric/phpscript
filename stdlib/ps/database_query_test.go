package ps

import "testing"

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
		// parenthesized union is refused rather than guessed at.
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
// which call it lost rather than that the database is read-only.
func TestQueryInfoRefusal(t *testing.T) {
	if got := (queryInfo{Type: "drop"}).refusal(); got != "drop is not allowed" {
		t.Fatalf("refusal = %q", got)
	}
	if got := (queryInfo{}).refusal(); got != "the statement does not start with a keyword" {
		t.Fatalf("refusal = %q", got)
	}
}
