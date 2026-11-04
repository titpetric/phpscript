<?php

Runtime\Router::Bind(function (HTTP\Mux $router) {
	$router->handle("/any", function ($w, $r) {});
});
