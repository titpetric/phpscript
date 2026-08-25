<?php

// Compile every template in templates/ through minitpl and report the
// generated php, so the escaping rewrite can be checked without a browser.

include "vendor/autoload.php";

$tpl = new MiniTPL\Template();
$tpl->set_paths("templates/");
$tpl->set_compile_location("cache/", false);

$files = glob("templates/*.tpl");
foreach ($files as $file) {
	$name = basename($file);
	$tpl->load($name);
	echo "== " . $name . "\n";
}

echo "done\n";
