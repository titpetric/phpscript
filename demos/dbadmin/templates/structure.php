<?php

<div class="grid two"><section class="card"><div class="cardhead"><h2>Columns</h2><a class="button" href="/table/
echo h($table);
/insert">Insert row</a></div><div class="tablewrap"><table><thead><tr><th>Name</th><th>Type</th><th>Not null</th><th>Default</th><th>Primary key</th></tr></thead><tbody>
foreach ($columns as $column) {
<tr><td><b>
	echo h($column["name"]);
</b></td><td><code>
	echo h($column["type"]);
</code></td><td>
	echo $column["notnull"] ? "Yes" : "No";
</td><td>
	echo h($column["dflt_value"]);
</td><td>
	echo $column["pk"] ? "Yes" : "No";
</td></tr>
}
</tbody></table></div></section>
<section class="card"><div class="cardhead"><h2>Indexes</h2></div><div class="tablewrap"><table><thead><tr><th>Name</th><th>Unique</th><th>Origin</th></tr></thead><tbody>
foreach ($indexes as $index) {
<tr><td>
	echo h($index["name"]);
</td><td>
	echo $index["unique"] ? "Yes" : "No";
</td><td>
	echo h($index["origin"]);
</td></tr>
}
if (count($indexes) == 0) {
<tr><td colspan="3" class="empty">No indexes</td></tr>
}
</tbody></table></div></section></div>
<section class="card sqlcard"><div class="cardhead"><h2>CREATE statement</h2></div><pre>
echo h($meta["sql"]);
</pre></section><section class="dangerzone"><h2>Danger zone</h2><p>Permanently remove this table and all of its records.</p><form method="post" action="/table/
echo h($table);
/drop" onsubmit="return confirm('Drop table 
echo h($table);
?')"><label>Type <b>
echo h($table);
</b> to confirm<input name="confirmation" required></label><button class="button dangerbtn">Drop table</button></form></section>
