name: compact
description: >
  Test that compact collects defined names from the current execution scope.
---
<?php
$title = "Dashboard";
$body = array("jobs" => 3);
$vars = compact("title", "body", "missing");

echo $vars['title'] . ":" . $vars['body']['jobs'];
if (!isset($vars['missing'])) echo ":missing-omitted";

function collect($name) {
	$local = "inside";
	return compact("name", "local", "title");
}

$locals = collect("worker");
echo "|" . $locals['name'] . ":" . $locals['local'];
if (!isset($locals['title'])) echo ":global-omitted";
?>
---
Dashboard:3:missing-omitted|worker:inside:global-omitted
