// Package shared holds PHP behaviour that runner and stdlib both need.
//
// stdlib imports runner; the reverse never happens. A behaviour both perform
// has no home in either. Form decoding is the case: runner builds $_GET and $_POST,
// parse_str() does the same to a string. The two must agree.
//
// Only that. A helper stdlib alone needs belongs in internal/phpval; one only
// runner needs belongs in runner.
package shared
