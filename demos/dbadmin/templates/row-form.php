<?php

<section class="card formcard"><div class="cardhead"><div><h2>
echo h($mode);
 row</h2><p>Leave “Use NULL” selected to store a SQL NULL value.</p></div></div><form method="post" class="stack">
foreach ($columns as $column) {
	$name = $column["name"];
	$value = "";
	$is_null = true;
	if (isset($row) && $row[$name] !== null) {
		$value = $row[$name];
		$is_null = false;
	}
<div class="fieldrow"><label><span>
	echo h($name);
 <code>
	echo h($column["type"]);
</code></span><textarea name="value_
	echo h($column["cid"]);
" rows="2">
	echo h($value);
</textarea></label><label class="check"><input type="checkbox" name="null_
	echo h($column["cid"]);
" value="1" 
	if ($is_null) {
checked
	}
> Use NULL</label></div>
}
<div class="formactions"><a class="button secondary" href="/table/
echo h($table);
">Cancel</a><button class="button">
echo h($mode);
 row</button></div></form></section>
