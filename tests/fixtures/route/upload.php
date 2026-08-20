<?php
// @route POST /upload
$file = $_FILES["report"];
echo $_POST["title"] . "|" . $file["name"] . "|" . $file["size"] . "|" . $file["error"] . "|" . $file["tmp_name"];
