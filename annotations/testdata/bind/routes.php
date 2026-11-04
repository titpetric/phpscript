<?php
// @startup


Runtime\Router::Bind(function (HTTP\Mux $router) {
	$router->handle("GET /ping", function ($w, $r) { $w->write("pong"); });

	$router->group(function (HTTP\Mux $api) {
		$api->use(function ($next) {
			return function ($w, $r) use ($next) {
				$w->header()->set("X-Group", "api");
				$next($w, $r);
			};
		});
		$api->handle("GET /api/users/{id}", function ($w, $r) { $w->write("user " . $r->path_value("id")); });
	});
});
