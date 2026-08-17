<?php

// @startup

$migrate = new Database\Migrate("dbadmin");

$migrate->load("./schema/*.up.sql");
$migrate->run();
