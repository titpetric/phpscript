<?php
// @route GET /stats/{counter}
$shm = new SharedMemory;
echo $shm->count($_REQUEST["counter"]);
