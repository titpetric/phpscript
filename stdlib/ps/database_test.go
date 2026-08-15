package ps

import (
	"context"
	"testing"
	"time"

	"github.com/titpetric/pdo/client"

	"github.com/titpetric/phpscript/model"
)

func TestDatabaseObserverRecordsQuerySpan(t *testing.T) {
	request := &model.Request{}
	ctx := model.WithSpanLine(model.WithSpanFilename(model.WithRequest(context.Background(), request), "queries/users.php"), 23)
	started := time.Now().Add(-time.Second)
	duration := 25 * time.Millisecond
	database := &Database{ID: "db"}
	ctx = database.withSpan(ctx, "Get")
	if len(request.Spans) != 1 || request.Spans[0].Message != "db.Get" {
		t.Fatalf("initial database span = %+v", request.Spans)
	}

	database.observe(ctx, client.QueryLogEntry{
		Query:    "select * from users where id = ?",
		Started:  started,
		Duration: duration,
	})

	if len(request.Spans) != 1 {
		t.Fatalf("spans = %+v", request.Spans)
	}
	span := request.Spans[0]
	if span.Type != model.SpanType.Database || span.Filename != "queries/users.php" || span.Line != 23 || span.Message != "db.Get: <code>select * from users where id = ?</code>" || !span.Time.Equal(started) || span.Duration != duration {
		t.Fatalf("database span = %+v", span)
	}
}

// TestDatabaseRowsAreReadableAsPHPArrays pins the row representation Get and
// GetAll hand to the VM: the bridge's own []map[string]any, uncopied. The VM's
// value-model helpers must see it as an array-like value with the columns
// reachable by key, which is what foreach and $row["col"] rely on.
func TestDatabaseRowsAreReadableAsPHPArrays(t *testing.T) {
	rows := any([]map[string]any{{
		"id":   int64(1),
		"name": "catalogue",
	}})

	if !model.IsCollection(rows) {
		t.Fatalf("rows are not array-like: %#v", rows)
	}
	if n, _ := model.LenValues(rows); n != 1 {
		t.Fatalf("rows length = %d", n)
	}

	var row any
	model.RangeValues(rows, func(key, value any) bool {
		if key != int64(0) {
			t.Fatalf("row key = %#v", key)
		}
		row = value
		return false
	})

	columns := map[string]any{}
	model.RangeValues(row, func(key, value any) bool {
		columns[key.(string)] = value
		return true
	})
	if columns["name"] != "catalogue" || columns["id"] != int64(1) {
		t.Fatalf("columns = %#v", columns)
	}

	// A row is a Go map, so a script adding a column mutates the row the result
	// set holds — the same reference semantics the *model.Array copy had.
	row.(map[string]any)["columns"] = int64(6)
	if got := rows.([]map[string]any)[0]["columns"]; got != int64(6) {
		t.Fatalf("added column = %#v", got)
	}
}
