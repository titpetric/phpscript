package core_test

import (
	"strings"
	"testing"
)

// TestClosureFromCallable covers turning a callable into a closure a script can
// call. Every spelling php accepts as a callable arrives here through
// rt.Callable, so a function name, a closure literal and a method pair all
// resolve; a value nothing can call is an error, where php raises a TypeError.
func TestClosureFromCallable(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
		function double($x) { return $x * 2; }
		$c = Closure::fromCallable("double");
		echo $c(21);
	`)
	if got != "42" {
		t.Errorf("the closure answered %q, want %q", got, "42")
	}

	got = runPHP(t, `<?php
		$literal = function ($x) { return $x + 1; };
		$c = Closure::fromCallable($literal);
		echo $c(41);
	`)
	if got != "42" {
		t.Errorf("a closure literal round-tripped to %q, want %q", got, "42")
	}

	got = runPHP(t, `<?php
		try { Closure::fromCallable("no_such_function"); }
		catch (Exception $e) { echo $e->getMessage(); }
	`)
	if !strings.Contains(got, "not callable") {
		t.Errorf("an uncallable name reported %q, want a message naming it as not callable", got)
	}
}

// TestClosureBind covers the one scope change this runtime can make, which is
// none. No property visibility is enforced here, so a scope argument has
// nothing to alter and the closure comes back unchanged. Rebinding $this would
// change what the body reads, so it is refused and never ignored in silence.
func TestClosureBind(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
		$fn = function () { return 7; };
		$bound = Closure::bind($fn, null);
		echo $bound();
	`)
	if got != "7" {
		t.Errorf("the bound closure answered %q, want %q", got, "7")
	}

	got = runPHP(t, `<?php
		$fn = function () { return 7; };
		try { Closure::bind($fn, 1); }
		catch (Exception $e) { echo $e->getMessage(); }
	`)
	if !strings.Contains(got, "rebinding $this is not supported") {
		t.Errorf("a non-null scope reported %q, want the refusal", got)
	}
}
