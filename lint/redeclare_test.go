package lint_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/lint"
)

// TestRedeclareIsFatal covers the severity: PHP refuses a file declaring one
// name twice and so does this runtime, so the finding fails the lint run. The
// message names the first declaration's line, which is the half a
// reader needs to find the pair.
func TestRedeclareIsFatal(t *testing.T) {
	t.Parallel()
	found, err := lint.File("index.php", `<?php
function helper() { return 1; }

function helper() { return 2; }`)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("findings = %v, want one", found)
	}
	if !found[0].Fatal {
		t.Error("the finding is advisory, want it fatal")
	}
	if want := "Cannot redeclare function helper() (previously declared on line 2)"; found[0].Message != want {
		t.Errorf("message = %q, want %q", found[0].Message, want)
	}
	if found[0].Line != 4 {
		t.Errorf("line = %d, want the second declaration on 4", found[0].Line)
	}
}

// TestRedeclareOverAHostName covers declaring over a name the runtime itself
// registers: strlen is installed by the standard library, so a file declaring
// it is refused the same way, with no earlier line to name.
func TestRedeclareOverAHostName(t *testing.T) {
	t.Parallel()
	found, err := lint.File("index.php", `<?php
function strlen($s) { return 0; }`)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if !has(messagesOf(found), "Cannot redeclare function strlen()") {
		t.Fatalf("findings = %v, want the host name refused", found)
	}
	for _, finding := range found {
		if strings.Contains(finding.Message, "Cannot redeclare") && !finding.Fatal {
			t.Error("the finding is advisory, want it fatal")
		}
	}
}

// TestRedeclareSkipsAGuardedName covers the polyfill idiom: declaring over an
// absent built-in is what function_exists guards, and the author has written
// both branches, so neither the duplicate nor the host name is reported.
func TestRedeclareSkipsAGuardedName(t *testing.T) {
	t.Parallel()
	found := messagesOf(mustLint(t, `<?php
if (!function_exists("str_contains")) {
	function str_contains($haystack, $needle) { return false; }
}`))
	if has(found, "Cannot redeclare") {
		t.Errorf("findings = %v, want a guarded declaration left alone", found)
	}
}

// TestRedeclareCountsAMethodApart covers the one name that is not a function
// name: a method belongs to its class, so two classes declaring render() are
// two methods and no collision.
func TestRedeclareCountsAMethodApart(t *testing.T) {
	t.Parallel()
	found := messagesOf(mustLint(t, `<?php
class Page { function render() { return 1; } }
class Post { function render() { return 2; } }`))
	if has(found, "Cannot redeclare") {
		t.Errorf("findings = %v, want two methods of two classes", found)
	}
}

// mustLint runs the linter over src and fails the test when it could not.
func mustLint(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	found, err := lint.File("index.php", src)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	return found
}

// messagesOf is the message of every finding, for a case asserting on text.
func messagesOf(found []lint.Diagnostic) []string {
	out := make([]string, 0, len(found))
	for _, finding := range found {
		out = append(out, finding.Message)
	}
	return out
}
