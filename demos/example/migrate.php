<?php

// @startup

$migrate = new Database\Migrate("bookmarks");

$migrate->load("./schema/*.up.sql");
$migrate->run();
