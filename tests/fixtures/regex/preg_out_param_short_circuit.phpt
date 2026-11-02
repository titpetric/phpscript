name: out parameter in a short circuit
description: A match written into an out parameter is visible to the next operand of &&.
---
<?php
$body = "@admin @ops";
if (preg_match_all('/@(\w+)/', $body, $names) && isset($names[1])) {
	echo implode(",", $names[1]);
} else {
	echo "lost";
}
echo "\n";
if (preg_match('/@(\w+)/', $body, $one) && isset($one[1])) {
	echo $one[1];
} else {
	echo "lost";
}
---
admin,ops
admin
