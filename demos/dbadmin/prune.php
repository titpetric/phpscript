<?php

// @schedule hourly -- prune

/**
 * Removes the sessions nobody can use any more.
 *
 * Two stores hold session state and neither expires on its own. The
 * user_session table keeps a row per sign-in, including the revoked ones a
 * replayed cookie has to be answered with; and Session\Storage\Disk keeps a
 * file per cookie under the temporary directory, which is where the id lives
 * that the row is looked up by.
 *
 * Both are swept together and on the same clock. A row is kept for a day past
 * its expiry so that a cookie presented just after the session ended is
 * answered by a row that says no rather than by silence, and the disk store is
 * pruned to the same age.
 */

include "bootstrap.php";

$storage = new Session\Storage\Disk;

$storage->prune(60 * 60 * 24 * 2);

$sessions->prune();

echo "pruned expired sessions\n";
