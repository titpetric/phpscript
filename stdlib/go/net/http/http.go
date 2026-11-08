// Package http binds go's HTTP client and the names around it as the
// Runtime\Go\Net\HTTP area. A script reaches them with `import "net/http";`
// and calls `http.Get($url)`; the default runtime carries none of these names.
//
// The area reaches any address the host can route to, as `HTTP\Client` does.
// The server half stays out: a php handler answers through `HTTP\Server`,
// which owns the re-entry the runtime needs (docs/use-cases/http-server.md).
package http

import (
	gohttp "net/http"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = modelutil.GoNamespace + `\Net\HTTP`

// Set is what the package binds. The signatures reach `http.Request`,
// `http.Response`, `http.Header` and `http.Cookie`, which the area enumerates
// as classes of its own.
var Set = bindings.For("net/http").
	// Runtime\Go\Net\HTTP\Get issues a GET to $url and returns the response.
	Bind("Get", gohttp.Get).
	// Runtime\Go\Net\HTTP\Head issues a HEAD to $url and returns the response.
	Bind("Head", gohttp.Head).
	// Runtime\Go\Net\HTTP\NewRequest builds a request for $method and $url, with $body or null.
	Bind("NewRequest", gohttp.NewRequest).
	// Runtime\Go\Net\HTTP\StatusText returns the text of status code $code, or the empty string.
	Bind("StatusText", gohttp.StatusText).
	// Runtime\Go\Net\HTTP\CanonicalHeaderKey returns $s in the canonical spelling of a header name.
	Bind("CanonicalHeaderKey", gohttp.CanonicalHeaderKey).
	// Runtime\Go\Net\HTTP\DetectContentType reports the MIME type the first bytes of $data name.
	Bind("DetectContentType", gohttp.DetectContentType).
	// Runtime\Go\Net\HTTP\ParseTime reads $text as one of the three HTTP time formats.
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
	// `new http.Client` is what sends a request the script built, with a
	// timeout of its own. No bound signature names the type.
	Type(gohttp.Client{})

func init() {
	Set.Install()
}
