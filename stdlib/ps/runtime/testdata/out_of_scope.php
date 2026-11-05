<?php

$kept = null;
Runtime\Router::Bind(function ($router) use (&$kept) { $kept = $router; });

try {
	$kept->handle("GET /late", function ($w, $r) {});
} catch (RuntimeException $e) {
	echo "late refused\n";
}
echo method_exists($kept, "routes") ? "lists" : "no listing", "\n";
