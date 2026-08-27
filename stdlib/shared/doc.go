// Package shared holds PHP behaviour that runner and stdlib both need.
//
// stdlib imports runner, never the reverse, so a behaviour both perform has
// nowhere to live in either. Form decoding is the case: runner builds $_GET
// and $_POST, parse_str() does the same to a string, and the two have to agree.
//
// Only that. A helper stdlib alone needs belongs in internal/phpval; one only
// runner needs belongs in runner.
package shared
