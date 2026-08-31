<?php

use Mobius\Render\Problem;
use Common\Config;

include "bootstrap.php";
require_once "shim.php";

echo Config::get("app", "development");
