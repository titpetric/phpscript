package runner_test

import (
	"net/http/httptest"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// browserHeaders holds the headers an ordinary browser sends. The count
// matters: a request holds a dozen or so; an eager decode would copy each
// header into $_SERVER with the HTTP_ spelling even when the script reads none.
var browserHeaders = map[string]string{
	"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	"Accept-Encoding":           "gzip, deflate, br",
	"Accept-Language":           "en-GB,en;q=0.9",
	"Cache-Control":             "max-age=0",
	"Connection":                "keep-alive",
	"Cookie":                    "session=abc123; theme=dark",
	"Referer":                   "https://example.com/previous",
	"Sec-Fetch-Dest":            "document",
	"Sec-Fetch-Mode":            "navigate",
	"Sec-Fetch-Site":            "same-origin",
	"Upgrade-Insecure-Requests": "1",
	"User-Agent":                "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
}

// BenchmarkRegisterSuperglobals covers a script naming no superglobal, the common page.
func BenchmarkRegisterSuperglobals(b *testing.B) {
	req := httptest.NewRequest("GET", "http://example.com/index.php?page=2&sort=name", nil)
	for name, value := range browserHeaders {
		req.Header.Set(name, value)
	}

	rt := runner.New(nil, runner.Options{})
	b.ReportAllocs()
	for b.Loop() {
		ctx := runner.FromRequest(req)
		ctx.Register(rt)
	}
}

// BenchmarkRegisterAndReadOne covers the same request where the script reads a
// single name, as a router reads REQUEST_URI.
func BenchmarkRegisterAndReadOne(b *testing.B) {
	req := httptest.NewRequest("GET", "http://example.com/index.php?page=2&sort=name", nil)
	for name, value := range browserHeaders {
		req.Header.Set(name, value)
	}

	rt := runner.New(nil, runner.Options{})
	program, err := rt.Load(`<?php $uri = $_SERVER["REQUEST_URI"];`)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		ctx := runner.FromRequest(req)
		ctx.Register(rt)
		if err := rt.Run(program); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRegisterOnly separates registering the superglobals from building
// the Context they come from, most of the work the two benchmarks above
// measure.
func BenchmarkRegisterOnly(b *testing.B) {
	req := httptest.NewRequest("GET", "http://example.com/index.php?page=2&sort=name", nil)
	for name, value := range browserHeaders {
		req.Header.Set(name, value)
	}
	ctx := runner.FromRequest(req)

	rt := runner.New(nil, runner.Options{})
	b.ReportAllocs()
	for b.Loop() {
		ctx.Register(rt)
	}
}
