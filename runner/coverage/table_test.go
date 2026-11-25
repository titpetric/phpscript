package coverage

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestStmtTable covers the lookup the hot path runs: a statement answers its
// own entry, a statement that was never registered answers nothing, and a
// repeated put keeps the entry already there.
func TestStmtTable(t *testing.T) {
	t.Parallel()
	var table stmtTable
	a, b := &model.Echo{}, &model.Echo{}
	first := &coverStmt{file: "a.php"}
	table.put(a, first)

	if got := table.get(stmtKey(a)); got != first {
		t.Errorf("get(a) = %p, want the registered entry", got)
	}
	if got := table.get(stmtKey(b)); got != nil {
		t.Errorf("get(b) = %p, want nil for an unregistered statement", got)
	}
	table.put(a, &coverStmt{file: "other.php"})
	if got := table.get(stmtKey(a)); got != first {
		t.Errorf("get(a) after a second put = %p, want the first entry", got)
	}
}

// TestStmtTable_Grow covers the rehash an include forces: every entry survives
// a table that outgrew its first allocation.
func TestStmtTable_Grow(t *testing.T) {
	t.Parallel()
	var table stmtTable
	stmts := make([]*model.Echo, 1000)
	entries := make([]*coverStmt, len(stmts))
	for i := range stmts {
		stmts[i] = &model.Echo{}
		entries[i] = &coverStmt{file: "file.php"}
		table.put(stmts[i], entries[i])
	}
	if table.count != len(stmts) {
		t.Fatalf("count = %d, want %d", table.count, len(stmts))
	}
	for i, stmt := range stmts {
		if got := table.get(stmtKey(stmt)); got != entries[i] {
			t.Fatalf("get after growth = %p, want entry %d", got, i)
		}
	}
}

// TestStmtTable_Each covers the walk Blocks uses, which must visit every entry
// once and no empty slot.
func TestStmtTable_Each(t *testing.T) {
	t.Parallel()
	var table stmtTable
	for range 10 {
		table.put(&model.Echo{}, &coverStmt{file: "file.php"})
	}
	seen := 0
	table.each(func(*coverStmt) { seen++ })
	if seen != 10 {
		t.Errorf("each visited %d entries, want 10", seen)
	}
}
