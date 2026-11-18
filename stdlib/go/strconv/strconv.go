// Package strconv binds the number conversions of Go's strconv package as the
// Runtime\Go\Strconv namespace. A script writes `import "strconv";` and calls
// `strconv.Atoi($s)`; the default runtime installs none of these names.
package strconv

import (
	gostrconv "strconv"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = modelutil.GoNamespace + `\Strconv`

// Set holds the package's bindings.
var Set = bindings.For("strconv").
	// Runtime\Go\Strconv\Atoi parses $s as a base-10 integer.
	Bind("Atoi", gostrconv.Atoi).
	// Runtime\Go\Strconv\Itoa renders $i in base 10.
	Bind("Itoa", gostrconv.Itoa)

func init() {
	Set.Install()
}
