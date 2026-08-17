<?php

// @route POST /bookmarks

include "bootstrap.php";
$title = trim($_POST["title"]);
$url = trim($_POST["url"]);
if ($title == "" || $url == "") {
	die("A title and a URL are required.");
}

$db->query("INSERT INTO bookmarks (title, url) VALUES (?, ?)", $title, $url);
$db->close();
redirect_to("/?saved=1");
