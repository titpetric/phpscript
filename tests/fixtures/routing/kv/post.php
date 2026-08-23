<?php
// @route POST /kv/{key}
$shm = new SharedMemory;
$shm->incr("requests");
$shm->incr("post");
$shm->set($_PATH["key"], $_POST["value"]);
echo "ok";
