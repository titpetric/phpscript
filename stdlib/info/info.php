#!/usr/bin/env phpscript
<?php
phpinfo();

if (getenv("PHPSCRIPT_INFO_VERBOSE") !== "1") {
	return;
}

echo "\n# Classes\n\n";
foreach (get_declared_classes() as $class) {
	echo "## " . $class . "\n\n";
}

echo "## Functions\n\n";
$functions = get_defined_functions();
foreach ($functions["internal"] as $function) {
	echo "- `" . $function . "`\n";
}
