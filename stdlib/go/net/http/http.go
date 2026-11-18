// Package http binds go's HTTP client and the names around it as the
// Runtime\Go\Net\HTTP area. A script reaches them through `import "net/http";`
// and calls `http.Get($url)`; the default runtime installs none of these names.
//
// The area reaches any address the host can route to, like `HTTP\Client`.
// The server half is `HTTP\Mux` and `HTTP\Server`, which own the re-entry the
// runtime needs (docs/use-cases/http-server.md) and answer here as ServeMux and
// Server for a runtime that selected Runtime\Go.
package http

import (
	gohttp "net/http"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = modelutil.GoNamespace + `\Net\HTTP`

// Set holds the package bindings. The signatures reach `http.Request`,
// `http.Response`, `http.Header` and `http.Cookie`, which the area enumerates
// as classes.
var Set = bindings.For("net/http").
	// Runtime\Go\Net\HTTP\Get issues a GET to $url and returns the response.
	Bind("Get", gohttp.Get).
	// Runtime\Go\Net\HTTP\Head issues a HEAD to $url and returns the response.
	Bind("Head", gohttp.Head).
	// Runtime\Go\Net\HTTP\NewRequest builds a request for $method and $url, with $body or null.
	Bind("NewRequest", gohttp.NewRequest).
	// Runtime\Go\Net\HTTP\StatusText returns the text of status code $code, or the empty string.
	Bind("StatusText", gohttp.StatusText).
	// Runtime\Go\Net\HTTP\Error answers $w with status $code and the text of $msg.
	Bind("Error", gohttp.Error).
	// Runtime\Go\Net\HTTP\NotFound answers $w with net/http's 404 text.
	Bind("NotFound", gohttp.NotFound).
	// Runtime\Go\Net\HTTP\CanonicalHeaderKey returns $s in the canonical spelling of a header name.
	Bind("CanonicalHeaderKey", gohttp.CanonicalHeaderKey).
	// Runtime\Go\Net\HTTP\DetectContentType reports the MIME type the first bytes of $data name.
	Bind("DetectContentType", gohttp.DetectContentType).
	// Runtime\Go\Net\HTTP\ParseTime reads $text in any of three HTTP time formats.
	Bind("ParseTime", gohttp.ParseTime).
	Bind("MethodGet", gohttp.MethodGet).
	Bind("MethodHead", gohttp.MethodHead).
	Bind("MethodPost", gohttp.MethodPost).
	Bind("MethodPut", gohttp.MethodPut).
	Bind("MethodPatch", gohttp.MethodPatch).
	Bind("MethodDelete", gohttp.MethodDelete).
	Bind("StatusOK", gohttp.StatusOK).
	Bind("StatusCreated", gohttp.StatusCreated).
	Bind("StatusNoContent", gohttp.StatusNoContent).
	Bind("StatusMovedPermanently", gohttp.StatusMovedPermanently).
	Bind("StatusFound", gohttp.StatusFound).
	Bind("StatusNotModified", gohttp.StatusNotModified).
	Bind("StatusBadRequest", gohttp.StatusBadRequest).
	Bind("StatusUnauthorized", gohttp.StatusUnauthorized).
	Bind("StatusForbidden", gohttp.StatusForbidden).
	Bind("StatusNotFound", gohttp.StatusNotFound).
	Bind("StatusTooManyRequests", gohttp.StatusTooManyRequests).
	Bind("StatusInternalServerError", gohttp.StatusInternalServerError).
	Bind("StatusBadGateway", gohttp.StatusBadGateway).
	Bind("StatusServiceUnavailable", gohttp.StatusServiceUnavailable).
	// `new http.Client` sends a request the script built, within the timeout
	// the Timeout field sets. No bound signature names the type.
	Type(gohttp.Client{})

func init() {
	runner.RegisterNamespace(Namespace, Register)
}
