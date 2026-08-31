package bindings

import (
	"net/http"
	"net/url"
	"time"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the request-aware functions to stdlib.Register.
func init() {
	runner.RegisterBinding(registerRequest)
}

// registerRequest installs the functions that answer for the request the
// runtime is serving.
//
// They resolve the request at call time through runner.RequestContext, not at
// registration time, because a host registers the standard library once and
// seeds the request afterwards - stdlib.Register(rt) then reqCtx.Register(rt),
// which is the order every host uses. That indirection is also what lets these
// live outside package runner.
//
// A script running without a request - the cli SAPI, a fixture that seeds no
// request - finds the functions present and inert, which is what PHP's own cli
// SAPI does with header().
func registerRequest(rt *runner.Runtime) {
	// getallheaders returns the request headers as an associative array keyed by canonical header name, and an empty array when there is no request.
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

	// header stages the "Name: value" response header in $header, written to the response after the script finishes; $replace (default true) overwrites an existing header of the same name, $code stages the response status, and a status line such as "HTTP/1.0 404 Not Found" stages the status it names.
	rt.RegisterFunc("header", func(header string, opts ...any) {
		request, ok := runner.RequestContext(rt.Context())
		if !ok {
			return
		}
		request.Header(header, opts...)
	})

	// http_response_code stages the response status in $response_code and returns the one it replaced; called without one it returns the status the response will be sent with, or false when there is no request to answer for.
	rt.RegisterFunc("http_response_code", func(opts ...any) any {
		request, ok := runner.RequestContext(rt.Context())
		if !ok {
			return false
		}
		return request.HTTPResponseCode(rt.SAPI(), opts...)
	})

	// setcookie stages a Set-Cookie header naming $name with $value url-encoded, and answers whether it could; $expires_or_options is a unix timestamp, 0 for a cookie that dies with the browser session, or an array of expires, path, domain, secure, httponly and samesite.
	rt.RegisterFunc("setcookie", func(name string, opts ...any) bool {
		return stageCookie(rt, name, opts, true)
	})

	// setrawcookie stages a Set-Cookie header the way setcookie does but writes $value as it stands, so a value carrying a semicolon or a space is the caller's problem rather than the encoder's.
	rt.RegisterFunc("setrawcookie", func(name string, opts ...any) bool {
		return stageCookie(rt, name, opts, false)
	})
}

// stageCookie builds the cookie both spellings stage.
//
// The header text is net/http's, not PHP's: attributes read Path and HttpOnly
// where PHP writes path and HttpOnly, and the expiry is RFC 1123 where PHP
// writes its own dashed variant. RFC 6265 makes attribute names
// case-insensitive and both date spellings parseable, so a client cannot tell;
// a test asserting the exact header text can, which is why this is written
// down.
func stageCookie(rt *runner.Runtime, name string, opts []any, encode bool) bool {
	request, ok := runner.RequestContext(rt.Context())
	if !ok {
		return false
	}

	cookie := &http.Cookie{Name: name}
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
		if options, isArray := opts[1].(*model.Array); isArray {
			applyCookieOptions(cookie, options)
		} else {
			applyCookiePositional(cookie, opts[1:])
		}
	}

	request.AddResponseHeader("Set-Cookie", cookie.String())
	return true
}

// applyCookiePositional reads the long argument list: $expires, $path,
// $domain, $secure, $httponly. There is no samesite in this form, which is why
// PHP grew the array one.
func applyCookiePositional(cookie *http.Cookie, args []any) {
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

// applyCookieOptions reads the array form. An option the array does not name
// keeps its zero value, which is the same as PHP leaving it out of the header.
func applyCookieOptions(cookie *http.Cookie, options *model.Array) {
	if value, ok := options.Get("expires"); ok {
		setCookieExpires(cookie, phpval.Int(value))
	}
	if value, ok := options.Get("path"); ok {
		cookie.Path = phpval.String(value)
	}
	if value, ok := options.Get("domain"); ok {
		cookie.Domain = phpval.String(value)
	}
	if value, ok := options.Get("secure"); ok {
		cookie.Secure = phpval.Truthy(value)
	}
	if value, ok := options.Get("httponly"); ok {
		cookie.HttpOnly = phpval.Truthy(value)
	}
	if value, ok := options.Get("samesite"); ok {
		cookie.SameSite = sameSite(phpval.String(value))
	}
}

// setCookieExpires reads PHP's $expires: a unix timestamp, where 0 means the
// cookie lives as long as the browser session and carries no expiry at all.
func setCookieExpires(cookie *http.Cookie, expires int64) {
	if expires == 0 {
		return
	}
	cookie.Expires = time.Unix(expires, 0).UTC()
}

// sameSite reads the attribute PHP spells as a string. An unrecognised value
// stages no attribute, which is what a browser does with one it cannot read.
func sameSite(value string) http.SameSite {
	switch value {
	case "Lax", "lax":
		return http.SameSiteLaxMode
	case "Strict", "strict":
		return http.SameSiteStrictMode
	case "None", "none":
		return http.SameSiteNoneMode
	}
	return http.SameSiteDefaultMode
}
