<?php

// @route GET /slow

// What connection_aborted() is for. The work finishes and this drops the
// response, because nobody remains to read it. ignore_user_abort(true) keeps
// the endpoint running long enough to make that choice, and without it the
// disconnect ends it where it next looks. connection_aborted() reads the
// request's context, and no tick writes a byte to find out who is waiting.

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
	// error_log puts this on the request's trace and through the host's error
	// handler, the channel a served script reports a failure on.
	error_log("aborted after " . $ticks . " ticks, response skipped");
	return;
}

header("Content-Type: text/plain; charset=utf-8");

echo "waited " . $ticks . " ticks, still connected\n";
