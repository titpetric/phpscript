package lint_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/lint"
)

// messages collects the findings of one source, so a case asserts on text and
// never on index positions that shift when a check is added.
func messages(t *testing.T, src string) []string {
	t.Helper()
	found, err := lint.File("index.php", src)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	out := make([]string, 0, len(found))
	for _, finding := range found {
		out = append(out, finding.Message)
	}
	return out
}

// has reports a finding whose message carries want.
func has(found []string, want string) bool {
	for _, message := range found {
		if strings.Contains(message, want) {
			return true
		}
	}
	return false
}

// TestReassignReportsADriftingType covers the convention the flat VM's typed
// operator selection leans on: the first literal assigned to a name declares
// its type, and a later literal of another type is the drift. It stays
// advisory, so the finding is never fatal and the program still runs.
func TestReassignReportsADriftingType(t *testing.T) {
	found, err := lint.File("index.php", `<?php
function render() {
	$total = 1;
	$total = "two";
}`)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("findings = %v, want one", found)
	}
	if !strings.Contains(found[0].Message, `$total previously declared as int`) {
		t.Errorf("message = %q", found[0].Message)
	}
	if found[0].Fatal {
		t.Error("the finding is fatal, want it advisory")
	}
}

// TestReassignIsOneScopePerBody covers where the type table lives: one per
// function body, with branches sharing it. An if arm assigning a string where
// the else arm assigns an int is the drift the rule names, and the same name
// in a second function is a second variable and no conflict.
func TestReassignIsOneScopePerBody(t *testing.T) {
	branches := messages(t, `<?php
function render($flag) {
	if ($flag) {
		$value = 1;
	} else {
		$value = "one";
	}
}`)
	if !has(branches, `$value previously declared as int`) {
		t.Errorf("findings = %v, want the drift across the two arms", branches)
	}

	separate := messages(t, `<?php
function first() {
	$value = 1;
}
function second() {
	$value = "one";
}`)
	if has(separate, "no reassignment") {
		t.Errorf("findings = %v, want the two bodies read apart", separate)
	}
}

// TestReassignIgnoresComputedValues covers what records no type and conflicts
// with none: a call, another variable or any computed value says nothing about
// what a name holds, and null is how PHP initialises a variable before the
// real value arrives.
func TestReassignIgnoresComputedValues(t *testing.T) {
	for _, src := range []string{
		`<?php
function render($in) {
	$value = 1;
	$value = strlen($in);
}`,
		`<?php
function render($in) {
	$value = 1;
	$value = $in;
}`,
		`<?php
function render() {
	$value = null;
	$value = "one";
}`,
		`<?php
function render() {
	$value = 1;
	$value += 2;
}`,
	} {
		if found := messages(t, src); has(found, "no reassignment") {
			t.Errorf("findings = %v for %s", found, src)
		}
	}
}

// TestReassignReportsAStringOffset covers the second finding this file
// produces: $s[$i] on a name declared as a string counts bytes, so a
// multi-byte character reads as one of its bytes. It is silent for an array
// under the same syntax, where indexing is what arrays are for.
func TestReassignReportsAStringOffset(t *testing.T) {
	onAString := messages(t, `<?php
function first_byte($i) {
	$text = "hello";
	return $text[$i];
}`)
	if !has(onAString, "string offset $text[...]") {
		t.Errorf("findings = %v, want the byte-offset finding", onAString)
	}

	onAnArray := messages(t, `<?php
function pick($i) {
	$rows = [1, 2, 3];
	return $rows[$i];
}`)
	if has(onAnArray, "string offset") {
		t.Errorf("findings = %v, want an array index left alone", onAnArray)
	}
}
