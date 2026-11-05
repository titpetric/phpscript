<?php

$mux = mounted();
$mux->handle("GET /open", function ($w, $r) { $w->write("open"); });
$mux->group(function (HTTP\Mux $g) {
	$g->use(function ($next) { return $next; });
	$g->handle("GET /grouped", function ($w, $r) { $w->write("grouped"); });
});
