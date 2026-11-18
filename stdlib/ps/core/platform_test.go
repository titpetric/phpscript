package core_test

import (
	"runtime"
	"testing"
)

// TestStrtr covers both spellings of strtr. Three arguments replace bytes
// positionally, stopping at the shorter of the two tables; a $to shorter
// than $from leaves the uncovered bytes alone. Two arguments replace
// substrings, longest key first. The result is independent of the order the
// pairs appear in. A replacement never rewrites its output:
// ["a" => "b", "b" => "c"] over "abc" is "bcc", never "ccc".
func TestStrtr(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
echo strtr("hello", "el", "ip"), "\n";
echo strtr("hi all", ["hi" => "hello", "all" => "world"]), "\n";
echo strtr("abc", ["a" => "b", "b" => "c"]), "\n";
echo strtr("xyz", ["x" => "1", "xy" => "2", "xyz" => "3"]), "\n";
echo strtr("hello", "abc", "x"), "\n";`)

	want := "hippo\nhello world\nbcc\n3\nhello\n"
	if got != want {
		t.Errorf("strtr = %q, want %q", got, want)
	}
}

// TestStrrpos covers the offset argument, which means two different things by
// its sign. A positive offset skips that many leading bytes; the search
// runs over the rest. A negative one caps where the match may start, counted
// back from the end. The result is still a position in the whole string:
// -5 over "hello world" finds the "o" at 4 and skips the "o" at 7.
//
// A needle that does not occur is false, not -1. A caller
// compares the miss with === false.
func TestStrrpos(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(strrpos("hello world", "o"), strrpos("hello world", "o", 5), strrpos("hello world", "o", -5), strrpos("hello", "z"));`)

	want := "int(7)\nint(7)\nint(4)\nbool(false)\n"
	if got != want {
		t.Errorf("strrpos = %q, want %q", got, want)
	}
}

// TestFilterVar covers three validators this runtime implements: each
// answers the typed value or false; the input never comes back. "42" validates to
// the int 42; a caller can use the result directly. filter_var trims the
// value first; a float spelling fails the integer validator and leaves the
// value whole.
//
// An unknown filter passes the value through, as FILTER_DEFAULT does.
func TestFilterVar(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(filter_var("42", FILTER_VALIDATE_INT));
var_dump(filter_var("4.2", FILTER_VALIDATE_INT));
var_dump(filter_var(" 42 ", FILTER_VALIDATE_INT));
var_dump(filter_var("4.5", FILTER_VALIDATE_FLOAT));
var_dump(filter_var("abc", FILTER_VALIDATE_FLOAT));
var_dump(filter_var("yes", FILTER_VALIDATE_BOOLEAN));
var_dump(filter_var("off", FILTER_VALIDATE_BOOLEAN));
var_dump(filter_var("maybe", FILTER_VALIDATE_BOOLEAN));
var_dump(filter_var("passed through"));`)

	want := "int(42)\nbool(false)\nint(42)\nfloat(4.5)\nbool(false)\n" +
		"bool(true)\nbool(false)\nbool(false)\nstring(14) \"passed through\"\n"
	if got != want {
		t.Errorf("filter_var = %q, want %q", got, want)
	}
}

// TestDefineDefinedConstant covers three functions a script uses to
// define constants. define answers true; defined answers for a name nothing
// defines; constant reads one back by name. A script reaches a constant
// whose name it computed.
func TestDefineDefinedConstant(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(define("GREETING", "hello"));
var_dump(defined("GREETING"), defined("NOPE"));
var_dump(constant("GREETING"));`)

	want := "bool(true)\nbool(true)\nbool(false)\nstring(5) \"hello\"\n"
	if got != want {
		t.Errorf("define, defined and constant = %q, want %q", got, want)
	}
}

// TestPlatformIntegerConstants covers the constants a library branches on
// before it does anything else. The integer limits are the 64-bit ones,
// the only width this runtime has. E_ALL is 30719; that leaves
// E_STRICT out as php 8 does.
func TestPlatformIntegerConstants(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(PHP_INT_MAX, PHP_INT_MIN, PHP_INT_SIZE, PHP_FLOAT_DIG, PHP_EOL);
var_dump(E_ALL, E_ERROR, E_WARNING, E_NOTICE, E_DEPRECATED);`)

	want := "int(9223372036854775807)\nint(-9223372036854775808)\nint(8)\nint(15)\nstring(1) \"\n\"\n" +
		"int(30719)\nint(1)\nint(2)\nint(8)\nint(8192)\n"
	if got != want {
		t.Errorf("platform constants = %q, want %q", got, want)
	}
}

// TestPlatformVersion covers the version this runtime reports. That is a
// decision, not a measurement: the fixtures follow that version's
// semantics. A library reads the number when it picks the language
// features it may use. Three spellings must agree; a library
// gates on whichever spelling it learned.
//
// The php binary the fixture matrix runs beside reports its version.
// This test states the value and settles nothing by running php.
func TestPlatformVersion(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(PHP_VERSION, PHP_MAJOR_VERSION, PHP_MINOR_VERSION, PHP_RELEASE_VERSION, PHP_EXTRA_VERSION, PHP_VERSION_ID);
var_dump(phpversion(), zend_version());`)

	want := "string(5) \"8.4.0\"\nint(8)\nint(4)\nint(0)\nstring(0) \"\"\nint(80400)\n" +
		"string(5) \"8.4.0\"\nstring(5) \"8.4.0\"\n"
	if got != want {
		t.Errorf("version surface = %q, want %q", got, want)
	}
}

// TestPlatformOS covers PHP_OS and PHP_OS_FAMILY, which hold php's spelling
// of the host and not Go's: php reads the uname system name and linux is
// "Linux". The family is the coarser grouping a script branches on. The
// two agree on every platform this runs on, except the BSDs and Solaris.
func TestPlatformOS(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skipf("the expected values are linux's; this is %s", runtime.GOOS)
	}
	got := runPHP(t, `<?php
var_dump(PHP_OS, PHP_OS_FAMILY, php_uname());`)

	want := "string(5) \"Linux\"\nstring(5) \"Linux\"\nstring(5) \"Linux\"\n"
	if got != want {
		t.Errorf("os surface = %q, want %q", got, want)
	}
}

// TestIniStubs covers the configuration surface, which answers the same way
// for every directive because this runtime has no php.ini to read. php reports
// an unset directive for an unknown name. Library code reads it as "this
// extension is not configured"; a probe for
// APCu or an opcache finds nothing and falls back.
//
// headers_sent is false for the length of a script. The runtime stages
// response headers, writes them after it finishes, and nothing leaves while
// php is still running.
func TestIniStubs(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(ini_get("precision"), ini_set("precision", "5"), error_reporting(), extension_loaded("apcu"), headers_sent());`)

	want := "bool(false)\nbool(false)\nint(0)\nbool(false)\nbool(false)\n"
	if got != want {
		t.Errorf("configuration stubs = %q, want %q", got, want)
	}
}
