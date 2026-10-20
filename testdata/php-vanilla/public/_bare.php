<?php

// The /hello handler with nothing under it: no autoloader, no framework, no
// router. It answers the bytes index.php answers for GET /hello, so the
// difference between this file and that route is everything Slim does per
// request and nothing else.
//
// This is the subject of the JIT comparison. A request here is php reading one
// superglobal, one comparison and one write, which is the shape a web request
// path has: opcache serves the compilation, and whatever remains for a JIT to
// compile is a handful of opcodes that run once.

header('Content-Type: text/plain; charset=utf-8');

$name = $_GET['name'] ?? '';
if ($name === '') {
	$name = 'world';
}

echo 'hello ', $name, "\n";
