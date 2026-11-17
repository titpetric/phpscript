package http

import (
	nethttp "net/http"
	"net/url"
	"strings"
	"time"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the request-aware functions to stdlib.Register.
func init() {
	runner.RegisterArea(`Runtime\PS\HTTP\SAPI`, RegisterRequest)
}

// RegisterRequest installs the request-aware functions, resolving the request at call time, inert without one.
func RegisterRequest(rt *runner.Runtime) {
	// getallheaders returns the request headers as an associative array keyed
	// by canonical header name, and an empty array when there is no request.
	rt.RegisterFunc("getallheaders", func() *model.Array {
		request, ok := runner.RequestContext(rt.Context())
		if !ok {
			return model.NewArray()
		}
		return request.GetAllHeaders()
	})
	// get_all_headers is an alias spelling of getallheaders.
	rt.RegisterFunc("get_all_headers", func() *model.Array {
		request, ok := runner.RequestContext(rt.Context())
		if !ok {
			return model.NewArray()
		}
		return request.GetAllHeaders()
	})
	// apache_request_headers is an alias of getallheaders.
	rt.RegisterFunc("apache_request_headers", func() *model.Array {
		request, ok := runner.RequestContext(rt.Context())
		if !ok {
			return model.NewArray()
		}
		return request.GetAllHeaders()
	})

	// header stages the "Name: value" response header in $header, written to
	// the response after the script finishes; $replace (default true)
	// overwrites an existing header of the same name, $code stages the
	// response status, and a status line such as "HTTP/1.0 404 Not Found"
	// stages the status it names.
	rt.RegisterFunc("header", func(header string, opts ...any) {
		request, ok := runner.RequestContext(rt.Context())
		if !ok {
			return
		}
		request.Header(header, opts...)
	})

	// http_response_code stages the response status in $response_code and
	// returns the one it replaced; called without one it returns the status
	// the response will send, or false when there is no request to answer for.
	rt.RegisterFunc("http_response_code", func(opts ...any) any {
		request, ok := runner.RequestContext(rt.Context())
		if !ok {
			return false
		}
		return request.HTTPResponseCode(rt.SAPI(), opts...)
	})

	// setcookie stages a Set-Cookie header naming $name with $value
	// url-encoded, and answers whether it could; $expires_or_options is a
	// unix timestamp, 0 for a cookie that dies with the browser session, or
	// an array of expires, path, domain, secure, httponly and samesite.
	rt.RegisterFunc("setcookie", func(name string, opts ...any) bool {
		return stageCookie(rt, name, opts, true)
	})

	// setrawcookie stages a Set-Cookie header the way setcookie does, writing
	// $value as it stands and a value with a semicolon or a space is the
	// caller's to handle; the encoder does nothing with it.
	rt.RegisterFunc("setrawcookie", func(name string, opts ...any) bool {
		return stageCookie(rt, name, opts, false)
	})
}

// stageCookie stages the Set-Cookie header both spellings build, in net/http's text form.
func stageCookie(rt *runner.Runtime, name string, opts []any, encode bool) bool {
	request, ok := runner.RequestContext(rt.Context())
	if !ok {
		return false
	}

	cookie := &nethttp.Cookie{Name: name}
	if len(opts) > 0 {
		value := phpval.String(opts[0])
		if encode {
			// PHP url-encodes with urlencode(), which spells a space as +.
			// QueryEscape is that same encoding.
			value = url.QueryEscape(value)
		}
		cookie.Value = value
	}

	if len(opts) > 1 {
		// The options form is a collection, whatever built it: the array a
		// literal writes and the map another binding returned both. Anything
		// else is the long argument list.
		if modelutil.IsCollection(opts[1]) {
			applyCookieOptions(cookie, opts[1])
		} else {
			applyCookiePositional(cookie, opts[1:])
		}
	}

	request.AddResponseHeader("Set-Cookie", cookie.String())
	return true
}

// applyCookiePositional reads $expires, $path, $domain, $secure and $httponly, with no samesite.
func applyCookiePositional(cookie *nethttp.Cookie, args []any) {
	for i, arg := range args {
		switch i {
		case 0:
			setCookieExpires(cookie, phpval.Int(arg))
		case 1:
			cookie.Path = phpval.String(arg)
		case 2:
			cookie.Domain = phpval.String(arg)
		case 3:
			cookie.Secure = phpval.Truthy(arg)
		case 4:
			cookie.HttpOnly = phpval.Truthy(arg)
		}
	}
}

// applyCookieOptions reads the array form, folding keys and leaving unnamed options zero.
func applyCookieOptions(cookie *nethttp.Cookie, options any) {
	modelutil.RangeValues(options, func(key, value any) bool {
		switch strings.ToLower(phpval.String(key)) {
		case "expires":
			setCookieExpires(cookie, phpval.Int(value))
		case "path":
			cookie.Path = phpval.String(value)
		case "domain":
			cookie.Domain = phpval.String(value)
		case "secure":
			cookie.Secure = phpval.Truthy(value)
		case "httponly":
			cookie.HttpOnly = phpval.Truthy(value)
		case "samesite":
			cookie.SameSite = sameSite(phpval.String(value))
		}
		return true
	})
}

// setCookieExpires sets a unix expiry, leaving the cookie a session cookie on zero.
func setCookieExpires(cookie *nethttp.Cookie, expires int64) {
	if expires == 0 {
		return
	}
	cookie.Expires = time.Unix(expires, 0).UTC()
}

// sameSite maps a PHP attribute string, an unrecognised value staging no attribute.
func sameSite(value string) nethttp.SameSite {
	switch value {
	case "Lax", "lax":
		return nethttp.SameSiteLaxMode
	case "Strict", "strict":
		return nethttp.SameSiteStrictMode
	case "None", "none":
		return nethttp.SameSiteNoneMode
	}
	return nethttp.SameSiteDefaultMode
}
