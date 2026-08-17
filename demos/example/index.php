<?php

// @route GET /

include "bootstrap.php";
$bookmarks = $db->get_all("SELECT id, title, url, created_at FROM bookmarks ORDER BY id DESC");
$message = "";
if (isset($_GET["saved"])) {
	$message = "Bookmark saved.";
}

$tpl->load("list.tpl");
$tpl->assign(array("title" => "Bookmarks", "bookmarks" => $bookmarks, "message" => $message));
$tpl->render();
$db->close();
