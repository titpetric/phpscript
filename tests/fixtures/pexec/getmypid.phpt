name: getmypid, the core spelling of the process id
description: >
  getmypid is core standard and posix_getpid is ext/posix, an optional
  extension a host can be built without; both name the running process and
  return the identical value, so phpscript binds both to one
  implementation. A pid differs per run, so every assertion is on shape -
  the type, that it is positive, and that the two spellings agree.
  putenv/getenv and bin2hex(random_bytes()) cover the neighbouring jobs of
  a per-process key. Guards the fix for
  https://github.com/titpetric/phpscript/issues/90.
---
<?php

// The process id, under the ext/posix spelling.
$pid = posix_getpid();
var_dump(is_int($pid));
var_dump($pid > 0);

// The environment, the other way a worker is handed an identity of its own
// from outside.
putenv("MOBIUS_WORKER=3");
var_dump(getenv("MOBIUS_WORKER"));

// A unique string without uniqid(). random_bytes and bin2hex are both bound,
// and ulid()/uuid() cover the same job with better ids.
var_dump(strlen(bin2hex(random_bytes(13))));
var_dump(bin2hex(random_bytes(8)) !== bin2hex(random_bytes(8)));

// getmypid, the core spelling of the value posix_getpid() already returns.
var_dump(is_int(getmypid()));
var_dump(getmypid() > 0);
var_dump(getmypid() === posix_getpid());
?>
---
bool(true)
bool(true)
string(1) "3"
int(26)
bool(true)
bool(true)
bool(true)
bool(true)
