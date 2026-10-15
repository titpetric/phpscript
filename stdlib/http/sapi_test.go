package http_test

import (
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// runRequest runs src against a request a host assembled by hand, and returns
// the request together with what the script echoed.
//
// The order is the one every host uses: the standard library is registered
// once, and the request is seeded afterwards. These functions resolve the
// request at call time, so registration cannot have captured it.
func runRequest(t *testing.T, request runner.Context, src string) string {
	t.Helper()
	return runRequestSAPI(t, request, "", src)
}

// runRequestSAPI is runRequest under a named SAPI, for the functions that
// answer differently on a web request than they do on the command line.
func runRequestSAPI(t *testing.T, request runner.Context, sapi, src string) string {
	t.Helper()

	var out strings.Builder
	rt := runner.New(&out, runner.Options{SAPI: sapi})
	stdlib.Register(rt)
	request.Register(rt)

	program, err := rt.Load(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v (output %q)", err, out.String())
	}
	return out.String()
}

// TestGetAllHeadersReadsTheRequest covers the three spellings of one function.
// Ported code reaches for getallheaders, get_all_headers or
// apache_request_headers depending on its target server, so
// all three answer the same array.
func TestGetAllHeadersReadsTheRequest(t *testing.T) {
	t.Parallel()
	request := runner.NewContext()
	request.Headers = map[string]string{"X-Token": "secret", "Accept": "application/json"}

	for _, name := range []string{"getallheaders", "get_all_headers", "apache_request_headers"} {
		t.Run(name, func(t *testing.T) {
			out := runRequest(t, request, `<?php
$headers = `+name+`();
echo count($headers) . " " . $headers["X-Token"] . " " . $headers["Accept"];
`)
			if want := "2 secret application/json"; out != want {
				t.Errorf("echoed %q, want %q", out, want)
			}
		})
	}
}

// TestRequestFunctionsAreInertWithoutARequest covers the cli SAPI and a fixture
// that seeds no request. PHP's own cli SAPI accepts header() and does nothing
// with it, so a script written for the web runs on the command line without
// being rewritten.
func TestRequestFunctionsAreInertWithoutARequest(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)

	program, err := rt.Load(`<?php
echo count(getallheaders());
header("Content-Type: text/plain");
var_export(http_response_code());
var_export(setcookie("session", "abc"));
var_export(setrawcookie("session", "abc"));
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v (output %q)", err, out.String())
	}
	if want := "0falsefalsefalse"; out.String() != want {
		t.Errorf("echoed %q, want %q", out.String(), want)
	}
}

// TestHeaderStagesTheResponse covers the two shapes header() takes. A
// "Name: value" pair stages a header, and a status line such as
// "HTTP/1.0 404 Not Found" carries a status with no name and no value, so it
// stages the status it names.
func TestHeaderStagesTheResponse(t *testing.T) {
	t.Parallel()
	request := runner.NewContext()
	runRequest(t, request, `<?php
header("Content-Type: application/json");
header("X-Count: 1");
header("X-Count: 2");
header("X-Trace: a", false);
header("X-Trace: b", false);
header("HTTP/1.1 418 I am a teapot");
`)

	response := request.ResponseHeaders()
	if got := response.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	// $replace defaults to true, so the second X-Count wins.
	if got := response.Values("X-Count"); len(got) != 1 || got[0] != "2" {
		t.Errorf("X-Count = %v, want [2]", got)
	}
	// $replace false appends, so Set-Cookie is sent twice.
	if got := response.Values("X-Trace"); len(got) != 2 {
		t.Errorf("X-Trace = %v, want two values", got)
	}
	if got := request.ResponseStatus(); got != nethttp.StatusTeapot {
		t.Errorf("status = %d, want %d", got, nethttp.StatusTeapot)
	}
}

// TestHTTPResponseCode covers PHP's two answers. Called with a status it stages
// it and returns the one it replaced; called with none it reports the status
// the response will be sent with, so a template can branch on it.
//
// On a web SAPI a request starts out answering 200, so there is always a
// previous number to return.
func TestHTTPResponseCode(t *testing.T) {
	t.Parallel()
	request := runner.NewContext()
	out := runRequestSAPI(t, request, "apache", `<?php
echo http_response_code();
echo " " . http_response_code(404);
echo " " . http_response_code();
`)
	if want := "200 200 404"; out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
	if got := request.ResponseStatus(); got != nethttp.StatusNotFound {
		t.Errorf("status = %d, want %d", got, nethttp.StatusNotFound)
	}
}

// TestHTTPResponseCodeOffTheWeb covers the two answers php reserves for a run
// that has chosen no status: false while none is set, and true for the first
// one, there being no previous number to report. The command line is where
// that happens, and so is a Go host that named no SAPI.
func TestHTTPResponseCodeOffTheWeb(t *testing.T) {
	t.Parallel()
	request := runner.NewContext()
	out := runRequest(t, request, `<?php
var_dump(http_response_code());
var_dump(http_response_code(404));
var_dump(http_response_code());
`)
	want := "bool(false)\nbool(true)\nint(404)\n"
	if out != want {
		t.Errorf("printed %q, want %q", out, want)
	}
}

// TestSetCookie covers the header both the positional and the array call stage.
// The value is url-encoded as PHP's setcookie() encodes it, so a space is a
// plus, and an expiry of 0 carries no Expires attribute: the cookie dies with
// the browser session.
func TestSetCookie(t *testing.T) {
	t.Parallel()
	expires := time.Unix(1700000000, 0).UTC().Format(nethttp.TimeFormat)

	for _, test := range []struct {
		name string
		call string
		want []string
	}{
		{
			name: "name and value",
			call: `setcookie("session", "a b")`,
			want: []string{"session=a+b"},
		},
		{
			name: "session cookie",
			call: `setcookie("session", "abc", 0)`,
			want: []string{"session=abc"},
		},
		{
			name: "positional options",
			call: `setcookie("session", "abc", 1700000000, "/admin", "example.test", true, true)`,
			want: []string{"session=abc", "Path=/admin", "Domain=example.test", "Secure", "HttpOnly", "Expires=" + expires},
		},
		{
			name: "array options",
			call: `setcookie("session", "abc", array("path" => "/", "httponly" => true, "samesite" => "Strict"))`,
			want: []string{"session=abc", "Path=/", "HttpOnly", "SameSite=Strict"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := runner.NewContext()
			out := runRequest(t, request, `<?php var_export(`+test.call+`);`)
			if out != "true" {
				t.Errorf("%s echoed %q, want true", test.call, out)
			}

			staged := request.ResponseHeaders().Values("Set-Cookie")
			if len(staged) != 1 {
				t.Fatalf("Set-Cookie = %v, want one value", staged)
			}
			for _, want := range test.want {
				if !strings.Contains(staged[0], want) {
					t.Errorf("Set-Cookie = %q, want it to carry %q", staged[0], want)
				}
			}
		})
	}

	// An expiry of 0 carries no Expires attribute at all.
	request := runner.NewContext()
	runRequest(t, request, `<?php setcookie("session", "abc", 0);`)
	if staged := request.ResponseHeaders().Get("Set-Cookie"); strings.Contains(staged, "Expires") {
		t.Errorf("Set-Cookie = %q, want no Expires for a session cookie", staged)
	}
}

// TestSetRawCookie covers the spelling that writes the value as it stands. A
// value carrying a space or a semicolon is the caller's to handle, so the
// encoder does nothing with it and a script holding a pre-encoded value keeps
// it byte for byte.
func TestSetRawCookie(t *testing.T) {
	t.Parallel()
	request := runner.NewContext()
	out := runRequest(t, request, `<?php var_export(setrawcookie("session", "a%20b"));`)
	if out != "true" {
		t.Errorf("echoed %q, want true", out)
	}

	staged := request.ResponseHeaders().Get("Set-Cookie")
	if !strings.Contains(staged, "session=a%20b") {
		t.Errorf("Set-Cookie = %q, want the value unencoded", staged)
	}

	// setcookie would have written the percent itself as %25.
	encoded := runner.NewContext()
	runRequest(t, encoded, `<?php setcookie("session", "a%20b");`)
	if got := encoded.ResponseHeaders().Get("Set-Cookie"); !strings.Contains(got, "a%2520b") {
		t.Errorf("setcookie staged %q, want the value url-encoded", got)
	}
}

// TestSetCookieStagesSeveralCookies covers the header that arrives more than
// once. Each call appends, so a login response that sets a session and a
// preference sends two Set-Cookie headers and never one that replaced the
// other.
func TestSetCookieStagesSeveralCookies(t *testing.T) {
	t.Parallel()
	request := runner.NewContext()
	runRequest(t, request, `<?php
setcookie("session", "abc");
setcookie("theme", "dark");
`)

	staged := request.ResponseHeaders().Values("Set-Cookie")
	if len(staged) != 2 {
		t.Fatalf("Set-Cookie = %v, want two values", staged)
	}
	if !strings.Contains(staged[0], "session=abc") || !strings.Contains(staged[1], "theme=dark") {
		t.Errorf("Set-Cookie = %v, want the two cookies in the order they were staged", staged)
	}
}
