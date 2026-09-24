package runner_test

import (
	"net/http/httptest"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// browserHeaders is what an ordinary browser sends. The count is the point: a
// request carries a dozen or so, each of which used to be copied into $_SERVER
// under its HTTP_ name whether or not the script read one.
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

// BenchmarkRegisterSuperglobals covers what a request pays to reach a script
// that never names a superglobal, which is the common page.
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
// single name, which is what a router does with REQUEST_URI.
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
// the Context they come from, which is most of what the two benchmarks above
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
