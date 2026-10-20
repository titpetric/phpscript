name: destroy ends the session and expires its cookie
runner:
  php: false
description: >
  A logout has two halves and both are the manager's: the data behind the
  cookie is deleted, so a client that kept the cookie holds nothing, and a
  Set-Cookie goes out that expires the one the browser has. The attributes
  match what start() wrote, because a client applies a deletion to the cookie
  it matches, and Max-Age=0 is what net/http writes for an expiry in the past.

  The response assertion is the whole point of the fixture: the session the
  request arrives with is the only one, so the deletion is the only staged
  header and can be named exactly. A session started inside the request would
  stage its own cookie first, with an id from crypto/rand that no expectation
  can carry.

  Destroying twice, and destroying a session that was already pruned, are not
  errors. Both are the request the client meant, and both end with no session.
request:
  cookie:
    session: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
response:
  headers:
    Set-Cookie: "session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax"
---
<?php
$storage = new Session\Storage\Memory;
$storage->save("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "4CD5VEBDA10GAVVT88QN6YW5EZ");

$session = new Session\Manager($storage);
var_dump($session->valid());
echo $session->get(), "\n";

$session->destroy();
var_dump($session->valid());

// The data is gone from the storage and not only from this manager, so the
// cookie a client kept is worth nothing to the next request either.
$next = new Session\Manager($storage);
var_dump($next->valid());

// Logging out twice is the same request twice.
$session->destroy();
var_dump($session->valid());
?>
---
bool(true)
4CD5VEBDA10GAVVT88QN6YW5EZ
bool(false)
bool(false)
bool(false)
