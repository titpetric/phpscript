package http_test

// Sending is tested here, against httptest, rather than in a .phpt fixture: a
// fixture that reached the network would be a fixture that fails when the
// network does. The fixture covers construction and introspection, which is
// what a script can check without a server.

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/titpetric/phpscript/stdlib/http"
)

func TestClientSendsRequest(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotHeader string
		gotBody   string
	)
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotHeader = r.Header.Get("X-Token")
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		gotBody = string(body)

		w.Header().Set("X-Served-By", "httptest")
		w.WriteHeader(nethttp.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := http.NewClient(ctx, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	request, err := http.NewRequest(ctx, "POST", server.URL+"/users", `{"name":"ada"}`)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("X-Token", "secret")

	response, err := client.Send(ctx, request)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/users" {
		t.Errorf("path = %q, want /users", gotPath)
	}
	if gotHeader != "secret" {
		t.Errorf("X-Token = %q, want secret", gotHeader)
	}
	if gotBody != `{"name":"ada"}` {
		t.Errorf("body = %q, want the request body", gotBody)
	}

	if response.Status() != 201 {
		t.Errorf("Status() = %d, want 201", response.Status())
	}
	if !response.OK() {
		t.Error("OK() = false for 201, want true")
	}
	if response.Err() != "" {
		t.Errorf("Err() = %q, want empty for a request that succeeded", response.Err())
	}
	if got := response.Header("x-served-by"); got != "httptest" {
		t.Errorf("Header() = %q, want httptest (header lookup is case-insensitive)", got)
	}
	decoded, err := response.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if m, ok := decoded.(map[string]any); !ok || m["ok"] != true {
		t.Errorf("JSON() = %#v, want {ok: true}", decoded)
	}
}

// TestRequestIsANetHTTPRequest pins the shape a script gets back. The binding
// hands over the net/http value rather than a facade, so a script reads and
// writes it the way Go does.
func TestRequestIsANetHTTPRequest(t *testing.T) {
	ctx := context.Background()
	request, err := http.NewRequest(ctx, "GET", "https://example.invalid/users?page=1")
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	var _ *nethttp.Request = request

	if request.Method != "GET" {
		t.Errorf("Method = %q, want GET", request.Method)
	}
	if request.URL.Path != "/users" {
		t.Errorf("URL.Path = %q, want /users", request.URL.Path)
	}
	if request.URL.Query().Get("page") != "1" {
		t.Errorf("query page = %q, want 1", request.URL.Query().Get("page"))
	}
	if request.Host != "example.invalid" {
		t.Errorf("Host = %q, want example.invalid", request.Host)
	}

	request.Header.Set("Accept", "application/json")
	if request.Header.Get("accept") != "application/json" {
		t.Errorf("Header.Get = %q, want application/json", request.Header.Get("accept"))
	}
}

// TestRequestMethodIsUpperCased pins that a lowercase verb is corrected rather
// than sent as written: a server treats the method as case-sensitive and would
// reject it.
func TestRequestMethodIsUpperCased(t *testing.T) {
	request, err := http.NewRequest(context.Background(), "post", "https://example.invalid")
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if request.Method != "POST" {
		t.Errorf("Method = %q, want POST", request.Method)
	}
}

