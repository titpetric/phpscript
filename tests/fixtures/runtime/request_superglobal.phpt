name: $_REQUEST is seeded as an array
description: >
  $_REQUEST used to be a reserved name that read as null; it is now seeded
  with the request fields, so a CLI run finds an empty array the way php's
  CLI does. What it carries under a server differs deliberately: the route's
  path values are merged over the query, form and cookie fields, which no
  request reaches a CLI run to show. The write behaviour shares
  superglobals.phpt with the other eight.
---
<?php

// Installed with the other superglobals, so a CLI read finds an array
// rather than null.
var_dump(isset($_REQUEST));
var_dump($_REQUEST);

// Visible inside a function without a global declaration.
function read_request() { return $_REQUEST; }
var_dump(read_request());
---
bool(true)
array(0) {
}
array(0) {
}
