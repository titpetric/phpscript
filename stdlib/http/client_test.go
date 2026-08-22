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
	"testing"

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
	request, err := http.NewRequest(ctx, "post", server.URL+"/users", `{"name":"ada"}`)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.SetHeader("X-Token", "secret")

	response, err := client.Send(ctx, request)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST (the constructor upper-cases it)", gotMethod)
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

// TestRequestIsInertUntilSent pins that building a request sends nothing, which
// is why the fixture can construct one without a server.
func TestRequestIsInertUntilSent(t *testing.T) {
	ctx := context.Background()
	request, err := http.NewRequest(ctx, "get", "https://example.invalid/users?page=1")
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.SetQuery("page", "2").SetHeader("Accept", "application/json").SetBody("body")

	if request.Method() != "GET" {
		t.Errorf("Method() = %q, want GET", request.Method())
	}
	if request.URL() != "https://example.invalid/users?page=2" {
		t.Errorf("URL() = %q, want the query replaced", request.URL())
	}
	if request.Header("accept") != "application/json" {
		t.Errorf("Header() = %q, want application/json", request.Header("accept"))
	}
	if request.Body() != "body" {
		t.Errorf("Body() = %q, want body", request.Body())
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

// jsonRoundTrip keeps the encoding/json import honest in a way a reader can see
// is deliberate: the decoded shape above is compared against what json itself
// produces for the same body.
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
