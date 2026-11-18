// Package errors binds go's error constructors in the Runtime\Go\Errors
// area. A script reaches the constructors by writing `import "errors";` and then
// `throw errors.New("...")`; the default runtime installs none of these names.
package errors

import (
	goerrors "errors"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = modelutil.GoNamespace + `\Errors`

// Set holds the package's bindings.
var Set = bindings.For("errors").
	// Runtime\Go\Errors\New returns an error holding $text.
	Bind("New", newError)

// newError answers the error as a value: the invoker reads an error-typed slot as a failure.
func newError(text string) any { return goerrors.New(text) }

func init() {
	Set.Install()
}
