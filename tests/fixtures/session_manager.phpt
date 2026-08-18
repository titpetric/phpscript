name: session manager and storage bindings
runner:
  php: false
description: Session storage constructors are available to PHP and the manager validates, starts, and reads HTTP-only cookie-backed sessions.
request:
  cookie:
    session: not-a-session-id
  headers:
    Authorize: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
---
<?php
$memory = new Session\Storage\Memory;
$disk = new Session\Storage\Disk;

$headerID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
$memory->save($headerID, "header-user");

$authorized = new Session\Manager($memory);
echo $authorized->valid() ? "authorized\n" : "invalid\n";
echo $authorized->get() . "\n";

$session = new Session\Manager($memory);
$session->SessionCookieName = "sid";
$session->start(42);
echo $session->get() . "\n";
echo $session->valid() ? "valid\n" : "invalid\n";

$diskSession = new Session\Manager($disk);
$diskSession->start("disk-user");
echo $diskSession->get() . "\n";
echo $diskSession->valid() ? "valid\n" : "invalid\n";
?>
---
authorized
header-user
42
valid
disk-user
valid
