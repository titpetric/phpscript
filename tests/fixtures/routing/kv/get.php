<?php
// @route GET /kv/{key}
$shm = new SharedMemory;
$shm->incr("requests");
$shm->incr("get");
echo $shm->get($_REQUEST["key"]);
