package stdlib

// Blank imports wire the standard binding packages into any binary that uses
// stdlib: each one contributes its runtime installer through runner.
// RegisterBinding in its init(), and Register runs them. A host that wants a
// different set constructs its Runtime without this package, or passes extra
// bindings to Register.
import (
	_ "github.com/titpetric/phpscript/stdlib/compat"
	_ "github.com/titpetric/phpscript/stdlib/database"
	_ "github.com/titpetric/phpscript/stdlib/files"
	_ "github.com/titpetric/phpscript/stdlib/info"
	_ "github.com/titpetric/phpscript/stdlib/ps"
	_ "github.com/titpetric/phpscript/stdlib/smtp"
	_ "github.com/titpetric/phpscript/stdlib/span"
)
