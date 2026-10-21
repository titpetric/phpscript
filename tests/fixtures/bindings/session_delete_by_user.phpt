name: delete_by_user signs one user out of every client
runner:
  php: false
description: >
  A user holds a session per client, so removing an account is a sweep and not
  a check on the current request. delete_by_user() drops every session whose
  payload is that user's primary key and answers how many it took, which is
  what the caller reports.

  A session carries its user's key as its whole payload, so a store matches on
  that and keeps no index of its own. Another user's session is untouched, and
  a second sweep over a user holding none takes nothing and is no error.

  The request arrives with one of the swept sessions, so valid() answers false
  afterwards: the manager drops its own session id when the sweep took it,
  rather than leaving the request signed in as an account that went away.
request:
  cookie:
    session: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
---
<?php
$storage = new Session\Storage\Memory;

// Two clients of one user, and a third of another.
$storage->save("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "4CD5VEBDA10GAVVT88QN6YW5EZ");
$storage->save("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "4CD5VEBDA10GAVVT88QN6YW5EZ");
$storage->save("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "6RWBNH7F2KQ8TJYXZ3M0VDCP1A");

$session = new Session\Manager($storage);
var_dump($session->valid());

echo $session->delete_by_user("4CD5VEBDA10GAVVT88QN6YW5EZ"), "\n";
var_dump($session->valid());

// The other user's session stayed, which the storage answers for directly.
echo $storage->load("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"), "\n";

// A user holding no sessions is no error.
echo $session->delete_by_user("4CD5VEBDA10GAVVT88QN6YW5EZ"), "\n";
?>
---
bool(true)
2
bool(false)
6RWBNH7F2KQ8TJYXZ3M0VDCP1A
0
