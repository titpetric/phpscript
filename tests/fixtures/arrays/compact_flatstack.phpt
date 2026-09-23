name: compact collects the names it is given, on both engines
description: >
  The bytecode compiler writes compact("a", "b") out as the map it names,
  because a flat frame has erased the names into slots by the time the binding
  would read them. This covers what that translation has to get right: a name
  that is not set is omitted, a name set to null is not, a name assigned in a
  branch that did not run is omitted, and a parameter counts as set. Key order
  is not asserted, because compact answers a native map and the order is Go's.
---
<?php

$title = "Dashboard";
$count = 3;
$nothing = null;

$vars = compact("title", "count", "nothing", "missing");
echo count($vars), "\n";
echo $vars["title"], ":", $vars["count"], "\n";

// A name holding null is set, so it is collected.
var_dump(array_key_exists("nothing", $vars));
var_dump($vars["nothing"]);

// A name that was never assigned is not.
var_dump(array_key_exists("missing", $vars));

// A branch that did not run leaves its name unset.
if ($count > 100) {
	$unreached = "no";
}
$branch = compact("unreached");
echo count($branch), "\n";

// Parameters and locals of a function are in scope; a global is not.
function collect($name) {
	$local = "inside";
	return compact("name", "local", "title");
}
$locals = collect("worker");
echo count($locals), ":", $locals["name"], ":", $locals["local"], "\n";

// Assigning after the call does not reach back into it.
$later = compact("afterwards");
$afterwards = 1;
echo count($later), "\n";

// A name the compiler cannot read off the source is resolved when the call
// runs: a variable holding one, and a call answering one.
$which = "title";
function pick() { return "count"; }

$byVar = compact($which);
echo count($byVar), ":", $byVar["title"], "\n";

$byCall = compact(pick());
echo count($byCall), ":", $byCall["count"], "\n";

// The two forms mix, and a dynamic name that is not set is omitted too.
$mixed = compact("title", $which, pick(), "absent");
echo count($mixed), "\n";

// Order is Go's, so a script that needs one names its keys. This is the
// spelling docs/README.md points at.
foreach (["title", "count"] as $k) {
	echo $k, "=", $vars[$k], ";";
}
echo "\n";
?>
---
3
Dashboard:3
bool(true)
NULL
bool(false)
0
2:worker:inside
0
1:Dashboard
1:3
2
title=Dashboard;count=3;
