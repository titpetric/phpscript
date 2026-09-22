name: a Mail handle discloses nothing about the server behind it
description: >
  The invariant the binding exists to hold. The script holds the marketing
  server, which the suite configured with a username and a password, and no
  spelling gets either back: the object carries no properties, so a property
  read is null, get_object_vars is empty and json_encode renders an empty
  object. It does not know its own server name either. The php runner is opted
  out: Mail is a host binding and the name does not exist in php.
runner:
  php: false
---
<?php

$mail = new Mail("marketing");

echo json_encode($mail), "\n";
var_dump(get_object_vars($mail));
var_dump($mail->password);
var_dump($mail->username);
var_dump($mail->host);
var_dump($mail->name);
var_dump(method_exists($mail, "send"));
?>
---
{}
array(0) {
}
NULL
NULL
NULL
NULL
bool(true)
