<?php
$mux = new HTTP\Mux();

function tag(string $name) {
	return function ($next) use ($name) {
		return function ($w, $r) use ($next, $name) {
			$w->write($name . ">");
			$next($w, $r);
		};
	};
}

$mux->handle("GET /open", function ($w, $r) { $w->write("open"); });
$mux->use(tag("outer"));
$mux->group(function (HTTP\Mux $g) {
	$g->use(tag("inner"));
	$g->handle("GET /grouped", function ($w, $r) { $w->write("grouped"); });
});
$mux->handle("GET /after", function ($w, $r) { $w->write("after"); });

define("MUX", $mux);

