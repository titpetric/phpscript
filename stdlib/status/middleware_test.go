package status

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/runner"
)

func TestServerStatusRecordsRequestAndRuntime(t *testing.T) {
	status := NewServerStatus()
	handler := status.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := RequestID(r.Context()); id == "" {
			t.Fatal("request id missing from context")
		}
		rt := runner.New(w, runner.Options{})
		rt.SetContext(r.Context())
		rt.Observe(status)
		rt.UpdateFilename("routes/hello.php")
		rt.UpdateIncludedFiles(3)
		program, err := rt.Load(`<?php echo "hello"; ?>`)
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.Run(program); err != nil {
			t.Fatal(err)
		}
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/hello?q=1", nil)
	request.Host = "example.test"
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "hello" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	id := recorder.Header().Get("Request-Id")
	if !regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`).MatchString(id) {
		t.Fatalf("invalid ULID %q", id)
	}

	snapshot := status.Snapshot()
	if snapshot.Total != 1 || snapshot.Active != 0 || len(snapshot.Requests) != 0 {
		t.Fatalf("unexpected snapshot: total=%d active=%d requests=%d", snapshot.Total, snapshot.Active, len(snapshot.Requests))
	}
	if len(snapshot.Statistics.Top) != 1 {
		t.Fatalf("statistics = %+v", snapshot.Statistics)
	}
	stat := snapshot.Statistics.Top[0]
	if stat.Request != "GET /hello?q=1" || stat.Hostname != "example.test" || stat.Filename != "routes/hello.php" || stat.AverageIncludedFiles != 3 || stat.Count != 1 || stat.Share != 100 || stat.AverageResponseBytes != 5 {
		t.Fatalf("unexpected statistic: %+v", stat)
	}
	if len(snapshot.Log) != 1 || snapshot.Log[0].ID != id || snapshot.Log[0].Hostname != "example.test" || snapshot.Log[0].Filename != "routes/hello.php" || snapshot.Log[0].IncludedFiles != 3 {
		t.Fatalf("unexpected log: %+v", snapshot.Log)
	}
	spans := snapshot.Log[0].Spans
	if len(spans) != 3 || !spans[0].Open || spans[0].Message != "/hello" || spans[1].Message != "routes/hello.php" || spans[1].Type != SpanType.Internal || !spans[2].Close || spans[2].Message != "routes/hello.php" {
		t.Fatalf("implicit request spans = %+v", spans)
	}
	var stateShare float64
	for _, state := range snapshot.StateTime {
		if state.Duration <= 0 {
			t.Fatalf("non-positive state duration: %+v", state)
		}
		stateShare += state.Share
	}
	if len(snapshot.StateTime) < 3 || stateShare < 99.99 || stateShare > 100.01 {
		t.Fatalf("unexpected lifetime state time: %+v", snapshot.StateTime)
	}
}

func TestServerStatusRepresentations(t *testing.T) {
	status := NewServerStatus()
	recorded := httptest.NewRecorder()
	status.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("body"))
	})).ServeHTTP(recorded, httptest.NewRequest(http.MethodGet, "/recorded", nil))
	tests := []struct {
		name, path, accept, userAgent, contentType, contains, excludes string
	}{
		{"json live", ServerStatusLivePath, "text/json", "", "text/json", `"total_requests":1`, ""},
		{"plain overview", ServerStatusPath, "text/plain", "", "text/plain", "REQUEST-ID", "Top requests"},
		{"curl log", ServerStatusLogPath, "*/*", "curl/8.0", "text/plain", "Request log", "Top requests"},
		{"html overview", ServerStatusPath, "text/html", "Mozilla/5.0", "text/html", ServerStatusDetailPath, "No completed requests"},
		{"html live", ServerStatusLivePath, "text/html", "Mozilla/5.0", "text/html", "Lifetime request state time", "No completed requests"},
		{"html log", ServerStatusLogPath, "text/html", "Mozilla/5.0", "text/html", "GET /recorded", "No requests in flight"},
		{"html stats", ServerStatusStatsPath, "text/html", "Mozilla/5.0", "text/html", "Top 20 requests", "No requests in flight"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Accept", tt.accept)
			req.Header.Set("User-Agent", tt.userAgent)
			recorder := httptest.NewRecorder()
			status.ServeHTTP(recorder, req)
			if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.contentType) {
				t.Fatalf("content type = %q", got)
			}
			if !strings.Contains(recorder.Body.String(), tt.contains) {
				t.Fatalf("body %q does not contain %q", recorder.Body.String(), tt.contains)
			}
			if tt.excludes != "" && strings.Contains(recorder.Body.String(), tt.excludes) {
				t.Fatalf("body %q unexpectedly contains %q", recorder.Body.String(), tt.excludes)
			}
			if tt.name == "json live" {
				var snapshot Snapshot
				if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestMiddlewareRecordsStatusRequest(t *testing.T) {
	status := NewServerStatus()
	handler := status.Middleware(http.NotFoundHandler())
	for _, path := range []string{ServerStatusPath, ServerStatusLivePath, ServerStatusLogPath, ServerStatusStatsPath} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("%s returned status %d", path, recorder.Code)
		}
		if id := recorder.Header().Get("Request-Id"); id == "" {
			t.Errorf("%s did not receive request id", path)
		}
	}
	if snapshot := status.Snapshot(); snapshot.Total != 4 || len(snapshot.Requests) != 0 || snapshot.Statistics.WindowSize != 4 {
		t.Fatalf("status request was not recorded: %+v", snapshot)
	}
}

func TestRequestStatisticsRollingWindow(t *testing.T) {
	status := NewServerStatus()
	handler := status.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	for i := 0; i < historyLimit+5; i++ {
		path := "/less-common"
		if i%2 == 0 {
			path = "/common"
		}
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	snapshot := status.Snapshot()
	if len(snapshot.Requests) != 0 || snapshot.Statistics.WindowSize != historyLimit {
		t.Fatalf("unexpected process/statistics sizes: %d, %d", len(snapshot.Requests), snapshot.Statistics.WindowSize)
	}
	if len(snapshot.Statistics.Top) != 2 {
		t.Fatalf("top statistics = %+v", snapshot.Statistics.Top)
	}
	if len(snapshot.Log) != historyLimit || snapshot.Log[0].Request != "GET /common" {
		t.Fatalf("unexpected log: len=%d first=%+v", len(snapshot.Log), snapshot.Log[0])
	}
	for _, got := range snapshot.Statistics.Top {
		if got.Count != 50 || got.Share != 50 {
			t.Fatalf("statistic = %+v", got)
		}
	}
}

func TestRequestSpansAndDetail(t *testing.T) {
	status := NewServerStatus()
	handler := status.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Span(r.Context(), "getUser", Flag("database"))
		Span(r.Context(), "render", SpanType.Template, OpenSpan)
		Span(r.Context(), "partial")
		Span(r.Context(), "render", SpanType.Template, CloseSpan)
		_, _ = w.Write([]byte("ok"))
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/users/1", nil))
	id := recorder.Header().Get("Request-Id")

	request := status.Snapshot().Log[0]
	if len(request.Spans) != 6 {
		t.Fatalf("spans = %+v", request.Spans)
	}
	for i, span := range request.Spans {
		if span.ID != i+1 {
			t.Fatalf("span %d ID = %d", i, span.ID)
		}
	}
	if first, last := request.Spans[0], request.Spans[len(request.Spans)-1]; first.Type != SpanType.HTTP || !first.Open || last.Type != SpanType.HTTP || !last.Close {
		t.Fatalf("HTTP boundary spans = %+v / %+v", first, last)
	}
	if request.Spans[1].Type != Flag("database") || request.Spans[1].Message != "getUser" {
		t.Fatalf("database span = %+v", request.Spans[1])
	}
	if request.Spans[3].Type != SpanType.Internal {
		t.Fatalf("default span type = %q", request.Spans[3].Type)
	}

	detail := httptest.NewRecorder()
	detailRequest := httptest.NewRequest(http.MethodGet, ServerStatusDetailPath+id, nil)
	detailRequest.Header.Set("Accept", "text/html")
	status.ServeHTTP(detail, detailRequest)
	for _, text := range []string{"<tr><th>Type</th><th>Time</th><th>Duration</th><th>Message</th></tr>", "database", "getUser", "span-type", "span-bullet", "margin-left:1.5em", "margin-left:3.0em", "border-left:4px solid #2563eb"} {
		if !strings.Contains(detail.Body.String(), text) {
			t.Fatalf("detail body does not contain %q: %s", text, detail.Body.String())
		}
	}

	log := httptest.NewRecorder()
	status.ServeHTTP(log, httptest.NewRequest(http.MethodGet, ServerStatusLogPath, nil))
	if !strings.Contains(log.Body.String(), ServerStatusDetailPath+id) {
		t.Fatalf("log does not link detail: %s", log.Body.String())
	}
}

func TestULIDContainsTimestamp(t *testing.T) {
	id, err := newULID(time.UnixMilli(1_700_000_000_123))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "01HF7YAT") {
		t.Fatalf("ULID %q has wrong timestamp", id)
	}
}
