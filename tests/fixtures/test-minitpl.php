<?php

include("minitpl/Compiler.php");
include("minitpl/Template.php");

$tpl = new Template();
$tpl->set_paths('templates/');
$tpl->set_compile_location('cache/', false);

$tpl->load('hello.tpl');
$tpl->assign('name', 'phpscript');
$tpl->render();
