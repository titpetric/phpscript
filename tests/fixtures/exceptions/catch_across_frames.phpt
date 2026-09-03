name: a catch one frame above the throw survives jumps in the callee
description: >
  The handler a try arms belongs to the frame that armed it. A called
  function's branches, loops and other jumps run at program counters outside
  the caller's try body, and none of them may disarm the caller's catch: the
  throw that follows still lands in the clause one frame up. The callee
  settling its own throw first does not consume the caller's handler either.
  The expected output is what php 8.5 prints for this source.
---
<?php

// The catch lives one frame above the throw, and the callee branches before
// throwing. The branch must not cost the caller its armed handler.
function branchy() {
	if (true) {
		echo "branch\n";
	}
	throw new RuntimeException("from branchy");
}

try {
	branchy();
} catch (RuntimeException $e) {
	echo "caught: " . $e->getMessage() . "\n";
}

// A loop in the callee jumps backwards on every iteration.
function loopy() {
	for ($i = 0; $i < 3; $i++) {
		echo "tick " . $i . "\n";
	}
	throw new RuntimeException("from loopy");
}

try {
	loopy();
} catch (RuntimeException $e) {
	echo "caught: " . $e->getMessage() . "\n";
}

// The callee settles its own throw first; only the second one is the caller's.
function nested() {
	try {
		throw new LogicException("inner");
	} catch (LogicException $e) {
		echo "inner: " . $e->getMessage() . "\n";
	}
	throw new RuntimeException("outer");
}

try {
	nested();
} catch (RuntimeException $e) {
	echo "caught: " . $e->getMessage() . "\n";
}

echo "end\n";
?>
---
branch
caught: from branchy
tick 0
tick 1
tick 2
caught: from loopy
inner: inner
caught: outer
end
