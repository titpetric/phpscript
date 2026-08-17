<?php

// @route POST /bookmarks/{id}/delete

include "bootstrap.php";
$id = (int)$_PATH["id"];
$bookmark = $db->get("SELECT id FROM bookmarks WHERE id = ?", $id);
if (!$bookmark) {
	die("Bookmark not found.");
}

$db->query("DELETE FROM bookmarks WHERE id = ?", $id);
$db->close();
redirect_to("/");
