// Package fmt binds the formatting calls of go's fmt under the Runtime\Go\Fmt
// area. A script reaches them with `import "fmt";` and calls
// `fmt.Sprintf($format, $args)`; the default runtime carries none of these
// names.
package fmt

import (
	gofmt "fmt"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = modelutil.GoNamespace + `\Fmt`

// Set is what the package binds. The printing functions stay out: a script
// writes its output with echo, and `fmt.Println` would write past the output
// buffer the runtime holds.
var Set = bindings.For("fmt").
	// Runtime\Go\Fmt\Sprintf formats $args under $format and returns the string.
	Bind("Sprintf", gofmt.Sprintf).
	// Runtime\Go\Fmt\Sprint concatenates $args, with spaces between neighbours that are both not strings.
	Bind("Sprint", gofmt.Sprint).
	// Runtime\Go\Fmt\Sprintln concatenates $args with spaces between them and a newline behind.
	Bind("Sprintln", gofmt.Sprintln)

func init() {
	Set.Install()
}
