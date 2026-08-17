<?php

// titpetric/minitpl is a composer dependency (see composer.json), so the engine
// is reached through the autoloader rather than an include of its sources.
// phpscript implements vendor/autoload.php natively; stock PHP uses composer's
// generated one. Either way the include is explicit, as in any PHP project.
include("vendor/autoload.php");

include("code/functions.php");

$tpl = new MiniTPL\Template();
$tpl->set_paths('templates/');
$tpl->set_compile_location('cache/', false);

$tpl->load('hello.tpl');
$tpl->assign('name', 'phpscript');
$got = $tpl->get();

$want = "Hello, phpscript!\n";
if ($got !== $want) {
	echo "FAIL: rendered " . $got . ", want " . $want;
	exit(1);
}

echo $got;
