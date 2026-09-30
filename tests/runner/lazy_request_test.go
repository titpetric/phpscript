package runner_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestRequestDecodesOnlyWhatIsRead covers the promise the views make: building
// a Context reads nothing out of the request, and each input is decoded by the
// first read that needs it.
func TestRequestDecodesOnlyWhatIsRead(t *testing.T) {
	body := "name=bob&tags[]=a&tags[]=b"
	req := httptest.NewRequest("POST", "http://example.com/?q=search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})

	ctx := runner.FromRequest(req)

	// Nothing has been read, so nothing has been decoded.
	if len(ctx.Get) != 0 || len(ctx.Post) != 0 || len(ctx.Cookie) != 0 {
		t.Fatalf("building the context decoded something: get=%v post=%v cookie=%v",
			ctx.Get, ctx.Post, ctx.Cookie)
	}
	if len(ctx.RawBody()) == 0 {
		t.Fatal("php://input is empty")
	}

	// Each accessor decodes its own input and no other.
	fresh := runner.FromRequest(httptest.NewRequest("GET", "http://example.com/?q=search", nil))
	if got := fresh.GetMap()["q"]; got != "search" {
		t.Fatalf("$_GET[q] = %q, want search", got)
	}
	if len(fresh.Cookie) != 0 {
		t.Fatal("reading the query decoded the cookies too")
	}

	if got := ctx.PostMap()["name"]; got != "bob" {
		t.Fatalf("$_POST[name] = %q, want bob", got)
	}
	if got := ctx.CookieMap()["session"]; got != "abc" {
		t.Fatalf("$_COOKIE[session] = %q, want abc", got)
	}
}
