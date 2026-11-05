<?php

try {
	Runtime\Router::Bind(function ($router) {});
} catch (RuntimeException $e) {
	echo "caught";
}
