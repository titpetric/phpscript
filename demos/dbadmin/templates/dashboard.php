<?php

<div class="stats"><div class="stat"><span>Tables</span><strong>
echo $table_count;
</strong><small>User-managed tables</small></div><div class="stat"><span>Columns</span><strong>
echo $column_count;
</strong><small>Across the database</small></div><div class="stat"><span>Rows</span><strong>
echo $row_count;
</strong><small>Catalogue records</small></div></div>
<section class="card"><div class="cardhead"><div><h2>Table catalogue</h2><p>Inspect schemas and manage records.</p></div><a class="button" href="/table/create">+ New table</a></div>
<div class="tablewrap"><table><thead><tr><th>Table</th><th>Columns</th><th>Rows</th><th>Definition</th><th>Actions</th></tr></thead><tbody>
foreach ($tables as $table) {
<tr><td><a class="tablename" href="/table/
	echo h($table["name"]);
">
	echo h($table["name"]);
</a></td><td>
	echo $table["columns"];
</td><td>
	echo $table["rows"];
</td><td><code class="truncate">
	echo h($table["sql"]);
</code></td><td class="actions"><a href="/table/
	echo h($table["name"]);
">Browse</a><a href="/table/
	echo h($table["name"]);
/structure">Structure</a></td></tr>
}
</tbody></table></div></section>
