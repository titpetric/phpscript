<?php
// @route GET /stats/{counter}
$shm = new SharedMemory;
echo $shm->count($_PATH["counter"]);
