name: exit inside try
description: exit() is not catchable and runs no finally block, so a redirect written inside a try still ends the script.
---
<?php
function guarded() {
	try {
		echo "before\n";
		exit();
		echo "unreachable\n";
	} catch (Exception $e) {
		echo "caught: " . $e->getMessage() . "\n";
	} finally {
		echo "finally\n";
	}

	echo "after the try\n";
}

guarded();

echo "after the call\n";
---
before
