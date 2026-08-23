// Package core holds the part of PHP's standard library that computes:
// strings, arrays, json, the language constructs exposed as functions, the
// tokenizer, the environment and the platform surface a program expects to
// find before it does anything interesting. It also holds the phpscript
// extensions that are each too small to be worth a package: SharedMemory,
// defer and register_shutdown_function.
//
// The line against stdlib/compat is behavioural, not historical: compat holds
// the surface whose behaviour is the interpreter's (output buffering, preg_*,
// the date functions), core holds the surface whose behaviour is its own.
//
// Every file registers its own area from its own init(), so an area is added,
// moved or dropped without touching another file. Importing the package is
// what wires them in, the way a program imports a database/sql driver:
//
//	import _ "github.com/titpetric/phpscript/stdlib/core"
//
// stdlib/imports.go does that for you.
package core
