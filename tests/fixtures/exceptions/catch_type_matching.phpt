name: a try enters the first catch clause whose declared type matches
description: >
  A try with several catch clauses picks by declared type in source order, not
  by taking the first clause written: a RuntimeException skips catch
  (LogicException) and reaches the clause after it, a union type matches on
  either alternative, and Throwable matches anything. When no clause matches,
  the throw keeps propagating to the enclosing try and the finally block still
  runs on the way out. The expected output is what php 8.5 prints for this
  source.
---
<?php

// A later clause runs when the earlier one declares a type that does not match.
try {
	throw new RuntimeException("first");
} catch (LogicException $e) {
	echo "wrong: LogicException caught a RuntimeException\n";
} catch (RuntimeException $e) {
	echo "second clause: " . $e->getMessage() . "\n";
}

// Throwable after a non-matching clause still catches everything.
try {
	throw new RuntimeException("second");
} catch (LogicException $e) {
	echo "wrong: LogicException caught a RuntimeException\n";
} catch (Throwable $e) {
	echo "throwable clause: " . $e->getMessage() . "\n";
}

// A union type matches on its second alternative.
try {
	throw new RuntimeException("third");
} catch (LogicException|RuntimeException $e) {
	echo "union clause: " . $e->getMessage() . "\n";
}

// finally runs when a clause matched.
try {
	throw new RuntimeException("fourth");
} catch (RuntimeException $e) {
	echo "matched: " . $e->getMessage() . "\n";
} finally {
	echo "finally after match\n";
}

// finally runs when no clause matched, and the throw keeps propagating.
try {
	try {
		throw new RuntimeException("fifth");
	} catch (LogicException $e) {
		echo "wrong: LogicException caught a RuntimeException\n";
	} finally {
		echo "finally after no match\n";
	}
	echo "wrong: reached code after an unmatched throw\n";
} catch (Throwable $e) {
	echo "propagated: " . $e->getMessage() . "\n";
}

// A throw out of a clause body is not offered to the sibling clauses of the
// same try, but its finally block still runs on the way out.
try {
	try {
		throw new RuntimeException("sixth");
	} catch (RuntimeException $e) {
		throw new LogicException("rethrown");
	} catch (Throwable $e) {
		echo "wrong: a sibling clause caught a rethrow\n";
	} finally {
		echo "finally after rethrow\n";
	}
} catch (Throwable $e) {
	echo "rethrown out: " . $e->getMessage() . "\n";
}

// The clause order is source order, not declared-type specificity.
try {
	throw new RuntimeException("seventh");
} catch (Throwable $e) {
	echo "first clause wins: " . $e->getMessage() . "\n";
} catch (RuntimeException $e) {
	echo "wrong: a later clause ran\n";
}

echo "still running\n";
?>
---
second clause: first
throwable clause: second
union clause: third
matched: fourth
finally after match
finally after no match
propagated: fifth
finally after rethrow
rethrown out: rethrown
first clause wins: seventh
still running
