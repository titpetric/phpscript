// Package time binds Go's time package as the Runtime\Go\Time namespace. A
// script imports it with `import "time";` and calls `time.Sleep($d)`; the
// default runtime installs none of these names.
package time

import (
	gotime "time"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = modelutil.GoNamespace + `\Time`

// Set is what the package binds.
var Set = bindings.For("time").
	// Runtime\Go\Time\Sleep pauses the script for $d, a duration or a string such as "100ms".
	Bind("Sleep", gotime.Sleep)

func init() {
	Set.Install()
}
