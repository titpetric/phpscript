<?php

// What a page behind the login includes: setup.php plus the gate. A controller
// added later then requires a session by default, and a public page says so by
// including setup.php instead.
//
// That is the whole reason this file holds two lines, and no line repeats in
// every controller: a gate per controller is a gate somebody forgets, and a
// page that forgets it then answers without a session.
include "setup.php";

require_login($session, $db, $tpl);
