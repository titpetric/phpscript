name: execution limits and the client connection
runner:
  php: false
description: >
  set_time_limit, ignore_user_abort and connection_aborted, in phpscript's
  types rather than php's: connection_aborted answers a bool where php answers
  int 1 or 0, and ignore_user_abort takes a required bool and returns nothing
  where php's is readable as well as settable and answers the previous setting.
  A command line run has no client, so nothing is ever aborted and setting
  ignore_user_abort changes no answer. sleep and usleep wait on the runtime
  context rather than on the clock alone, so a script that has run out of time
  cannot sit in one; neither has anything to wait for here. What running out of
  time does - a fatal no catch takes, with the shutdown callbacks still run - is
  held by the Go tests in runner/deadline_test.go, because the harness answers a
  failed fixture with a status rather than with what it printed. Only phpscript
  defines the expected output.
---
<?php

var_dump(set_time_limit(30));
var_dump(connection_aborted());

ignore_user_abort(true);
var_dump(connection_aborted());

ignore_user_abort(false);
var_dump(set_time_limit(0));

register_shutdown_function(function () {
	echo "shutdown ran\n";
});

sleep(0);
usleep(1000);
echo "waits returned\n";

// A limit far enough out that finishing is what happens.
set_time_limit(30);
for ($i = 0; $i < 1000; $i++) {
	$n = $i;
}
echo "loop finished\n";
---
bool(true)
bool(false)
bool(false)
bool(true)
waits returned
loop finished
shutdown ran