func TestClientOptions(t *testing.T) {
	var gotAgent, gotAuth string
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		gotAgent = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := http.NewClient(ctx, map[string]any{
		"timeout":    int64(5),
		"base_url":   server.URL,
		"user_agent": "phpscript/1",
		"headers":    map[string]any{"Authorization": "Bearer token"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// A relative URL resolves against base_url, which is the point of the option.
	request, err := http.NewRequest(ctx, "GET", "/things")
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response, err := client.Send(ctx, request)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if response.Body() != "ok" {
		t.Errorf("Body() = %q, want ok", response.Body())
	}
	if gotAgent != "phpscript/1" {
		t.Errorf("User-Agent = %q, want phpscript/1", gotAgent)
	}
	if gotAuth != "Bearer token" {
		t.Errorf("Authorization = %q, want the client header", gotAuth)
	}

	// Send clones, so the caller's request is unchanged and can be sent again.
	if request.URL.IsAbs() {
		t.Errorf("Send mutated the caller's request URL to %q", request.URL)
	}
}

// TestClientRelativeURLWithoutBaseIsReported pins that a relative URL with no
// base_url is a named error rather than a transport failure.
func TestClientRelativeURLWithoutBaseIsReported(t *testing.T) {
	ctx := context.Background()
	client, err := http.NewClient(ctx, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	request, err := http.NewRequest(ctx, "GET", "/things")
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if _, err := client.Send(ctx, request); err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Errorf("Send error = %v, want one naming base_url", err)
	}
}

// TestClientRejectsUnknownOption pins that a typo is reported rather than
// silently doing nothing, which is the SMTP binding's rule too.
func TestClientRejectsUnknownOption(t *testing.T) {
	_, err := http.NewClient(context.Background(), map[string]any{"timeuot": int64(5)})
	if err == nil {
		t.Fatal("NewClient accepted an unknown option")
	}
	if !strings.Contains(err.Error(), "timeuot") {
		t.Errorf("error %q does not name the unknown option", err)
	}
}

// TestClientDoesNotFollowRedirects pins follow_redirects, which a script sets
// when it wants to see the Location rather than the destination.
func TestClientDoesNotFollowRedirects(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.URL.Path == "/from" {
			nethttp.Redirect(w, r, "/to", nethttp.StatusFound)
			return
		}
		w.Write([]byte("arrived"))
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := http.NewClient(ctx, map[string]any{"follow_redirects": false})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.Get(ctx, server.URL+"/from")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if response.Status() != 302 {
		t.Errorf("Status() = %d, want 302", response.Status())
	}
	if got := response.Header("Location"); got != "/to" {
		t.Errorf("Location = %q, want /to", got)
	}
}

// TestClientParallel is the batch case: every request runs at once, and the
// results come back under the keys the caller chose.
func TestClientParallel(t *testing.T) {
	var inFlight, peak atomic.Int64
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		current := inFlight.Add(1)
		for {
			seen := peak.Load()
			if current <= seen || peak.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)
		w.Write([]byte(r.URL.Path))
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := http.NewClient(ctx, map[string]any{"base_url": server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	requests := map[string]*nethttp.Request{}
	for _, name := range []string{"users", "stats", "flags"} {
		request, err := http.NewRequest(ctx, "GET", "/"+name)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		requests[name] = request
	}

	start := time.Now()
	results, err := client.Parallel(ctx, requests)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Parallel: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Parallel returned %d results, want 3", len(results))
	}
	for name, response := range results {
		if !response.OK() {
			t.Errorf("%s: OK() = false, err %q", name, response.Err())
		}
		if response.Body() != "/"+name {
			t.Errorf("%s: Body() = %q, want /%s", name, response.Body(), name)
		}
	}

	// Concurrent, not sequential: three 30ms requests one after another take
	// 90ms, and the handler records how many were in flight at once.
	if got := peak.Load(); got < 2 {
		t.Errorf("peak concurrent requests = %d, want at least 2", got)
	}
	if elapsed > 80*time.Millisecond {
		t.Errorf("Parallel took %v for three 30ms requests, want them overlapped", elapsed)
	}
}

// TestClientParallelReportsFailurePerRequest pins that one unreachable host
// does not hide the results of the rest of the batch.
func TestClientParallelReportsFailurePerRequest(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Write([]byte("fine"))
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := http.NewClient(ctx, map[string]any{"timeout": "200ms"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	good, err := http.NewRequest(ctx, "GET", server.URL)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// Port 1 on the loopback address refuses immediately.
	bad, err := http.NewRequest(ctx, "GET", "http://127.0.0.1:1/nope")
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	results, err := client.Parallel(ctx, map[string]*nethttp.Request{"good": good, "bad": bad})
	if err != nil {
		t.Fatalf("Parallel returned an error for a per-request failure: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Parallel returned %d results, want 2", len(results))
	}
	if !results["good"].OK() {
		t.Errorf("good: OK() = false, err %q", results["good"].Err())
	}
	if results["bad"].OK() {
		t.Error("bad: OK() = true for a refused connection")
	}
	if results["bad"].Err() == "" {
		t.Error("bad: Err() is empty for a refused connection")
	}
	if results["bad"].Status() != 0 {
		t.Errorf("bad: Status() = %d, want 0 for a request that got no response", results["bad"].Status())
	}
}

// TestClientParallelRejectsBadInput pins that the argument has to be an array
// of requests, which is the one thing Parallel throws for.
func TestClientParallelRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	client, err := http.NewClient(ctx, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	for _, input := range []any{nil, "not an array", map[string]any{"a": "not a request"}} {
		if _, err := client.Parallel(ctx, input); err == nil {
			t.Errorf("Parallel accepted %#v", input)
		}
	}
}

// TestRequestRequiresMethodAndURL pins the two arguments that have no useful
// default.
func TestRequestRequiresMethodAndURL(t *testing.T) {
	ctx := context.Background()
	if _, err := http.NewRequest(ctx, "", "https://example.invalid"); err == nil {
		t.Error("NewRequest accepted an empty method")
	}
	if _, err := http.NewRequest(ctx, "GET", ""); err == nil {
		t.Error("NewRequest accepted an empty url")
	}
}

// TestResponseJSONReportsBadBody pins that a non-JSON body throws rather than
// returning a zero value a script would then use.
func TestResponseJSONReportsBadBody(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := http.NewClient(ctx, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := response.JSON(); err == nil {
		t.Error("JSON() accepted a body that is not JSON")
	}
}

// TestClientTimesOut pins that a client always has a deadline, which is the one
// failure a script cannot recover from on its own.
func TestClientTimesOut(t *testing.T) {
	block := make(chan struct{})
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		<-block
	}))
	defer func() { close(block); server.Close() }()

	ctx := context.Background()
	client, err := http.NewClient(ctx, map[string]any{"timeout": "50ms"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.Get(ctx, server.URL); err == nil {
		t.Error("Get returned before the timeout elapsed")
	}
}

// TestResponseJSONMatchesEncodingJSON checks the decoded shape against what
// encoding/json produces for the same body.
func TestResponseJSONMatchesEncodingJSON(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Write([]byte(`{"a":[1,2],"b":"c"}`))
	}))
	defer server.Close()

	ctx := context.Background()
	client, _ := http.NewClient(ctx, nil)
	response, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := response.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var want any
	if err := json.Unmarshal([]byte(response.Body()), &want); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if gotJSON, _ := json.Marshal(got); string(gotJSON) != `{"a":[1,2],"b":"c"}` {
		t.Errorf("JSON() = %s, want the body decoded", gotJSON)
	}
}
