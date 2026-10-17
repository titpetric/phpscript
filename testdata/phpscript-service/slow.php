<?php

// @route GET /slow

// What connection_aborted() is for. The work is allowed to finish, because in a
// real application it is the transaction that already committed; what is
// skipped is the part nobody is left to read. ignore_user_abort(true) is what
// keeps the endpoint running long enough to make that choice, because without
// it the disconnect ends it where it next looks.
//
// The same loop the bridge runs, and the same answer: connection_aborted()
// reads the request's context here as it does there, so nothing is written per
// tick to find out whether anyone is still waiting.

ignore_user_abort(true);

$ticks = 0;
while ($ticks < 20) {
	usleep(100000);
	$ticks++;
	if (connection_aborted()) {
		break;
	}
}

if (connection_aborted()) {
	// Nothing is written to the response: there is nobody to write it to.
	// error_log puts this on the request's trace and through the host's error
	// handler, which is where a served script says something went wrong.
	error_log("aborted after " . $ticks . " ticks, response skipped");
	return;
}

header("Content-Type: text/plain; charset=utf-8");

echo "waited " . $ticks . " ticks, still connected\n";
