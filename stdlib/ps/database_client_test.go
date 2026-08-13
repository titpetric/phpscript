package ps

import (
	"context"
	"testing"
	"time"

	"github.com/titpetric/pdo/client"

	"github.com/titpetric/phpscript/model"
)

func TestDatabaseClientObserverRecordsQuerySpan(t *testing.T) {
	request := &model.Request{}
	ctx := model.WithSpanLine(model.WithSpanFilename(model.WithRequest(context.Background(), request), "queries/users.php"), 23)
	started := time.Now().Add(-time.Second)
	duration := 25 * time.Millisecond
	databaseClient := &DatabaseClient{ID: "db"}
	ctx = databaseClient.withSpan(ctx, "Get")
	if len(request.Spans) != 1 || request.Spans[0].Message != "db.Get" {
		t.Fatalf("initial database span = %+v", request.Spans)
	}

	databaseClient.observe(ctx, client.QueryLogEntry{
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

func TestDatabaseClientValueConvertsRowsToPHPArrays(t *testing.T) {
	value := databaseClientValue([]map[string]any{{
		"id":   int64(1),
		"name": "catalogue",
	}})

	rows, ok := value.(*model.Array)
	if !ok || rows.Len() != 1 {
		t.Fatalf("rows = %#v", value)
	}
	value, ok = rows.Get(int64(0))
	if !ok {
		t.Fatal("first row is missing")
	}
	row, ok := value.(*model.Array)
	if !ok {
		t.Fatalf("row = %#v", value)
	}
	if name, _ := row.Get("name"); name != "catalogue" {
		t.Fatalf("name = %#v", name)
	}

	row.Set("columns", int64(6))
	if columns, _ := row.Get("columns"); columns != int64(6) {
		t.Fatalf("columns = %#v", columns)
	}
}
