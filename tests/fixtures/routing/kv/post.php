<?php
// @route POST /kv/{key}
$shm = new SharedMemory;
$shm->incr("requests");
$shm->incr("post");
$shm->set($_REQUEST["key"], $_POST["value"]);
echo "ok";
