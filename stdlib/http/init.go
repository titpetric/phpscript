// Package http provides the HTTP\Client and HTTP\Request bindings, the
// outbound HTTP surface a script reaches instead of PHP's curl_* family.
//
// The types a script sees are facades over net/http rather than the net/http
// types themselves. Registration is not a method allowlist: every exported
// method and field of the value a constructor returns is reachable from PHP,
// so embedding *net/http.Request would publish the whole of net/http as
// methods on HTTP\Request. Each facade therefore holds its net/http value in a
// named unexported field, and the methods below are the whole surface.
package http

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the HTTP bindings to stdlib.Register, the way a program
// wires in a database/sql driver.
func init() {
	runner.RegisterBinding(Register)
}
