<?php

Runtime\Router::Bind(function (HTTP\Mux $router) {
	$router->handle("GET\t/tabbed", function ($w, $r) { $w->write("tabbed"); });
	$router->handle("POST  /spaced", function ($w, $r) { $w->write("spaced"); });
});
