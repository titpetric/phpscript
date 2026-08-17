<?php

// titpetric/minitpl comes from composer; every entrypoint includes this file,
// so this is the one place the autoloader has to be pulled in.
include "vendor/autoload.php";

$db = new Database("bookmarks");

$tpl = new MiniTPL\Template("templates/");

$tpl->set_compile_location("cache/", false);

/** Sends a redirect and stops the script. */
function redirect_to($url) {
	header("Location: " . $url);
	exit();
}
