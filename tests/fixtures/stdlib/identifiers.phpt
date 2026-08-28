name: ulid and uuid mint k-sortable identifiers
description: >
  ulid() and uuid() are phpscript extensions (PHP's own library mints no
  unique ids), so the php runner is opted out. Both format the same layout,
  a millisecond timestamp over random bits: the ulid as 26 characters of
  Crockford base32, the uuid as a version 7 UUID, so ids of either kind
  sort by creation time.
runner:
  php: false
---
<?php

$u1 = ulid();
$spin = microtime(true);
while (microtime(true) - $spin < 0.003) {
}
$u2 = ulid();
echo strlen($u1), " ", strlen($u2), "\n";
echo preg_match('/^[0-9A-HJKMNP-TV-Z]{26}$/', $u1), "\n";
echo $u1 < $u2 ? "ulids sort" : "ulids do not sort", "\n";

$v1 = uuid();
$spin = microtime(true);
while (microtime(true) - $spin < 0.003) {
}
$v2 = uuid();
echo strlen($v1), "\n";
echo preg_match('/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/', $v1), "\n";
echo $v1 < $v2 ? "uuids sort" : "uuids do not sort", "\n";
echo $u1 === $u2 || $v1 === $v2 ? "collision" : "unique", "\n";
---
26 26
1
ulids sort
36
1
uuids sort
unique
