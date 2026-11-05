package stdlib

// Blank imports wire the standard binding packages into any binary that uses
// stdlib: each one contributes its runtime installer through runner.
// RegisterArea in its init(), and Register runs them. A host needing a
// different set constructs its Runtime without this package, or passes extra
// bindings to Register.
import (
	_ "github.com/titpetric/phpscript/stdlib/go/database/sql"
	_ "github.com/titpetric/phpscript/stdlib/go/encoding/json"
	_ "github.com/titpetric/phpscript/stdlib/go/maps"
	_ "github.com/titpetric/phpscript/stdlib/go/path"
	_ "github.com/titpetric/phpscript/stdlib/go/slices"
	_ "github.com/titpetric/phpscript/stdlib/go/strings"
	_ "github.com/titpetric/phpscript/stdlib/ps/auth"
	_ "github.com/titpetric/phpscript/stdlib/ps/compat"
	_ "github.com/titpetric/phpscript/stdlib/ps/core"
	_ "github.com/titpetric/phpscript/stdlib/ps/crypto"
	_ "github.com/titpetric/phpscript/stdlib/ps/database"
	_ "github.com/titpetric/phpscript/stdlib/ps/files"
	_ "github.com/titpetric/phpscript/stdlib/ps/gd"
	_ "github.com/titpetric/phpscript/stdlib/ps/http"
	_ "github.com/titpetric/phpscript/stdlib/ps/info"
	_ "github.com/titpetric/phpscript/stdlib/ps/internals"
	_ "github.com/titpetric/phpscript/stdlib/ps/mail"
	_ "github.com/titpetric/phpscript/stdlib/ps/pexec"
	_ "github.com/titpetric/phpscript/stdlib/ps/regexp"
	_ "github.com/titpetric/phpscript/stdlib/ps/runtime"
	_ "github.com/titpetric/phpscript/stdlib/ps/session"
	_ "github.com/titpetric/phpscript/stdlib/ps/span"
	_ "github.com/titpetric/phpscript/stdlib/ps/time"
)
