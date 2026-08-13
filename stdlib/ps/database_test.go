package ps

import (
	"context"
	"testing"

	"github.com/titpetric/phpscript/model"
)

func TestTraceDatabaseQueryMeasuresSingleSpan(t *testing.T) {
	request := &model.Request{}
	ctx := model.WithSpanFilename(model.WithRequest(context.Background(), request), "queries/users.php")
	done := traceDatabaseQuery(ctx, true, "select 1")
	if len(request.Spans) != 1 {
		t.Fatalf("spans before query = %+v", request.Spans)
	}
	done()
	span := request.Spans[0]
	if span.Type != model.SpanType.Database || span.Filename != "queries/users.php" || span.Message != "<code>select 1</code>" || span.Duration <= 0 {
		t.Fatalf("database span = %+v", span)
	}
}

func TestTraceDatabaseQueryCanBeDisabled(t *testing.T) {
	request := &model.Request{}
	done := traceDatabaseQuery(model.WithRequest(context.Background(), request), false, "select 1")
	done()
	if len(request.Spans) != 0 {
		t.Fatalf("disabled tracing recorded %+v", request.Spans)
	}
}
