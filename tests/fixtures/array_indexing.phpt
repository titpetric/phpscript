name: array indexing
flatstack: true
description: >
  Test nested array reads and minitpl-style foreach targets.
---
<?php
$a = array(
	"foo" => array(
		"bar" => array("baz")
	)
);

echo $a['foo']['bar'][0];

if (!empty($a['foo']['bar'])) foreach ($a['foo']['bar'] as $val) echo $val;

$_v = array(
	"body" => array(
		"jobs" => array(
			"first" => array("name" => "backup", "history" => array(1, 2)),
			"second" => array("name" => "cleanup", "history" => array(3)),
		),
	),
);

if (!empty($_v['body']['jobs'])) foreach ($_v['body']['jobs'] as $_v['k'] => $_v['job']) {
	echo "|" . $_v['k'] . ":" . $_v['job']['name'] . "=";
	foreach ($_v['job']['history'] as $_v['point']) echo $_v['point'];
}
?>
---
bazbaz|first:backup=12|second:cleanup=3
