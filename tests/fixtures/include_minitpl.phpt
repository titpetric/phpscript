name: include minitpl
description: This includes titpetric/minitpl source files.
---
<?php

include("code/Compiler.php");
include("code/Template.php");

$tpl = new Template();
$tpl->set_paths('templates/');
$tpl->set_compile_location('cache/', false);

$tpl->load('hello.tpl');
$tpl->assign('name', 'phpscript');
$tpl->render();
---
Hello, phpscript!
