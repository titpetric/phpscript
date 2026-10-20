<?php
// helper.php is beside this file and not in the working directory, so the
// include resolves in the directory of the calling script.
include "helper.php";
// counter.php exists in both places: the working directory wins.
include "counter.php";
