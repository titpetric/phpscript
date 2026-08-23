<?php

try {

$_SERVER = array(
	'REQUEST_METHOD' => "GET",
	'REQUEST_URI' => "/",
);

switch (count($argv)) {
	case 1:
		break;
	case 2:
		$_SERVER['REQUEST_METHOD'] = 'GET';
		$_SERVER['REQUEST_URI'] = $argv[1];
		break;
	case 3:
		$_SERVER['REQUEST_METHOD'] = $argv[1];
		$_SERVER['REQUEST_URI'] = $argv[2];
		// TODO: how to use with stdin+request body in CLI
		break;
}

if (count($argv) > 1) {
	// pass url as argument in CLI
	$_SERVER['REQUEST_URI'] = $argv[1];
}

$path = explode("/", trim($_SERVER['REQUEST_URI'], "/"));
$module = $path[0];
$modules = array("menu");

echo "path: " . json_encode($path) . "\n";
echo "module: " . $module . "\n";

if ($module != "") {
	if (in_array($module, $modules)) {
		$filename = sprintf("./modules/%s.php", $module);
		include($filename);

		$module = new module;
		echo "Got module: " . $module->name() . "\n";
	}
} else {
	throw new Exception("Invalid route", 403);
}

} catch (Exception $e) {
	echo sprintf("HTTP %d: %s\n", $e->getCode(), $e->getMessage());
}
