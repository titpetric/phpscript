// Package core holds the part of PHP's standard library that computes:
// strings, arrays, json, the language constructs exposed as functions, the
// tokenizer, the environment and the platform surface a program reads before
// it does anything interesting. It also holds the phpscript
// extensions that are each too small to be worth a package: SharedMemory,
// defer and register_shutdown_function.
//
// The line against stdlib/ps/compat runs on behaviour, not history: compat
// holds the surface whose behaviour comes from the interpreter (output
// buffering, preg_*, the date functions); core holds the surface whose
// behaviour it defines.
//
// Every file registers an area from its init(); adding, moving or dropping an
// area touches no other file. Importing the package wires them in, as a
// program imports a database/sql driver:
//
//	import _ "github.com/titpetric/phpscript/stdlib/ps/core"
//
// stdlib/imports.go does that for you.
package core
